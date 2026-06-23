package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/config"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/codexruntime"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/credentials"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/codex"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/fake"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

func registerProviders(ctx context.Context, providerRegistry *provider.Registry, cfg config.Config, runtimeResolver *codexruntime.Resolver) (*codex.Provider, error) {
	if strings.EqualFold(cfg.ProviderMode, "fake") {
		if err := providerRegistry.Register(ctx, newDefaultFakeProvider()); err != nil {
			return nil, fmt.Errorf("register fake provider: %w", err)
		}
		return nil, nil
	}

	runtime, err := runtimeResolver.Resolve(ctx)
	if err != nil {
		code := entity.ErrorCodeUnavailable
		if registerErr := providerRegistry.RegisterUnavailable(provider.Description{
			ID:          "codex",
			DisplayName: "OpenAI Codex",
			ErrorCode:   &code,
		}, err); registerErr != nil {
			return nil, fmt.Errorf("register unavailable codex provider: %w", registerErr)
		}
		return nil, nil
	}

	credentialStore, err := credentials.NewStore(credentials.Config{
		RootDir:        cfg.CredentialsDir,
		ActiveCodexDir: cfg.CodexHome,
	})
	if err != nil {
		return nil, fmt.Errorf("create credentials store: %w", err)
	}
	codexProvider, err := codex.New(codex.Config{
		Bin:         runtime.Path,
		Credentials: credentialStore,
		ResolveBin: func(ctx context.Context) (string, error) {
			rt, err := runtimeResolver.Resolve(ctx)
			if err != nil {
				return "", err
			}
			return rt.Path, nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create codex provider: %w", err)
	}
	if err := providerRegistry.Register(ctx, codexProvider); err != nil {
		return nil, fmt.Errorf("register codex provider: %w", err)
	}
	return codexProvider, nil
}

func newDefaultFakeProvider() provider.Provider {
	usedPercent := 12.5
	account := entity.Account{
		ProviderID: "codex",
		AccountID:  "fake-codex-account",
		StorageID:  entity.StorageIDForAccount("codex", "fake-codex-account"),
		Label:      "Fake Codex Account",
		Email:      stringPtr("fake@example.local"),
		PlanType:   stringPtr("fake"),
	}
	return fake.New(fake.Config{
		ID:          "codex",
		DisplayName: "OpenAI Codex",
		Accounts: []fake.AccountState{{
			Account: account,
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
