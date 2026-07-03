package fake

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

// AccountState 描述 fake provider 中的一个账号及其 usage 状态。
type AccountState struct {
	Account entity.Account
	Usage   entity.UsageSnapshot
}

// Config 保存 fake provider 构造参数。
type Config struct {
	ID            string
	DisplayName   string
	Capabilities  provider.Capabilities
	Accounts      []AccountState
	DescribeError error
	Unavailable   bool
}

// Provider 是可配置的内存 provider 实现。
type Provider struct {
	mu sync.Mutex

	description   provider.Description
	describeError error
	unavailable   bool
	accounts      map[string]entity.Account
	usages        map[string]entity.UsageSnapshot
	authJSONs     map[string]string
	resetKeys     map[string]map[string]struct{}
	closed        bool
}

// New 创建 fake provider。
func New(cfg Config) *Provider {
	id := cfg.ID
	if id == "" {
		id = "fake"
	}
	displayName := cfg.DisplayName
	if displayName == "" {
		displayName = "Fake Provider"
	}
	capabilities := cfg.Capabilities
	if capabilities == (provider.Capabilities{}) {
		capabilities = DefaultCapabilities()
	}

	fakeProvider := &Provider{
		description: provider.Description{
			ID:           id,
			DisplayName:  displayName,
			Capabilities: capabilities,
			Status:       provider.StatusAvailable,
		},
		describeError: cfg.DescribeError,
		unavailable:   cfg.Unavailable,
		accounts:      map[string]entity.Account{},
		usages:        map[string]entity.UsageSnapshot{},
		authJSONs:     map[string]string{},
		resetKeys:     map[string]map[string]struct{}{},
	}
	for _, state := range cfg.Accounts {
		account := state.Account
		if account.ProviderID == "" {
			account.ProviderID = id
		}
		if account.StorageID == "" {
			account.StorageID = entity.StorageIDForAccount(account.ProviderID, account.AccountID)
		}
		key := accountKey(account.ProviderID, account.AccountID)
		fakeProvider.accounts[key] = account

		usage := state.Usage
		if usage.ProviderID == "" {
			usage.ProviderID = account.ProviderID
		}
		if usage.AccountID == "" {
			usage.AccountID = account.AccountID
		}
		if usage.Status == "" {
			usage.Status = entity.UsageStatusReady
		}
		fakeProvider.usages[key] = usage

	}
	return fakeProvider
}

// DefaultCapabilities 返回 fake provider 默认支持的能力。
func DefaultCapabilities() provider.Capabilities {
	return provider.Capabilities{
		CanRefreshUsage:                   true,
		CanActivateAccount:                true,
		RequiresClientReloadAfterActivate: true,
	}
}

// Describe 返回 provider 描述。
func (fakeProvider *Provider) Describe(context.Context) (provider.Description, error) {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if fakeProvider.describeError != nil {
		return provider.Description{}, fakeProvider.describeError
	}
	description := fakeProvider.description
	if fakeProvider.unavailable {
		description.Status = provider.StatusUnavailable
		code := entity.ErrorCodeUnavailable
		description.ErrorCode = &code
	}
	return description, nil
}

// ImportCurrentAccount 返回 fake provider 中稳定排序后的第一个账号。
func (fakeProvider *Provider) ImportCurrentAccount(context.Context) (*entity.Account, error) {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if err := fakeProvider.ensureAvailableLocked(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(fakeProvider.accounts))
	for key := range fakeProvider.accounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	account := fakeProvider.accounts[keys[0]]
	return &account, nil
}

// RefreshAccount 返回账号对应的 fake usage snapshot。
func (fakeProvider *Provider) RefreshAccount(_ context.Context, account entity.Account) (*entity.UsageSnapshot, error) {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if err := fakeProvider.ensureAvailableLocked(); err != nil {
		return nil, err
	}
	if !fakeProvider.description.Capabilities.CanRefreshUsage {
		return nil, provider.Unsupported()
	}
	key := accountKey(account.ProviderID, account.AccountID)
	usage, ok := fakeProvider.usages[key]
	if !ok {
		usedPercent := 0.0
		usage = entity.UsageSnapshot{
			ProviderID:  account.ProviderID,
			AccountID:   account.AccountID,
			Status:      entity.UsageStatusReady,
			UsedPercent: &usedPercent,
			RefreshedAt: time.Now().UTC().UnixMilli(),
		}
		fakeProvider.usages[key] = usage
	}
	if usage.RefreshedAt == 0 {
		usage.RefreshedAt = time.Now().UTC().UnixMilli()
	}
	return &usage, nil
}

