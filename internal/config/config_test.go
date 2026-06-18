package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/config"
)

func TestLoadFileUsesDefaultsWhenConfigIsMissing(t *testing.T) {
	t.Setenv("CODEX_HOME", "")

	cfg, err := config.LoadFile(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	wantRoot := projectRootForTest(t)
	if cfg.BindAddr != "127.0.0.1:43127" {
		t.Fatalf("BindAddr = %q, want 127.0.0.1:43127", cfg.BindAddr)
	}
	if cfg.ConfigDir != filepath.Dir(cfg.ConfigFile) {
		t.Fatalf("ConfigDir = %q, want config file dir", cfg.ConfigDir)
	}
	if cfg.DataDir != filepath.Join(wantRoot, ".data") {
		t.Fatalf("DataDir = %q, want project .data", cfg.DataDir)
	}
	if cfg.CredentialsDir != filepath.Join(wantRoot, ".credentials") {
		t.Fatalf("CredentialsDir = %q, want project .credentials", cfg.CredentialsDir)
	}
	if cfg.CodexHome == "" {
		t.Fatal("CodexHome is empty")
	}
}

func TestLoadFileUsesConfigFile(t *testing.T) {
	t.Setenv("CODEX_HOME", "")
	configFile := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(configFile, []byte(`{
  "bindAddr": "127.0.0.1:43128",
  "dataDir": ".custom-data",
  "credentialsDir": ".custom-credentials",
  "codexBin": "/opt/codex-file",
  "codexHome": ".custom-codex-home",
  "providerMode": "fake"
}`), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := config.LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	wantRoot := projectRootForTest(t)
	if cfg.ConfigFile != configFile {
		t.Fatalf("ConfigFile = %q, want %q", cfg.ConfigFile, configFile)
	}
	if cfg.BindAddr != "127.0.0.1:43128" {
		t.Fatalf("BindAddr = %q, want file value", cfg.BindAddr)
	}
	if cfg.DataDir != filepath.Join(wantRoot, ".custom-data") {
		t.Fatalf("DataDir = %q, want file relative path", cfg.DataDir)
	}
	if cfg.CredentialsDir != filepath.Join(wantRoot, ".custom-credentials") {
		t.Fatalf("CredentialsDir = %q, want file relative path", cfg.CredentialsDir)
	}
	if cfg.CodexBin != "/opt/codex-file" {
		t.Fatalf("CodexBin = %q, want file value", cfg.CodexBin)
	}
	if cfg.CodexHome != filepath.Join(wantRoot, ".custom-codex-home") {
		t.Fatalf("CodexHome = %q, want file relative path", cfg.CodexHome)
	}
	if cfg.ProviderMode != "fake" {
		t.Fatalf("ProviderMode = %q, want fake", cfg.ProviderMode)
	}
}

func TestLoadFileCodexHomeOverridesEnvironment(t *testing.T) {
	envCodexHome := t.TempDir()
	t.Setenv("CODEX_HOME", envCodexHome)
	configFile := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(configFile, []byte(`{"codexHome": ".file-codex-home"}`), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := config.LoadFile(configFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	wantRoot := projectRootForTest(t)
	if cfg.CodexHome != filepath.Join(wantRoot, ".file-codex-home") {
		t.Fatalf("CodexHome = %q, want config file override", cfg.CodexHome)
	}
}

func TestLoadFileFallsBackToCodeXHomeEnvironment(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	cfg, err := config.LoadFile(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.CodexHome != codexHome {
		t.Fatalf("CodexHome = %q, want CODEX_HOME", cfg.CodexHome)
	}
}

func TestLoadFileRejectsNonLoopbackBindAddr(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(configFile, []byte(`{"bindAddr":"0.0.0.0:43127"}`), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, err := config.LoadFile(configFile)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want error")
	}
}

func TestLoadFileRejectsUnknownConfigFileField(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "app.json")
	if err := os.WriteFile(configFile, []byte(`{"unknown": true}`), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, err := config.LoadFile(configFile)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want unknown field error")
	}
}

func projectRootForTest(t *testing.T) string {
	t.Helper()
	workingDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve working dir: %v", err)
	}
	for dir := workingDir; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return workingDir
		}
	}
}
