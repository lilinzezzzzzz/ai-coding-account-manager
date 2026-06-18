// Package app 负责应用依赖装配和进程生命周期编排。
package app

import (
	"context"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

type services struct {
	Provider  service.ProviderService
	Account   *service.AccountService
	LoginTask *service.LoginTaskService
}

// Run 加载配置、装配依赖并启动应用。
func Run(args []string) error {
	logger := setupLogger()

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