// RefreshAccountWithMetadata 返回 fake usage，并尽量返回 provider 侧账号元数据。
func (fakeProvider *Provider) RefreshAccountWithMetadata(ctx context.Context, account entity.Account) (*entity.Account, *entity.UsageSnapshot, error) {
	usage, err := fakeProvider.RefreshAccount(ctx, account)
	if err != nil {
		return nil, nil, err
	}

	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	key := accountKey(account.ProviderID, account.AccountID)
	refreshedAccount := account
	if storedAccount, ok := fakeProvider.accounts[key]; ok {
		refreshedAccount = storedAccount
	}
	return &refreshedAccount, usage, nil
}

// ImportAccountAuthJSONAndRefresh 通过 fake auth.json 识别账号并返回 usage。
func (fakeProvider *Provider) ImportAccountAuthJSONAndRefresh(_ context.Context, authJSON []byte) (*entity.Account, *entity.UsageSnapshot, error) {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if err := fakeProvider.ensureAvailableLocked(); err != nil {
		return nil, nil, err
	}
	if !fakeProvider.description.Capabilities.CanRefreshUsage {
		return nil, nil, provider.Unsupported()
	}
	var value map[string]any
	if err := json.Unmarshal(authJSON, &value); err != nil || len(value) == 0 {
		return nil, nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "auth.json 无效")
	}
	if errorCode := stringField(value, "refreshErrorCode"); errorCode != "" {
		return nil, nil, entity.NewAppError(entity.ErrorCode(errorCode))
	}

	email := stringField(value, "email")
	accountID := stringField(value, "accountId")
	if accountID == "" && email != "" {
		accountID = entity.AccountIDFromEmail(email)
	}
	if accountID == "" {
		return nil, nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "auth.json 缺少账号信息")
	}

	account := entity.Account{
		ProviderID: fakeProvider.description.ID,
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount(fakeProvider.description.ID, accountID),
		Label:      accountID,
	}
	if email != "" {
		account.Label = email
		account.Email = &email
	}
	if planType := stringField(value, "planType"); planType != "" {
		account.PlanType = &planType
	}

	key := accountKey(account.ProviderID, account.AccountID)
	fakeProvider.accounts[key] = account
	fakeProvider.authJSONs[key] = string(authJSON)

	usage := entity.UsageSnapshot{
		ProviderID:  account.ProviderID,
		AccountID:   account.AccountID,
		Status:      entity.UsageStatusReady,
		RefreshedAt: time.Now().UTC().UnixMilli(),
	}
	if usageStatus := stringField(value, "usageStatus"); usageStatus != "" {
		usage.Status = entity.UsageStatus(usageStatus)
	}
	fakeProvider.usages[key] = usage
	return &account, &usage, nil
}

