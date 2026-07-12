package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	// ResolveBin 返回当前可用的 codex 二进制路径。
	// 在启动 app-server 失败时用于重新发现 runtime 路径（例如 VS Code 扩展自动更新后）。
	ResolveBin func(context.Context) (string, error)
}

// Provider 实现 OpenAI Codex 账号 provider。
type Provider struct {
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

// ImportCurrentAccount 读取活动 CODEX_HOME 登录态，并把 auth.json 导入账号隔离目录。
func (providerImpl *Provider) ImportCurrentAccount(ctx context.Context) (*entity.Account, error) {
	activeDir := providerImpl.credentials.ActiveCodexDir()
	account, err := providerImpl.ReadAccountFromCodexDir(ctx, activeDir)
	if err != nil {
		return nil, err
	}
	if err := providerImpl.ImportAccountAuthFromCodexDir(ctx, *account, activeDir); err != nil {
		return nil, err
	}
	return account, nil
}

// ReadAccountFromCodexDir 使用指定 CODEX_HOME 读取 Codex 账号元数据。
func (providerImpl *Provider) ReadAccountFromCodexDir(ctx context.Context, codexHome string) (*entity.Account, error) {
	client, err := providerImpl.startClient(ctx, codexHome)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = client.Close(context.Background())
	}()
	return providerImpl.readAccount(ctx, client, false)
}

// ImportAccountAuthFromCodexDir 把指定 CODEX_HOME 的 auth.json 导入账号隔离目录。
func (providerImpl *Provider) ImportAccountAuthFromCodexDir(ctx context.Context, account entity.Account, codexHome string) error {
	if err := providerImpl.credentials.ImportFromCodexDir(ctx, providerID, account.StorageID, codexHome); err != nil {
		return entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "导入 Codex auth.json 失败", err)
	}
	return nil
}

// ImportAccountAuthJSONAndRefresh 导入 auth.json，识别账号并刷新 usage。
func (providerImpl *Provider) ImportAccountAuthJSONAndRefresh(ctx context.Context, authJSON []byte) (*entity.Account, *entity.UsageSnapshot, error) {
	runtimeDir, err := os.MkdirTemp("", "ai-coding-account-manager-codex-home-*")
	if err != nil {
		return nil, nil, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "创建 Codex 临时运行目录失败", err)
	}
	defer func() {
		_ = os.RemoveAll(runtimeDir)
	}()
	if err := providerImpl.credentials.ImportAuthJSONToCodexDir(ctx, runtimeDir, authJSON); err != nil {
		return nil, nil, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "准备 Codex 临时 auth.json 失败", err)
	}

	refreshedAccount, snapshot, err := providerImpl.refreshCodexHome(ctx, runtimeDir, "")
	if err != nil {
		return nil, nil, err
	}
	if err := providerImpl.credentials.ImportFromCodexDir(ctx, providerID, refreshedAccount.StorageID, runtimeDir); err != nil {
		return nil, nil, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "保存刷新后的 Codex auth.json 失败", err)
	}
	return refreshedAccount, snapshot, nil
}

// RefreshAccount 使用账号隔离 CODEX_HOME 刷新 usage。
func (providerImpl *Provider) RefreshAccount(ctx context.Context, account entity.Account) (*entity.UsageSnapshot, error) {
	_, snapshot, err := providerImpl.RefreshAccountWithMetadata(ctx, account)
	return snapshot, err
}

