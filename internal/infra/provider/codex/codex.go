package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
}

// Provider 实现 OpenAI Codex 账号 provider。
type Provider struct {
	bin         string
	credentials *credentials.Store
	newClient   ClientFactory
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
		now:         now,
	}, nil
}

// Describe 返回 Codex provider 能力。
func (providerImpl *Provider) Describe(context.Context) (provider.Description, error) {
	return provider.Description{
		ID:          providerID,
		DisplayName: providerDisplayName,
		Capabilities: provider.Capabilities{
			CanRefreshUsage:                   true,
			CanActivateAccount:                true,
			RequiresClientReloadAfterActivate: true,
		},
		Status: provider.StatusAvailable,
	}, nil
}

// RefreshAccount 使用账号隔离 CODEX_HOME 刷新 usage。
func (providerImpl *Provider) RefreshAccount(ctx context.Context, account entity.Account) (*entity.UsageSnapshot, error) {
	accountDir, err := providerImpl.credentials.AccountCodexDir(providerID, account.StorageID)
	if err != nil {
		return nil, entity.WrapAppError(entity.ErrorCodeUnavailable, err)
	}
	client, err := providerImpl.startClient(ctx, accountDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = client.Close(context.Background())
	}()

	if _, err := providerImpl.readAccount(ctx, client, true); err != nil {
		return nil, err
	}

	var response rateLimitsReadResponse
	if err := client.Call(ctx, "account/rateLimits/read", map[string]any{}, &response); err != nil {
		return nil, mapAppServerError("read Codex rate limits", err)
	}
	return mapUsageSnapshot(account, response, providerImpl.now().UTC().UnixMilli())
}

// ActivateAccount 替换活动 CODEX_HOME 的 auth.json。
func (providerImpl *Provider) ActivateAccount(ctx context.Context, account entity.Account) error {
	return providerImpl.credentials.ActivateAccount(ctx, providerID, account.StorageID)
}

// RemoveAccountData 删除账号隔离凭据。
func (providerImpl *Provider) RemoveAccountData(ctx context.Context, account entity.Account) error {
	return providerImpl.credentials.RemoveAccount(ctx, providerID, account.StorageID)
}

// Close 关闭 provider。
func (providerImpl *Provider) Close(ctx context.Context) error {
	return ctx.Err()
}

func (providerImpl *Provider) startClient(ctx context.Context, codexHome string) (appServerClient, error) {
	client, err := providerImpl.newClient(ctx, appserver.Config{
		Bin:       providerImpl.bin,
		CodexHome: codexHome,
	})
	if err != nil {
		return nil, mapAppServerError("start Codex app-server", err)
	}
	return client, nil
}

func (providerImpl *Provider) readAccount(ctx context.Context, client appServerClient, refreshToken bool) (*entity.Account, error) {
	var response accountReadResponse
	err := client.Call(ctx, "account/read", accountReadParams{RefreshToken: refreshToken}, &response)
	if err != nil {
		return nil, mapAppServerError("read Codex account", err)
	}
	return mapAccount(response)
}

func mapAccount(response accountReadResponse) (*entity.Account, error) {
	if response.Account == nil || response.RequiresOpenaiAuth {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "Codex 账号未登录")
	}
	if response.Account.Type != "chatgpt" {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeUnsupported, "当前 Codex 账号类型不支持")
	}
	email := strings.TrimSpace(response.Account.Email)
	if email == "" {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "Codex 账号缺少 email")
	}
	planType := strings.TrimSpace(response.Account.PlanType)
	accountID := entity.AccountIDFromEmail(email)
	account := entity.Account{
		ProviderID: providerID,
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount(providerID, accountID),
		Label:      email,
		Email:      &email,
	}
	if planType != "" {
		account.PlanType = &planType
	}
	return &account, nil
}

func mapUsageSnapshot(account entity.Account, response rateLimitsReadResponse, refreshedAt int64) (*entity.UsageSnapshot, error) {
	status := entity.UsageStatusReady
	if response.RateLimits.RateLimitReachedType != nil {
		status = entity.UsageStatusRateLimitReached
	}

	var usedPercent *float64
	var resetsAt *int64
	if response.RateLimits.Primary != nil {
		usedPercent = response.RateLimits.Primary.UsedPercent
		resetsAt = response.RateLimits.Primary.ResetsAt
	}
	snapshotPayload, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("encode Codex rate limit snapshot: %w", err)
	}
	snapshotJSON := string(snapshotPayload)
	return &entity.UsageSnapshot{
		ProviderID:   account.ProviderID,
		AccountID:    account.AccountID,
		Status:       status,
		UsedPercent:  usedPercent,
		ResetsAt:     resetsAt,
		SnapshotJSON: &snapshotJSON,
		RefreshedAt:  refreshedAt,
	}, nil
}

func mapAppServerError(message string, err error) error {
	if err == nil {
		return nil
	}
	return entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, message, err)
}

type accountReadParams struct {
	RefreshToken bool `json:"refreshToken"`
}

type accountReadResponse struct {
	Account            *codexAccount `json:"account"`
	RequiresOpenaiAuth bool          `json:"requiresOpenaiAuth"`
}

type codexAccount struct {
	Type     string `json:"type"`
	Email    string `json:"email"`
	PlanType string `json:"planType"`
}

type rateLimitsReadResponse struct {
	RateLimits rateLimitSnapshot `json:"rateLimits"`
}

type rateLimitSnapshot struct {
	Primary              *rateLimitWindow `json:"primary"`
	Secondary            *rateLimitWindow `json:"secondary"`
	PlanType             *string          `json:"planType"`
	RateLimitReachedType *string          `json:"rateLimitReachedType"`
}

type rateLimitWindow struct {
	UsedPercent        *float64 `json:"usedPercent"`
	ResetsAt           *int64   `json:"resetsAt"`
	WindowDurationMins *int64   `json:"windowDurationMins"`
}
