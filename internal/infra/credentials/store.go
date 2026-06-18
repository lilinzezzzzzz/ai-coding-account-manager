package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const authFileName = "auth.json"

// Store 管理 provider 隔离凭证目录和活动 CODEX_HOME 的 auth.json 替换。
type Store struct {
	rootDir        string
	activeCodexDir string
}

// Config 保存凭据目录配置。
type Config struct {
	RootDir        string
	ActiveCodexDir string
}

// NewStore 创建凭据 store。
func NewStore(cfg Config) (*Store, error) {
	if cfg.RootDir == "" {
		return nil, fmt.Errorf("credentials root dir is required")
	}
	if cfg.ActiveCodexDir == "" {
		return nil, fmt.Errorf("active codex dir is required")
	}
	rootDir, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve credentials root dir: %w", err)
	}
	activeCodexDir, err := filepath.Abs(cfg.ActiveCodexDir)
	if err != nil {
		return nil, fmt.Errorf("resolve active codex dir: %w", err)
	}
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		return nil, fmt.Errorf("create credentials root dir: %w", err)
	}
	return &Store{rootDir: rootDir, activeCodexDir: activeCodexDir}, nil
}

// ActiveCodexDir 返回当前活动 CODEX_HOME。
func (store *Store) ActiveCodexDir() string {
	return store.activeCodexDir
}

// AccountCodexDir 返回账号隔离 CODEX_HOME。
func (store *Store) AccountCodexDir(providerID string, storageID string) (string, error) {
	if err := validateSegment(providerID); err != nil {
		return "", err
	}
	if err := validateSegment(storageID); err != nil {
		return "", err
	}
	return filepath.Join(store.rootDir, "providers", providerID, "accounts", storageID), nil
}

// ImportFromCodexDir 把来源 CODEX_HOME 的 auth.json 写入账号隔离目录。
func (store *Store) ImportFromCodexDir(ctx context.Context, providerID string, storageID string, sourceCodexDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	accountDir, err := store.AccountCodexDir(providerID, storageID)
	if err != nil {
		return err
	}
	sourceAuth := filepath.Join(sourceCodexDir, authFileName)
	if err := validateAuthFile(sourceAuth); err != nil {
		return err
	}
	if err := os.MkdirAll(accountDir, 0o700); err != nil {
		return fmt.Errorf("create account credentials dir: %w", err)
	}
	return copyAuthAtomic(sourceAuth, filepath.Join(accountDir, authFileName))
}

// ValidateAccount 校验账号隔离目录中是否存在可用 auth.json。
func (store *Store) ValidateAccount(ctx context.Context, providerID string, storageID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	accountDir, err := store.AccountCodexDir(providerID, storageID)
	if err != nil {
		return err
	}
	return validateAuthFile(filepath.Join(accountDir, authFileName))
}

// ExportToCodexDir 把账号隔离 auth.json 写入目标 CODEX_HOME。
func (store *Store) ExportToCodexDir(ctx context.Context, providerID string, storageID string, targetCodexDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	accountDir, err := store.AccountCodexDir(providerID, storageID)
	if err != nil {
		return err
	}
	sourceAuth := filepath.Join(accountDir, authFileName)
	if err := validateAuthFile(sourceAuth); err != nil {
		return err
	}
	if err := os.MkdirAll(targetCodexDir, 0o700); err != nil {
		return fmt.Errorf("create target codex dir: %w", err)
	}
	return copyAuthAtomic(sourceAuth, filepath.Join(targetCodexDir, authFileName))
}

// ActivateAccount 把账号隔离 auth.json 原子替换为活动 CODEX_HOME 的 auth.json。
func (store *Store) ActivateAccount(ctx context.Context, providerID string, storageID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	accountDir, err := store.AccountCodexDir(providerID, storageID)
	if err != nil {
		return err
	}
	sourceAuth := filepath.Join(accountDir, authFileName)
	if err := validateAuthFile(sourceAuth); err != nil {
		return err
	}
	if err := os.MkdirAll(store.activeCodexDir, 0o700); err != nil {
		return fmt.Errorf("create active codex dir: %w", err)
	}
	return copyAuthAtomic(sourceAuth, filepath.Join(store.activeCodexDir, authFileName))
}

// RemoveAccount 删除账号隔离凭据目录。
func (store *Store) RemoveAccount(ctx context.Context, providerID string, storageID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	accountDir, err := store.AccountCodexDir(providerID, storageID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(accountDir); err != nil {
		return fmt.Errorf("remove account credentials dir: %w", err)
	}
	return nil
}

func copyAuthAtomic(sourcePath string, targetPath string) error {
	if err := validateAuthFile(sourcePath); err != nil {
		return err
	}

	targetDir := filepath.Dir(targetPath)
	tempFile, err := os.CreateTemp(targetDir, ".auth-*.json")
	if err != nil {
		return fmt.Errorf("create temp auth file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("open source auth file: %w", err)
	}
	defer sourceFile.Close()

	if _, err := io.Copy(tempFile, sourceFile); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("copy auth file: %w", err)
	}
	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temp auth file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync temp auth file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp auth file: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("replace auth file: %w", err)
	}
	return nil
}

func validateAuthFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open auth file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 2*1024*1024))
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode auth file: %w", err)
	}
	if len(value) == 0 {
		return fmt.Errorf("auth file is empty")
	}
	return nil
}

func validateSegment(value string) error {
	if value == "" || value == "." || value == ".." {
		return fmt.Errorf("invalid credentials path segment")
	}
	if filepath.Base(value) != value {
		return fmt.Errorf("invalid credentials path segment")
	}
	return nil
}