// RefreshAccountWithMetadata 使用账号隔离 CODEX_HOME 刷新 usage，并返回最新账号元数据。
func (providerImpl *Provider) RefreshAccountWithMetadata(ctx context.Context, account entity.Account) (*entity.Account, *entity.UsageSnapshot, error) {
	if err := providerImpl.credentials.ValidateAccount(ctx, providerID, account.StorageID); err != nil {
		return nil, nil, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "账号未导入 Codex auth.json，请先登录添加账号", err)
	}
	runtimeDir, err := os.MkdirTemp("", "ai-coding-account-manager-codex-home-*")
	if err != nil {
		return nil, nil, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "创建 Codex 临时运行目录失败", err)
	}
	defer func() {
		_ = os.RemoveAll(runtimeDir)
	}()
	if err := providerImpl.credentials.ExportToCodexDir(ctx, providerID, account.StorageID, runtimeDir); err != nil {
		return nil, nil, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "准备 Codex 临时 auth.json 失败", err)
	}

	refreshedAccount, snapshot, err := providerImpl.refreshCodexHome(ctx, runtimeDir, account.AccountID)
	if err != nil {
		return nil, nil, err
	}
	if err := providerImpl.credentials.ImportFromCodexDir(ctx, providerID, account.StorageID, runtimeDir); err != nil {
		return nil, nil, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "保存刷新后的 Codex auth.json 失败", err)
	}
	return refreshedAccount, snapshot, nil
}

// ResetAccountRateLimit 消耗一次 reset credit，并返回重置后的 usage。
func (providerImpl *Provider) ResetAccountRateLimit(ctx context.Context, account entity.Account, idempotencyKey string) (provider.RateLimitResetResult, error) {
	if err := providerImpl.credentials.ValidateAccount(ctx, providerID, account.StorageID); err != nil {
		return provider.RateLimitResetResult{}, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "账号未导入 Codex auth.json，请先登录添加账号", err)
	}
	runtimeDir, err := os.MkdirTemp("", "ai-coding-account-manager-codex-home-*")
	if err != nil {
		return provider.RateLimitResetResult{}, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "创建 Codex 临时运行目录失败", err)
	}
	defer func() {
		_ = os.RemoveAll(runtimeDir)
	}()
	if err := providerImpl.credentials.ExportToCodexDir(ctx, providerID, account.StorageID, runtimeDir); err != nil {
		return provider.RateLimitResetResult{}, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "准备 Codex 临时 auth.json 失败", err)
	}

	client, err := providerImpl.startClient(ctx, runtimeDir)
	if err != nil {
		return provider.RateLimitResetResult{}, err
	}
	refreshedAccount, err := providerImpl.readAccount(ctx, client, false)
	if err != nil {
		_ = client.Close(context.Background())
		return provider.RateLimitResetResult{}, err
	}
	if refreshedAccount.AccountID != account.AccountID {
		_ = client.Close(context.Background())
		return provider.RateLimitResetResult{}, entity.NewAppErrorWithMessage(entity.ErrorCodeConflict, "auth.json 对应账号与当前账号不一致")
	}

	var consumeResponse consumeRateLimitResetCreditResponse
	if err := client.Call(ctx, "account/rateLimitResetCredit/consume", consumeRateLimitResetCreditParams{IdempotencyKey: idempotencyKey}, &consumeResponse); err != nil {
		_ = client.Close(context.Background())
		return provider.RateLimitResetResult{}, mapAppServerError("consume Codex rate limit reset credit failed", err)
	}
	if !validRateLimitResetOutcome(consumeResponse.Outcome) {
		_ = client.Close(context.Background())
		return provider.RateLimitResetResult{}, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "Codex 返回了未知的额度重置结果")
	}

	var rateLimitsResponse rateLimitsReadResponse
	if err := client.Call(ctx, "account/rateLimits/read", map[string]any{}, &rateLimitsResponse); err != nil {
		_ = client.Close(context.Background())
		return provider.RateLimitResetResult{}, mapAppServerError("read Codex rate limits after reset failed", err)
	}
	if rateLimitsResponse.RateLimits.PlanType != nil {
		planType := strings.TrimSpace(*rateLimitsResponse.RateLimits.PlanType)
		if planType != "" {
			refreshedAccount.PlanType = &planType
		}
	}
	if err := client.Close(context.Background()); err != nil {
		return provider.RateLimitResetResult{}, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "关闭 Codex app-server 失败", err)
	}
	snapshot, err := mapUsageSnapshot(*refreshedAccount, rateLimitsResponse, providerImpl.now().UTC().UnixMilli())
	if err != nil {
		return provider.RateLimitResetResult{}, err
	}
	if err := providerImpl.credentials.ImportFromCodexDir(ctx, providerID, account.StorageID, runtimeDir); err != nil {
		return provider.RateLimitResetResult{}, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "保存刷新后的 Codex auth.json 失败", err)
	}
	return provider.RateLimitResetResult{
		Outcome: consumeResponse.Outcome,
		Account: refreshedAccount,
		Usage:   snapshot,
	}, nil
}

