package database_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/model"
)

func TestOpenInitializesEmptyDatabase(t *testing.T) {
	db := openTestDatabase(t)
	defer closeTestDatabase(t, db)

	assertTableExists(t, db, "accounts")
	assertTableExists(t, db, "usage_snapshots")
	assertTableExists(t, db, "schema_migrations")

	var version int
	if err := db.GORM().Model(&model.SchemaMigration{}).Select("MAX(version)").Scan(&version).Error; err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 1 {
		t.Fatalf("schema version = %d, want 1", version)
	}

	var foreignKeys int
	if err := db.GORM().Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatalf("read foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var journalMode string
	if err := db.GORM().Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		t.Fatalf("read journal_mode pragma: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestOpenRunsMigrationsOnlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	first, err := database.Open(context.Background(), database.Config{Path: path})
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	closeTestDatabase(t, first)

	second, err := database.Open(context.Background(), database.Config{Path: path})
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer closeTestDatabase(t, second)

	var count int64
	if err := second.GORM().Model(&model.SchemaMigration{}).Count(&count).Error; err != nil {
		t.Fatalf("count schema migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("schema migration rows = %d, want 1", count)
	}
}

func TestOpenRejectsSchemaTooNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := database.Open(context.Background(), database.Config{Path: path})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.GORM().Create(&model.SchemaMigration{Version: 999, AppliedAt: 1}).Error; err != nil {
		t.Fatalf("insert future schema version: %v", err)
	}
	closeTestDatabase(t, db)

	_, err = database.Open(context.Background(), database.Config{Path: path})
	if err == nil {
		t.Fatal("Open() error = nil, want schema too new error")
	}
	var appErr *entity.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("Open() error = %v, want AppError", err)
	}
	if appErr.ErrorCode() != entity.ErrorCodeSchemaTooNew {
		t.Fatalf("ErrorCode() = %q, want %q", appErr.ErrorCode(), entity.ErrorCodeSchemaTooNew)
	}
}

func TestDSNUsesFileURIWithRequiredPragmas(t *testing.T) {
	dsn := database.DSN("/tmp/account manager/state.db")

	required := []string{
		"file:///tmp/account%20manager/state.db?",
		"_pragma=foreign_keys%281%29",
		"_pragma=journal_mode%28WAL%29",
		"_pragma=synchronous%28FULL%29",
		"_pragma=busy_timeout%285000%29",
	}
	for _, fragment := range required {
		if !strings.Contains(dsn, fragment) {
			t.Fatalf("DSN = %q, missing %q", dsn, fragment)
		}
	}
}

func openTestDatabase(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.Open(context.Background(), database.Config{
		Path: filepath.Join(t.TempDir(), "state.db"),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return db
}

func closeTestDatabase(t *testing.T, db *database.DB) {
	t.Helper()

	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func assertTableExists(t *testing.T, db *database.DB, tableName string) {
	t.Helper()

	if !db.GORM().Migrator().HasTable(tableName) {
		t.Fatalf("table %s does not exist", tableName)
	}
}
