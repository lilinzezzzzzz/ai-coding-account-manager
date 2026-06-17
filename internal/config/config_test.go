package config_test

import (
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/config"
)

func TestLoadUsesDefaultBindAddr(t *testing.T) {
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_BIND_ADDR", "")
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_DATA_DIR", "")
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_CODEX_BIN", "")
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_PROVIDER_MODE", "")
	t.Setenv("CODEX_HOME", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BindAddr != "127.0.0.1:43127" {
		t.Fatalf("BindAddr = %q, want 127.0.0.1:43127", cfg.BindAddr)
	}
	if cfg.DataDir == "" {
		t.Fatal("DataDir is empty")
	}
	if cfg.CodexHome == "" {
		t.Fatal("CodexHome is empty")
	}
}

func TestLoadRejectsNonLoopbackBindAddr(t *testing.T) {
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_BIND_ADDR", "0.0.0.0:43127")
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_DATA_DIR", t.TempDir())
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_CONTAINER", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadAllowsContainerWildcardBindAddr(t *testing.T) {
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_BIND_ADDR", "0.0.0.0:43127")
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_DATA_DIR", t.TempDir())
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_CONTAINER", "1")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BindAddr != "0.0.0.0:43127" {
		t.Fatalf("BindAddr = %q, want 0.0.0.0:43127", cfg.BindAddr)
	}
}

func TestLoadUsesConfiguredDataDir(t *testing.T) {
	dataDir := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_BIND_ADDR", "")
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_DATA_DIR", dataDir)
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_CODEX_BIN", "/opt/codex")
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_PROVIDER_MODE", "fake")
	t.Setenv("CODEX_HOME", codexHome)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DataDir != dataDir {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, dataDir)
	}
	if cfg.CodexHome != codexHome {
		t.Fatalf("CodexHome = %q, want %q", cfg.CodexHome, codexHome)
	}
	if cfg.CodexBin != "/opt/codex" {
		t.Fatalf("CodexBin = %q, want /opt/codex", cfg.CodexBin)
	}
	if cfg.ProviderMode != "fake" {
		t.Fatalf("ProviderMode = %q, want fake", cfg.ProviderMode)
	}
}
