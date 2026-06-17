package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Config 保存本地 Web 安全边界配置。
type Config struct {
	BindAddr string
}

// Manager 管理本地 Host/Origin 请求边界。
type Manager struct {
	allowedHosts   map[string]struct{}
	allowedOrigins map[string]struct{}
}

// NewManager 创建本地 Web 安全边界管理器。
func NewManager(cfg Config) (*Manager, error) {
	if cfg.BindAddr == "" {
		return nil, fmt.Errorf("bind address is required")
	}
	host, port, err := net.SplitHostPort(cfg.BindAddr)
	if err != nil {
		return nil, fmt.Errorf("parse bind address: %w", err)
	}
	if port == "" {
		return nil, fmt.Errorf("bind address port is required")
	}

	allowedHosts := map[string]struct{}{}
	addHost := func(name string) {
		if name != "" {
			allowedHosts[net.JoinHostPort(name, port)] = struct{}{}
		}
	}
	addHost(host)
	addHost("127.0.0.1")
	addHost("localhost")

	allowedOrigins := make(map[string]struct{}, len(allowedHosts))
	for allowedHost := range allowedHosts {
		allowedOrigins["http://"+allowedHost] = struct{}{}
	}

	return &Manager{
		allowedHosts:   allowedHosts,
		allowedOrigins: allowedOrigins,
	}, nil
}

// ValidateHost 校验请求 Host 是否属于当前本地服务。
func (manager *Manager) ValidateHost(host string) bool {
	normalized, ok := normalizeHost(host)
	if !ok {
		return false
	}
	_, ok = manager.allowedHosts[normalized]
	return ok
}

// ValidateOrigin 校验写请求 Origin 是否为当前服务 origin。
func (manager *Manager) ValidateOrigin(origin string) bool {
	normalizedOrigin, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	_, ok = manager.allowedOrigins[normalizedOrigin]
	return ok
}

// ValidateOriginForHost 校验 Origin 与本次请求 Host 精确匹配。
func (manager *Manager) ValidateOriginForHost(origin string, host string) bool {
	normalizedOrigin, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	normalizedHost, ok := normalizeHost(host)
	if !ok {
		return false
	}
	return normalizedOrigin == "http://"+normalizedHost
}

func normalizeHost(host string) (string, bool) {
	if strings.TrimSpace(host) != host || host == "" {
		return "", false
	}
	normalizedHost, port, err := net.SplitHostPort(host)
	if err != nil || normalizedHost == "" || port == "" {
		return "", false
	}
	return net.JoinHostPort(normalizedHost, port), true
}

func normalizeOrigin(origin string) (string, bool) {
	if origin == "" {
		return "", false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return "", false
	}
	normalizedHost, ok := normalizeHost(parsed.Host)
	if !ok {
		return "", false
	}
	return "http://" + normalizedHost, true
}