// ActivateAccount 校验目标账号可被激活。
func (fakeProvider *Provider) ActivateAccount(_ context.Context, account entity.Account) error {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if err := fakeProvider.ensureAvailableLocked(); err != nil {
		return err
	}
	if !fakeProvider.description.Capabilities.CanActivateAccount {
		return provider.Unsupported()
	}
	key := accountKey(account.ProviderID, account.AccountID)
	if _, ok := fakeProvider.accounts[key]; !ok {
		return entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return nil
}

// ResetAccountRateLimit 模拟消费一次 reset credit，并更新 usage snapshot。
func (fakeProvider *Provider) ResetAccountRateLimit(_ context.Context, account entity.Account, idempotencyKey string) (provider.RateLimitResetResult, error) {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if err := fakeProvider.ensureAvailableLocked(); err != nil {
		return provider.RateLimitResetResult{}, err
	}
	key := accountKey(account.ProviderID, account.AccountID)
	storedAccount, ok := fakeProvider.accounts[key]
	if !ok {
		return provider.RateLimitResetResult{}, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	usage, ok := fakeProvider.usages[key]
	if !ok || usage.SnapshotJSON == nil {
		return provider.RateLimitResetResult{}, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "fake usage snapshot 不可用")
	}
	if _, redeemed := fakeProvider.resetKeys[key][idempotencyKey]; redeemed {
		return provider.RateLimitResetResult{
			Outcome: provider.RateLimitResetOutcomeAlreadyRedeemed,
			Account: &storedAccount,
			Usage:   &usage,
		}, nil
	}

	var snapshot fakeRateLimitSnapshot
	if err := json.Unmarshal([]byte(*usage.SnapshotJSON), &snapshot); err != nil {
		return provider.RateLimitResetResult{}, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "解析 fake usage snapshot 失败", err)
	}
	outcome := provider.RateLimitResetOutcomeNothingToReset
	if snapshot.RateLimitResetCredits == nil || snapshot.RateLimitResetCredits.AvailableCount <= 0 {
		outcome = provider.RateLimitResetOutcomeNoCredit
	} else if resetFakeRateLimitWindows(&snapshot.RateLimits) {
		snapshot.RateLimitResetCredits.AvailableCount--
		outcome = provider.RateLimitResetOutcomeReset
		if fakeProvider.resetKeys[key] == nil {
			fakeProvider.resetKeys[key] = map[string]struct{}{}
		}
		fakeProvider.resetKeys[key][idempotencyKey] = struct{}{}
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return provider.RateLimitResetResult{}, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "编码 fake usage snapshot 失败", err)
	}
	snapshotJSON := string(payload)
	usage.SnapshotJSON = &snapshotJSON
	usage.Status = entity.UsageStatusReady
	usage.RefreshedAt = time.Now().UTC().UnixMilli()
	if snapshot.RateLimits.Primary != nil {
		usage.UsedPercent = snapshot.RateLimits.Primary.UsedPercent
	}
	fakeProvider.usages[key] = usage
	return provider.RateLimitResetResult{
		Outcome: outcome,
		Account: &storedAccount,
		Usage:   &usage,
	}, nil
}

// RemoveAccountData 删除 fake 账号数据。
func (fakeProvider *Provider) RemoveAccountData(_ context.Context, account entity.Account) error {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if err := fakeProvider.ensureAvailableLocked(); err != nil {
		return err
	}
	key := accountKey(account.ProviderID, account.AccountID)
	if _, ok := fakeProvider.accounts[key]; !ok {
		return entity.NewAppError(entity.ErrorCodeNotFound)
	}
	delete(fakeProvider.accounts, key)
	delete(fakeProvider.usages, key)
	return nil
}

// Close 标记 fake provider 已关闭。
func (fakeProvider *Provider) Close(context.Context) error {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	fakeProvider.closed = true
	return nil
}

func (fakeProvider *Provider) ensureAvailableLocked() error {
	if fakeProvider.closed {
		return entity.NewAppError(entity.ErrorCodeUnavailable)
	}
	if fakeProvider.unavailable {
		return entity.NewAppError(entity.ErrorCodeUnavailable)
	}
	return nil
}

func accountKey(providerID string, accountID string) string {
	return providerID + "\x00" + accountID
}

func stringField(value map[string]any, key string) string {
	raw, ok := value[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(raw)
}

type fakeRateLimitSnapshot struct {
	RateLimitResetCredits *fakeRateLimitResetCredits `json:"rateLimitResetCredits"`
	RateLimits            fakeRateLimits             `json:"rateLimits"`
}

type fakeRateLimitResetCredits struct {
	AvailableCount int64 `json:"availableCount"`
}

type fakeRateLimits struct {
	Primary              *fakeRateLimitWindow `json:"primary"`
	Secondary            *fakeRateLimitWindow `json:"secondary"`
	PlanType             *string              `json:"planType"`
	RateLimitReachedType *string              `json:"rateLimitReachedType"`
}

type fakeRateLimitWindow struct {
	UsedPercent        *float64 `json:"usedPercent"`
	ResetsAt           *int64   `json:"resetsAt"`
	WindowDurationMins *int64   `json:"windowDurationMins"`
}

func resetFakeRateLimitWindows(rateLimits *fakeRateLimits) bool {
	reset := false
	for _, window := range []*fakeRateLimitWindow{rateLimits.Primary, rateLimits.Secondary} {
		if window == nil || window.UsedPercent == nil || *window.UsedPercent <= 0 {
			continue
		}
		zero := 0.0
		window.UsedPercent = &zero
		reset = true
	}
	if reset {
		rateLimits.RateLimitReachedType = nil
	}
	return reset
}
