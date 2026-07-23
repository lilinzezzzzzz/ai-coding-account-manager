package app

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/config"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/codexruntime"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/loginrunner"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/codex"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

func buildServices(ctx context.Context, cfg config.Config, appDB *database.DB) (services, error) {
	runtimeResolver := codexruntime.NewResolver(codexruntime.Config{ConfiguredBin: cfg.CodexBin})
	providerRegistry := provider.NewRegistry()
	codexProvider, err := registerProviders(ctx, providerRegistry, cfg, runtimeResolver)
	if err != nil {
		return services{}, err
	}

	daos := dao.NewDAOs(appDB.GORM())
	activation := service.NewAccountActivationCoordinator()
	accountService := service.NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, providerRegistry, activation)
	loginTaskService, err := newLoginTaskService(cfg, daos, appDB, runtimeResolver, codexProvider, activation)
	if err != nil {
		return services{}, err
	}
	providerService := service.NewProviderService(providerRegistry)
	return services{
		Provider:  providerService,
		Account:   accountService,
		LoginTask: loginTaskService,
	}, nil
}

func newLoginTaskService(cfg config.Config, daos dao.DAOs, appDB *database.DB, runtimeResolver *codexruntime.Resolver, codexProvider *codex.Provider, activation *service.AccountActivationCoordinator) (*service.LoginTaskService, error) {
	if strings.EqualFold(cfg.ProviderMode, "fake") || codexProvider == nil {
		return nil, nil
	}
	return service.NewLoginTaskService(service.LoginTaskConfig{
		UnitOfWork: dao.NewUnitOfWork(appDB.GORM()),
		DAOs:       daos,
		Resolver:   runtimeResolver,
		Runner:     loginrunner.Runner{},
		Importer:   codexProvider,
		RootDir:    filepath.Join(cfg.DataDir, "login-tasks"),
		Activation: activation,
	})
}
