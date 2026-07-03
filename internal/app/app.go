// Package app 负责应用依赖装配和进程生命周期编排。
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

type services struct {
	Provider  service.ProviderService
	Account   *service.AccountService
	LoginTask *service.LoginTaskService
}

// Run 加载配置、装配依赖并启动应用。
func Run(args []string) (runErr error) {
	logger, logFile, err := setupLogger()
	if err != nil {
		slog.Error("application stopped", "error", err)
		return err
	}
	defer func() {
		if runErr != nil {
			logger.Error("application stopped", "error", runErr)
		}
		if logFile == nil {
			return
		}
		if err := logFile.Close(); err != nil {
			newLogger(os.Stderr).Error("close log file failed", "error", err)
			if runErr == nil {
				runErr = fmt.Errorf("close log file: %w", err)
			}
		}
	}()

	cfg, err := loadConfig(args)
	if err != nil {
		return err
	}
	if err := ensureRuntimeDirs(cfg); err != nil {
		return err
	}

	appDB, err := openDatabase(cfg)
	if err != nil {
		return err
	}
	defer closeDatabase(appDB, logger)

	appServices, err := buildServices(context.Background(), cfg, appDB)
	if err != nil {
		return err
	}

	httpServer, err := newHTTPServer(cfg, appServices)
	if err != nil {
		return err
	}
	return serveHTTP(httpServer, cfg.BindAddr, appServices.Provider, logger)
}
