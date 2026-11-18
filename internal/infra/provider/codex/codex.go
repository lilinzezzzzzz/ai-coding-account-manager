package codex

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/appserver"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/credentials"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

const (
	providerID          = "codex"
	providerDisplayName = "OpenAI Codex"
)

type appServerClient interface {
	Call(context.Context, string, any, any) error
	Close(context.Context) error
}

// ClientFactory 创建 app-server client，测试可替换为 fake 实现。
type ClientFactory func(context.Context, appserver.Config) (appServerClient, error)

// Config 保存 Codex provider 依赖。
type Config struct {
	Bin           string
	Credentials   *credentials.Store
	ClientFactory ClientFactory
	Now           func() time.Time
	// ResolveBin 返回当前可用的 codex 二进制路径。
	// 在启动 app-server 失败时用于重新发现 runtime 路径（例如 CLI 卸载或桌面应用更新后）。
	ResolveBin func(context.Context) (string, error)
}

// Provider 实现 OpenAI Codex 账号 provider。
type Provider struct {
	runtimeMu   sync.Mutex
	bin         string
	credentials *credentials.Store
	newClient   ClientFactory
	resolveBin  func(context.Context) (string, error)
	now         func() time.Time
}

// New 创建 Codex provider。
func New(cfg Config) (*Provider, error) {
	if cfg.Credentials == nil {
		return nil, fmt.Errorf("credentials store is required")
	}
	newClient := cfg.ClientFactory
	if newClient == nil {
		newClient = func(ctx context.Context, cfg appserver.Config) (appServerClient, error) {
			return appserver.Start(ctx, cfg)
		}
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Provider{
		bin:         cfg.Bin,
		credentials: cfg.Credentials,
		newClient:   newClient,
		resolveBin:  cfg.ResolveBin,
		now:         now,
	}, nil
}

// Describe 返回 Codex provider 能力。
func (providerImpl *Provider) Describe(context.Context) (provider.Description, error) {
	description := provider.Description{
		ID:          providerID,
		DisplayName: providerDisplayName,
		Capabilities: provider.Capabilities{
			CanExportAuthJSON:                 true,
			CanRefreshUsage:                   true,
			CanActivateAccount:                true,
			RequiresClientReloadAfterActivate: true,
		},
		Status: provider.StatusAvailable,
	}
	if providerImpl.runtimeBin() == "" {
		description.Status = provider.StatusUnavailable
		code := entity.ErrorCodeUnavailable
		description.ErrorCode = &code
	}
	return description, nil
}

// Close 关闭 provider。
func (providerImpl *Provider) Close(ctx context.Context) error {
	return ctx.Err()
}

func (providerImpl *Provider) startClient(ctx context.Context, codexHome string) (appServerClient, error) {
	bin := providerImpl.runtimeBin()
	if bin == "" {
		if providerImpl.resolveBin == nil {
			return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "Codex runtime 不可用，请安装 Codex CLI 或配置 codexBin 后重试")
		}
		resolvedBin, err := providerImpl.resolveBin(ctx)
		if err != nil {
			return nil, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "Codex runtime 不可用，请安装 Codex CLI 或配置 codexBin 后重试", err)
		}
		bin = resolvedBin
		providerImpl.setRuntimeBin(bin)
	}
	client, err := providerImpl.newClient(ctx, appserver.Config{
		Bin:       bin,
		CodexHome: codexHome,
	})
	if err != nil {
		// 二进制路径可能已过期（例如 CLI 卸载或桌面应用更新），尝试重新解析。
		if providerImpl.resolveBin != nil {
			if newBin, resolveErr := providerImpl.resolveBin(ctx); resolveErr == nil && newBin != bin {
				providerImpl.setRuntimeBin(newBin)
				client, err = providerImpl.newClient(ctx, appserver.Config{
					Bin:       newBin,
					CodexHome: codexHome,
				})
				if err == nil {
					return client, nil
				}
			}
		}
		return nil, mapAppServerError("start Codex app-server", err)
	}
	return client, nil
}

// runtimeMu 只保护路径读写，不能覆盖 runtime 探测或 app-server 启动等外部调用。
func (providerImpl *Provider) runtimeBin() string {
	providerImpl.runtimeMu.Lock()
	defer providerImpl.runtimeMu.Unlock()
	return providerImpl.bin
}

func (providerImpl *Provider) setRuntimeBin(bin string) {
	providerImpl.runtimeMu.Lock()
	defer providerImpl.runtimeMu.Unlock()
	providerImpl.bin = bin
}
