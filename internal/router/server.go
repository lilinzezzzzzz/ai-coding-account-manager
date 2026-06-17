package router

import (
	"fmt"
	"net/http"
	"time"
)

// ServerConfig 保存 HTTP server 构造参数。
type ServerConfig struct {
	Addr string
}

// NewServer 显式构造带超时限制的 http.Server。
func NewServer(cfg ServerConfig) (*http.Server, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("server address is required")
	}

	handler := NewHandler()

	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}, nil
}
