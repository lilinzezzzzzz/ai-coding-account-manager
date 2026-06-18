package httpserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/router"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

// Config 保存 HTTP server 构造参数。
type Config struct {
	Addr             string
	SecurityManager  *security.Manager
	ProviderService  service.ProviderService
	AccountService   *service.AccountService
	LoginTaskService *service.LoginTaskService
}

// NewServer 显式构造带超时限制的 http.Server。
func NewServer(cfg Config) (*http.Server, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("server address is required")
	}
	securityManager := cfg.SecurityManager
	if securityManager == nil {
		var err error
		securityManager, err = security.NewManager(security.Config{BindAddr: cfg.Addr})
		if err != nil {
			return nil, fmt.Errorf("create security manager: %w", err)
		}
	}

	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           router.NewHandler(router.Config{SecurityManager: securityManager, ProviderService: cfg.ProviderService, AccountService: cfg.AccountService, LoginTaskService: cfg.LoginTaskService}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}, nil
}
