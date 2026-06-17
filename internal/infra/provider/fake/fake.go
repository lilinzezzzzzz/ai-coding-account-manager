package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

// AccountState 描述 fake provider 中的一个账号及其 usage 状态。
type AccountState struct {
	Account   entity.Account
	Usage     entity.UsageSnapshot
	IsCurrent bool
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
	currentKey    string
	loginTasks    map[string]provider.LoginStatus
	nextTaskID    int
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
		loginTasks:    map[string]provider.LoginStatus{},
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

		if state.IsCurrent {
			fakeProvider.currentKey = key
		}
	}
	return fakeProvider
}

// DefaultCapabilities 返回 fake provider 默认支持的能力。
func DefaultCapabilities() provider.Capabilities {
	return provider.Capabilities{
		CanImportCurrentAccount:           true,
		CanLogin:                          true,
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

// DiscoverCurrentAccount 返回当前 fake 账号。
func (fakeProvider *Provider) DiscoverCurrentAccount(context.Context) (*entity.Account, error) {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if err := fakeProvider.ensureAvailableLocked(); err != nil {
		return nil, err
	}
	if !fakeProvider.description.Capabilities.CanImportCurrentAccount {
		return nil, provider.Unsupported()
	}
	if fakeProvider.currentKey == "" {
		return nil, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	account := fakeProvider.accounts[fakeProvider.currentKey]
	return &account, nil
}

// StartLogin 创建 fake 登录任务。
func (fakeProvider *Provider) StartLogin(context.Context) (*provider.LoginTask, error) {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if err := fakeProvider.ensureAvailableLocked(); err != nil {
		return nil, err
	}
	if !fakeProvider.description.Capabilities.CanLogin {
		return nil, provider.Unsupported()
	}

	fakeProvider.nextTaskID++
	taskID := fmt.Sprintf("fake-login-%d", fakeProvider.nextTaskID)
	status := provider.LoginStatus{
		ID:    taskID,
		State: provider.LoginStatePending,
	}
	fakeProvider.loginTasks[taskID] = status

	return &provider.LoginTask{
		ID:        taskID,
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute).UnixMilli(),
	}, nil
}

// PollLogin 返回 fake 登录任务状态。
func (fakeProvider *Provider) PollLogin(_ context.Context, taskID string) (*provider.LoginStatus, error) {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if err := fakeProvider.ensureAvailableLocked(); err != nil {
		return nil, err
	}
	status, ok := fakeProvider.loginTasks[taskID]
	if !ok {
		return nil, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return &status, nil
}

// CancelLogin 取消 fake 登录任务。
func (fakeProvider *Provider) CancelLogin(_ context.Context, taskID string) error {
	fakeProvider.mu.Lock()
	defer fakeProvider.mu.Unlock()

	if err := fakeProvider.ensureAvailableLocked(); err != nil {
		return err
	}
	status, ok := fakeProvider.loginTasks[taskID]
	if !ok {
		return entity.NewAppError(entity.ErrorCodeNotFound)
	}
	status.State = provider.LoginStateCanceled
	fakeProvider.loginTasks[taskID] = status
	return nil
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
		return nil, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	if usage.RefreshedAt == 0 {
		usage.RefreshedAt = time.Now().UTC().UnixMilli()
	}
	return &usage, nil
}

// ActivateAccount 将目标账号标记为当前账号。
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
	fakeProvider.currentKey = key
	return nil
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
	if fakeProvider.currentKey == key {
		fakeProvider.currentKey = ""
	}
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
