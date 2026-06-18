package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/config"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/router"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

func newHTTPServer(cfg config.Config, appServices services) (*http.Server, error) {
	if cfg.BindAddr == "" {
		return nil, fmt.Errorf("server address is required")
	}
	securityManager, err := security.NewManager(security.Config{BindAddr: cfg.BindAddr})
	if err != nil {
		return nil, fmt.Errorf("create security manager: %w", err)
	}

	return &http.Server{
		Addr: cfg.BindAddr,
		Handler: router.NewHandler(router.Config{
			SecurityManager:  securityManager,
			ProviderService:  appServices.Provider,
			AccountService:   appServices.Account,
			LoginTaskService: appServices.LoginTask,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}, nil
}

func serveHTTP(httpServer *http.Server, bindAddr string, providerService service.ProviderService, logger *slog.Logger) error {
	// 先显式 Listen，再启动 Serve，便于拿到真实监听地址并尽早暴露端口冲突。
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", bindAddr, err)
	}

	// SIGINT/SIGTERM 统一转换为 context 取消，作为主协程退出信号。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Serve 在后台阻塞运行；buffer=1 避免主协程先进入 shutdown 时 goroutine 卡在发送错误。
	serveErr := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			serveErr <- nil
			return
		}
		serveErr <- err
	}()

	baseURL := "http://" + listener.Addr().String() + "/"
	logger.Info("server started", "addr", listener.Addr().String(), "url", baseURL)

	// 任一分支先发生都会结束主等待：服务异常退出直接返回，收到系统信号则进入优雅关闭。
	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	if err := shutdownHTTPServer(httpServer, providerService, logger); err != nil {
		return err
	}

	// 等 Serve goroutine 确认退出，避免进程在后台 goroutine 未收尾时直接返回。
	if err := <-serveErr; err != nil {
		return err
	}
	logger.Info("server stopped")
	return nil
}

func shutdownHTTPServer(httpServer *http.Server, providerService service.ProviderService, logger *slog.Logger) error {
	// Shutdown 会停止接收新连接，并等待已有请求在超时时间内结束。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}
	if err := providerService.Close(shutdownCtx); err != nil {
		logger.Warn("close providers failed", "error", err)
	}
	return nil
}
