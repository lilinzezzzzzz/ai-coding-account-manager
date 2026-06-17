package httpserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/router"
)

// Config 保存 HTTP server 构造参数。
type Config struct {
	Addr string
}

// NewServer 显式构造带超时限制的 http.Server。
func NewServer(cfg Config) (*http.Server, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("server address is required")
	}

	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           router.NewHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}, nil
}
