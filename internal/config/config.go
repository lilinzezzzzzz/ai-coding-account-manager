package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

const (
	defaultBindAddr = "127.0.0.1:43127"
	envBindAddr     = "AI_CODING_ACCOUNT_MANAGER_BIND_ADDR"
	envDataDir      = "AI_CODING_ACCOUNT_MANAGER_DATA_DIR"
	envCodexBin     = "AI_CODING_ACCOUNT_MANAGER_CODEX_BIN"
	envCodexHome    = "CODEX_HOME"
	envProviderMode = "AI_CODING_ACCOUNT_MANAGER_PROVIDER_MODE"
	envContainer    = "AI_CODING_ACCOUNT_MANAGER_CONTAINER"
	appDirName      = "ai-coding-account-manager"
)

// Config 保存应用启动配置。
type Config struct {
	BindAddr     string
	DataDir      string
	CodexBin     string
	CodexHome    string
	ProviderMode string
}

// Load 从环境变量读取配置，并填充本地运行默认值。
func Load() (Config, error) {
	bindAddr := os.Getenv(envBindAddr)
	if bindAddr == "" {
		bindAddr = defaultBindAddr
	}
	if err := validateBindAddr(bindAddr, os.Getenv(envContainer) == "1"); err != nil {
		return Config{}, err
	}

	dataDir, err := loadDataDir()
	if err != nil {
		return Config{}, err
	}
	codexHome, err := loadCodexHome()
	if err != nil {
		return Config{}, err
	}
	return Config{
		BindAddr:     bindAddr,
		DataDir:      dataDir,
		CodexBin:     os.Getenv(envCodexBin),
		CodexHome:    codexHome,
		ProviderMode: os.Getenv(envProviderMode),
	}, nil
}

func validateBindAddr(bindAddr string, allowContainerWildcard bool) error {
	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return fmt.Errorf("invalid bind address: %w", err)
	}
	if port == "" {
		return fmt.Errorf("invalid bind address: port is required")
	}
	if host != "127.0.0.1" && host != "localhost" && !(allowContainerWildcard && host == "0.0.0.0") {
		return fmt.Errorf("invalid bind address: host must be loopback")
	}
	return nil
}

func loadDataDir() (string, error) {
	if dataDir := os.Getenv(envDataDir); dataDir != "" {
		absDataDir, err := filepath.Abs(dataDir)
		if err != nil {
			return "", fmt.Errorf("invalid data dir: %w", err)
		}
		return absDataDir, nil
	}

	baseDir := os.Getenv("XDG_DATA_HOME")
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".local", "share")
	}
	return filepath.Join(baseDir, appDirName), nil
}

func loadCodexHome() (string, error) {
	if codexHome := os.Getenv(envCodexHome); codexHome != "" {
		absCodexHome, err := filepath.Abs(codexHome)
		if err != nil {
			return "", fmt.Errorf("invalid CODEX_HOME: %w", err)
		}
		return absCodexHome, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(homeDir, ".codex"), nil
}
