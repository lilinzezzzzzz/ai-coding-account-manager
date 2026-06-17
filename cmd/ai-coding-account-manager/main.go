package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/config"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httpserver"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/credentials"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/codex"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/fake"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

func main() {
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// 进程入口先建立默认 logger，后续启动失败也能输出结构化错误。
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// 配置读取集中放在 config 包，main 只负责装配启动依赖。
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	appDB, err := database.Open(context.Background(), database.Config{
		Path: filepath.Join(cfg.DataDir, "state.db"),
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if err := appDB.Close(); err != nil {
			logger.Error("close database failed", "error", err)
		}
	}()

	providerRegistry := provider.NewRegistry()
	if err := registerProviders(context.Background(), providerRegistry, cfg); err != nil {
		return err
	}
	daos := dao.NewDAOs(appDB.GORM())
	accountService := service.NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, providerRegistry)
	providerService := service.NewProviderService(providerRegistry)
	securityManager, err := security.NewManager(security.Config{BindAddr: cfg.BindAddr})
	if err != nil {
		return fmt.Errorf("create security manager: %w", err)
	}

	// httpserver.NewServer 组装 http.Server 和应用 HTTP handler。
	httpServer, err := httpserver.NewServer(httpserver.Config{
		Addr:            cfg.BindAddr,
		SecurityManager: securityManager,
		ProviderService: providerService,
		AccountService:  accountService,
	})
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	// 先显式 Listen，再启动 Serve，便于拿到真实监听地址并尽早暴露端口冲突。
	listener, err := net.Listen("tcp", cfg.BindAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.BindAddr, err)
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

	// Shutdown 会停止接收新连接，并等待已有请求在超时时间内结束。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}
	if err := providerService.Close(shutdownCtx); err != nil {
		logger.Warn("close providers failed", "error", err)
	}

	// 等 Serve goroutine 确认退出，避免 main 在后台 goroutine 未收尾时直接返回。
	if err := <-serveErr; err != nil {
		return err
	}
	logger.Info("server stopped")
	return nil
}

func registerProviders(ctx context.Context, providerRegistry *provider.Registry, cfg config.Config) error {
	if strings.EqualFold(cfg.ProviderMode, "fake") {
		if err := providerRegistry.Register(ctx, newDefaultFakeProvider()); err != nil {
			return fmt.Errorf("register fake provider: %w", err)
		}
		return nil
	}

	credentialStore, err := credentials.NewStore(credentials.Config{
		RootDir:        filepath.Join(cfg.DataDir, "credentials"),
		ActiveCodexDir: cfg.CodexHome,
	})
	if err != nil {
		return fmt.Errorf("create credentials store: %w", err)
	}
	codexProvider, err := codex.New(codex.Config{
		Bin:         cfg.CodexBin,
		CodexHome:   cfg.CodexHome,
		Credentials: credentialStore,
	})
	if err != nil {
		return fmt.Errorf("create codex provider: %w", err)
	}
	if err := providerRegistry.Register(ctx, codexProvider); err != nil {
		return fmt.Errorf("register codex provider: %w", err)
	}
	return nil
}

func newDefaultFakeProvider() provider.Provider {
	usedPercent := 12.5
	account := entity.Account{
		ProviderID: "codex",
		AccountID:  "fake-codex-account",
		Label:      "Fake Codex Account",
		Email:      stringPtr("fake@example.local"),
		PlanType:   stringPtr("fake"),
	}
	return fake.New(fake.Config{
		ID:          "codex",
		DisplayName: "OpenAI Codex",
		Accounts: []fake.AccountState{{
			Account:   account,
			IsCurrent: true,
			Usage: entity.UsageSnapshot{
				Status:      entity.UsageStatusReady,
				UsedPercent: &usedPercent,
				RefreshedAt: time.Now().UTC().UnixMilli(),
			},
		}},
	})
}

func stringPtr(value string) *string {
	return &value
}