func (providerImpl *Provider) refreshCodexHome(ctx context.Context, codexHome string, expectedAccountID string) (*entity.Account, *entity.UsageSnapshot, error) {
	client, err := providerImpl.startClient(ctx, codexHome)
	if err != nil {
		return nil, nil, err
	}

	refreshedAccount, err := providerImpl.readAccount(ctx, client, false)
	if err != nil {
		_ = client.Close(context.Background())
		return nil, nil, err
	}
	if expectedAccountID != "" && refreshedAccount.AccountID != expectedAccountID {
		_ = client.Close(context.Background())
		return nil, nil, entity.NewAppErrorWithMessage(entity.ErrorCodeConflict, "auth.json 对应账号与当前账号不一致")
	}

	var response rateLimitsReadResponse
	if err := client.Call(ctx, "account/rateLimits/read", map[string]any{}, &response); err != nil {
		_ = client.Close(context.Background())
		return nil, nil, mapAppServerError("read Codex rate limits failed", err)
	}
	if response.RateLimits.PlanType != nil {
		planType := strings.TrimSpace(*response.RateLimits.PlanType)
		if planType != "" {
			refreshedAccount.PlanType = &planType
		}
	}
	if err := client.Close(context.Background()); err != nil {
		return nil, nil, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "关闭 Codex app-server 失败", err)
	}
	snapshot, err := mapUsageSnapshot(*refreshedAccount, response, providerImpl.now().UTC().UnixMilli())
	if err != nil {
		return nil, nil, err
	}
	return refreshedAccount, snapshot, nil
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
		// 二进制路径可能已过期（例如 VS Code 扩展自动更新），尝试重新解析。
		if providerImpl.resolveBin != nil {
			if newBin, resolveErr := providerImpl.resolveBin(ctx); resolveErr == nil && newBin != providerImpl.bin {
				providerImpl.bin = newBin
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

func (providerImpl *Provider) readAccount(ctx context.Context, client appServerClient, refreshToken bool) (*entity.Account, error) {
	var response accountReadResponse
	err := client.Call(ctx, "account/read", accountReadParams{RefreshToken: refreshToken}, &response)
	if err != nil {
		return nil, mapAppServerError("read Codex account", err)
	}
	return mapAccount(response)
}

func mapAccount(response accountReadResponse) (*entity.Account, error) {
	if response.Account == nil {
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
	if upstream, ok := appserver.UpstreamErrorFrom(err); ok {
		return entity.WrapAppErrorWithUpstreamError(entity.ErrorCodeUnavailable, upstream.Code, upstream.Message, err)
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
	RateLimitResetCredits *rateLimitResetCreditsSummary `json:"rateLimitResetCredits"`
	RateLimits            rateLimitSnapshot             `json:"rateLimits"`
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

type rateLimitResetCreditsSummary struct {
	AvailableCount int64 `json:"availableCount"`
}

type consumeRateLimitResetCreditParams struct {
	IdempotencyKey string `json:"idempotencyKey"`
}

type consumeRateLimitResetCreditResponse struct {
	Outcome provider.RateLimitResetOutcome `json:"outcome"`
}

func validRateLimitResetOutcome(outcome provider.RateLimitResetOutcome) bool {
	switch outcome {
	case provider.RateLimitResetOutcomeReset,
		provider.RateLimitResetOutcomeNothingToReset,
		provider.RateLimitResetOutcomeNoCredit,
		provider.RateLimitResetOutcomeAlreadyRedeemed:
		return true
	default:
		return false
	}
}
