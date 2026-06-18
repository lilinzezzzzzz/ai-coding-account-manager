package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

const (
	defaultBindAddr = "127.0.0.1:43127"
	envCodexHome    = "CODEX_HOME"
)

// Config 保存应用启动配置。
type Config struct {
	BindAddr       string
	ConfigDir      string
	ConfigFile     string
	DataDir        string
	CredentialsDir string
	CodexBin       string
	CodexHome      string
	ProviderMode   string
}

// Load 从默认值和 config/app.json 读取配置。
func Load() (Config, error) {
	return LoadFile("")
}

// LoadFile 从指定配置文件读取配置；configFile 为空时使用 config/app.json。
// CODEX_HOME 仅作为 codexHome 未配置时的外部工具 fallback。
func LoadFile(configFile string) (Config, error) {
	rootDir, err := loadProjectRoot()
	if err != nil {
		return Config{}, err
	}
	configFile, err = resolveConfigFile(rootDir, configFile)
	if err != nil {
		return Config{}, err
	}
	configDir := filepath.Dir(configFile)
	fileConfig, err := loadFileConfig(configFile)
	if err != nil {
		return Config{}, err
	}

	bindAddr := stringValue(defaultBindAddr, fileConfig.BindAddr)
	if err := validateBindAddr(bindAddr); err != nil {
		return Config{}, err
	}
	dataDir, err := resolveConfiguredDir(rootDir, filepath.Join(rootDir, ".data"), fileConfig.DataDir, "data dir")
	if err != nil {
		return Config{}, err
	}
	credentialsDir, err := resolveConfiguredDir(rootDir, filepath.Join(rootDir, ".credentials"), fileConfig.CredentialsDir, "credentials dir")
	if err != nil {
		return Config{}, err
	}
	codexHome, err := loadCodexHome(rootDir, fileConfig.CodexHome)
	if err != nil {
		return Config{}, err
	}
	return Config{
		BindAddr:       bindAddr,
		ConfigDir:      configDir,
		ConfigFile:     configFile,
		DataDir:        dataDir,
		CredentialsDir: credentialsDir,
		CodexBin:       stringValue("", fileConfig.CodexBin),
		CodexHome:      codexHome,
		ProviderMode:   stringValue("", fileConfig.ProviderMode),
	}, nil
}

type fileConfig struct {
	BindAddr       *string `json:"bindAddr"`
	DataDir        *string `json:"dataDir"`
	CredentialsDir *string `json:"credentialsDir"`
	CodexBin       *string `json:"codexBin"`
	CodexHome      *string `json:"codexHome"`
	ProviderMode   *string `json:"providerMode"`
}

func validateBindAddr(bindAddr string) error {
	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return fmt.Errorf("invalid bind address: %w", err)
	}
	if port == "" {
		return fmt.Errorf("invalid bind address: port is required")
	}
	if host != "127.0.0.1" && host != "localhost" {
		return fmt.Errorf("invalid bind address: host must be loopback")
	}
	return nil
}

func loadProjectRoot() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working dir: %w", err)
	}
	return projectRoot(workingDir), nil
}

func resolveConfigFile(rootDir string, configFile string) (string, error) {
	if configFile == "" {
		configFile = filepath.Join(rootDir, "config", "app.json")
	}
	return resolvePath(rootDir, "", configFile, "config file")
}

func loadFileConfig(configFile string) (fileConfig, error) {
	configFile, err := filepath.Abs(configFile)
	if err != nil {
		return fileConfig{}, fmt.Errorf("invalid config file: %w", err)
	}
	content, err := os.ReadFile(configFile)
	if os.IsNotExist(err) {
		return fileConfig{}, nil
	}
	if err != nil {
		return fileConfig{}, fmt.Errorf("read config file: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var cfg fileConfig
	if err := decoder.Decode(&cfg); err != nil {
		return fileConfig{}, fmt.Errorf("decode config file %s: %w", configFile, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return fileConfig{}, fmt.Errorf("decode config file %s: multiple JSON values", configFile)
	} else if err != io.EOF {
		return fileConfig{}, fmt.Errorf("decode config file %s: %w", configFile, err)
	}
	return cfg, nil
}

func resolveConfiguredDir(rootDir string, defaultDir string, fileValue *string, label string) (string, error) {
	value := stringValue(defaultDir, fileValue)
	return resolvePath(rootDir, defaultDir, value, label)
}

func loadCodexHome(rootDir string, fileValue *string) (string, error) {
	if fileValue != nil && *fileValue != "" {
		return resolvePath(rootDir, "", *fileValue, "CODEX_HOME")
	}
	envValue := os.Getenv(envCodexHome)
	if envValue != "" {
		return resolvePath(rootDir, "", envValue, "CODEX_HOME")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(homeDir, ".codex"), nil
}

func resolvePath(rootDir string, defaultValue string, value string, label string) (string, error) {
	if value == "" {
		value = defaultValue
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(rootDir, value)
	}
	absPath, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("invalid %s: %w", label, err)
	}
	return absPath, nil
}

func stringValue(defaultValue string, fileValue *string) string {
	if fileValue != nil {
		return *fileValue
	}
	return defaultValue
}

func projectRoot(startDir string) string {
	for dir := startDir; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return startDir
		}
	}
}
