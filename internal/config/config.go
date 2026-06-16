package config

import (
	"fmt"
	"net"
	"os"
)

const (
	defaultBindAddr = "127.0.0.1:43127"
	envBindAddr     = "AI_CODING_ACCOUNT_MANAGER_BIND_ADDR"
)

// Config 保存应用启动配置。
type Config struct {
	BindAddr string
}

// Load 从环境变量读取配置，并填充本地运行默认值。
func Load() (Config, error) {
	bindAddr := os.Getenv(envBindAddr)
	if bindAddr == "" {
		bindAddr = defaultBindAddr
	}
	if err := validateBindAddr(bindAddr); err != nil {
		return Config{}, err
	}
	return Config{BindAddr: bindAddr}, nil
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
