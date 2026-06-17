package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/libtnb/sqlite"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	supportedSchemaVersion = 1
	migrationDir           = "migrations"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Config 保存 SQLite 数据库初始化参数。
type Config struct {
	Path string
}

// DB 封装 GORM 和底层 sql.DB 生命周期。
type DB struct {
	gormDB *gorm.DB
	sqlDB  *sql.DB
}

// Open 初始化 SQLite、执行 migration 并完成启动校验。
func Open(ctx context.Context, cfg Config) (*DB, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	absPath, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(DSN(absPath)), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	database := &DB{gormDB: db, sqlDB: sqlDB}
	if err := database.initialize(ctx); err != nil {
		closeErr := sqlDB.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("initialize database: %w; close database: %v", err, closeErr)
		}
		return nil, err
	}
	return database, nil
}

// DSN 根据本地绝对路径构造 SQLite URI DSN。
func DSN(path string) string {
	values := url.Values{}
	values.Add("_pragma", "foreign_keys(1)")
	values.Add("_pragma", "journal_mode(WAL)")
	values.Add("_pragma", "synchronous(FULL)")
	values.Add("_pragma", "busy_timeout(5000)")

	uri := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: values.Encode(),
	}
	return uri.String()
}

// GORM 返回基础 *gorm.DB。调用方必须按边界从 WithContext 派生。
func (db *DB) GORM() *gorm.DB {
	return db.gormDB
}

// Close 关闭底层 SQLite 连接池。
func (db *DB) Close() error {
	if db == nil || db.sqlDB == nil {
		return nil
	}
	return db.sqlDB.Close()
}

func (db *DB) initialize(ctx context.Context) error {
	if err := db.verifyPragmas(ctx); err != nil {
		return err
	}
	if err := db.rejectTooNewSchema(ctx); err != nil {
		return err
	}
	if err := db.runMigrations(ctx); err != nil {
		return err
	}
	if err := db.quickCheck(ctx); err != nil {
		return err
	}
	return nil
}

func (db *DB) verifyPragmas(ctx context.Context) error {
	var foreignKeys int
	if err := db.gormDB.WithContext(ctx).Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		return fmt.Errorf("check foreign_keys pragma: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("foreign_keys pragma = %d, want 1", foreignKeys)
	}

	var journalMode string
	if err := db.gormDB.WithContext(ctx).Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		return fmt.Errorf("check journal_mode pragma: %w", err)
	}
	if strings.ToLower(journalMode) != "wal" {
		return fmt.Errorf("journal_mode pragma = %q, want wal", journalMode)
	}
	return nil
}

func (db *DB) rejectTooNewSchema(ctx context.Context) error {
	if !db.hasSchemaMigrationsTable(ctx) {
		return nil
	}

	var currentVersion int
	err := db.gormDB.WithContext(ctx).
		Model(&model.SchemaMigration{}).
		Select("COALESCE(MAX(version), 0)").
		Scan(&currentVersion).Error
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if currentVersion > supportedSchemaVersion {
		return entity.WrapAppErrorWithMessage(entity.ErrorCodeSchemaTooNew, entity.ErrorCodeSchemaTooNew.DefaultMessage(), fmt.Errorf("schema version %d > supported %d", currentVersion, supportedSchemaVersion))
	}
	return nil
}

func (db *DB) hasSchemaMigrationsTable(ctx context.Context) bool {
	var name string
	err := db.gormDB.WithContext(ctx).
		Raw("SELECT name FROM sqlite_master WHERE type = ? AND name = ?", "table", "schema_migrations").
		Scan(&name).Error
	return err == nil && name == "schema_migrations"
}

func (db *DB) runMigrations(ctx context.Context) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		return fmt.Errorf("no migrations embedded")
	}

	return db.gormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		applied, err := appliedMigrationVersions(tx)
		if err != nil {
			return err
		}

		now := time.Now().UTC().UnixMilli()
		for _, migration := range migrations {
			if applied[migration.Version] {
				continue
			}
			if err := tx.Exec(migration.SQL).Error; err != nil {
				return fmt.Errorf("apply migration %04d: %w", migration.Version, err)
			}
			record := model.SchemaMigration{
				Version:   migration.Version,
				AppliedAt: now,
			}
			if err := tx.Create(&record).Error; err != nil {
				return fmt.Errorf("record migration %04d: %w", migration.Version, err)
			}
		}
		return nil
	})
}

func appliedMigrationVersions(tx *gorm.DB) (map[int]bool, error) {
	applied := map[int]bool{}
	if !tx.Migrator().HasTable(&model.SchemaMigration{}) {
		return applied, nil
	}

	var rows []model.SchemaMigration
	if err := tx.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	for _, row := range rows {
		applied[row.Version] = true
	}
	return applied, nil
}

func (db *DB) quickCheck(ctx context.Context) error {
	var result string
	if err := db.gormDB.WithContext(ctx).Raw("PRAGMA quick_check").Scan(&result).Error; err != nil {
		return fmt.Errorf("run quick_check: %w", err)
	}
	if result != "ok" {
		return entity.WrapAppErrorWithMessage(entity.ErrorCodeStorageCorrupted, entity.ErrorCodeStorageCorrupted.DefaultMessage(), fmt.Errorf("quick_check = %q", result))
	}
	return nil
}

type migration struct {
	Version int
	SQL     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, migrationDir)
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := parseMigrationVersion(entry.Name())
		if err != nil {
			return nil, err
		}
		content, err := migrationFS.ReadFile(filepath.ToSlash(filepath.Join(migrationDir, entry.Name())))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		migrations = append(migrations, migration{
			Version: version,
			SQL:     string(content),
		})
	}

	sort.Slice(migrations, func(i int, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

func parseMigrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("invalid migration name %q", name)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("invalid migration version %q: %w", name, err)
	}
	if version <= 0 {
		return 0, fmt.Errorf("invalid migration version %q", name)
	}
	return version, nil
}
