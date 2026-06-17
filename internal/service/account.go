package service

import (
	"context"
	"sync"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

const defaultAccountListLimit = 500

// AccountWithUsage 表示账号和最近一次 usage snapshot 的组合视图。
type AccountWithUsage struct {
	Account entity.Account
	Usage   *entity.UsageSnapshot
}

// RefreshResult 表示单账号刷新结果。
type RefreshResult struct {
	ProviderID string
	AccountID  string
	Usage      *entity.UsageSnapshot
	ErrorCode  *entity.ErrorCode
}

// AccountService 编排账号生命周期、DAO 事务和 provider 调用。
type AccountService struct {
	uow       dao.UnitOfWork
	daos      dao.DAOs
	providers *provider.Registry
	now       func() time.Time

	activateMu sync.Mutex

	refreshMu  sync.Mutex
	refreshing bool

	loginMu    sync.Mutex
	loginTasks map[string]string
}

// NewAccountService 创建账号 service。
func NewAccountService(uow dao.UnitOfWork, daos dao.DAOs, providers *provider.Registry) *AccountService {
	return &AccountService{
		uow:        uow,
		daos:       daos,
		providers:  providers,
		now:        time.Now,
		loginTasks: map[string]string{},
	}
}

// ListAccounts 返回账号和最近 usage snapshot。
func (service *AccountService) ListAccounts(ctx context.Context) ([]AccountWithUsage, error) {
	accounts, err := service.daos.Accounts.ListAll(ctx, defaultAccountListLimit)
	if err != nil {
		return nil, err
	}
	snapshots, err := service.daos.UsageSnapshots.ListAll(ctx, defaultAccountListLimit)
	if err != nil {
		return nil, err
	}

	usageByAccount := make(map[string]entity.UsageSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		usageByAccount[accountKey(snapshot.ProviderID, snapshot.AccountID)] = snapshot
	}

	result := make([]AccountWithUsage, 0, len(accounts))
	for _, account := range accounts {
		view := AccountWithUsage{Account: account}
		if snapshot, ok := usageByAccount[accountKey(account.ProviderID, account.AccountID)]; ok {
			view.Usage = &snapshot
		}
		result = append(result, view)
	}
	return result, nil
}

// ImportCurrentAccount 从 provider 导入当前账号并设为 active。
func (service *AccountService) ImportCurrentAccount(ctx context.Context, providerID string) (AccountWithUsage, error) {
	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return AccountWithUsage{}, err
	}

	account, err := registeredProvider.DiscoverCurrentAccount(ctx)
	if err != nil {
		return AccountWithUsage{}, err
	}
	normalizedAccount := service.normalizeAccount(providerID, *account)

	var usage *entity.UsageSnapshot
	if snapshot, err := registeredProvider.RefreshAccount(ctx, normalizedAccount); err == nil && snapshot != nil {
		normalizedSnapshot := normalizeUsageSnapshot(normalizedAccount, *snapshot)
		usage = &normalizedSnapshot
	}

	err = service.uow.WithinTransaction(ctx, func(daos dao.DAOs) error {
		accountForUpsert := normalizedAccount
		accountForUpsert.IsActive = false
		if err := daos.Accounts.Upsert(ctx, accountForUpsert); err != nil {
			return err
		}
		if err := daos.Accounts.SetActive(ctx, normalizedAccount.ProviderID, normalizedAccount.AccountID, service.now().UTC().UnixMilli()); err != nil {
			return err
		}
		if usage != nil {
			return daos.UsageSnapshots.Upsert(ctx, *usage)
		}
		return nil
	})
	if err != nil {
		return AccountWithUsage{}, err
	}

	persisted, err := service.daos.Accounts.Get(ctx, normalizedAccount.ProviderID, normalizedAccount.AccountID)
	if err != nil {
		return AccountWithUsage{}, err
	}
	return AccountWithUsage{Account: persisted, Usage: usage}, nil
}

// RenameAccount 更新账号 label。
func (service *AccountService) RenameAccount(ctx context.Context, providerID string, accountID string, label string) (entity.Account, error) {
	now := service.now().UTC().UnixMilli()
	if err := service.daos.Accounts.UpdateLabel(ctx, providerID, accountID, label, now); err != nil {
		return entity.Account{}, err
	}
	return service.daos.Accounts.Get(ctx, providerID, accountID)
}

// ActivateAccount 激活账号，并要求同一时间只有一个 activate 操作。
func (service *AccountService) ActivateAccount(ctx context.Context, providerID string, accountID string) (entity.Account, error) {
	if !service.activateMu.TryLock() {
		return entity.Account{}, entity.NewAppError(entity.ErrorCodeOperationInProgress)
	}
	defer service.activateMu.Unlock()

	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return entity.Account{}, err
	}
	account, err := service.daos.Accounts.Get(ctx, providerID, accountID)
	if err != nil {
		return entity.Account{}, err
	}
	if err := registeredProvider.ActivateAccount(ctx, account); err != nil {
		return entity.Account{}, err
	}
	if err := service.uow.WithinTransaction(ctx, func(daos dao.DAOs) error {
		return daos.Accounts.SetActive(ctx, providerID, accountID, service.now().UTC().UnixMilli())
	}); err != nil {
		return entity.Account{}, err
	}
	return service.daos.Accounts.Get(ctx, providerID, accountID)
}

// DeleteAccount 删除非 active 账号。
func (service *AccountService) DeleteAccount(ctx context.Context, providerID string, accountID string) error {
	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return err
	}
	account, err := service.daos.Accounts.Get(ctx, providerID, accountID)
	if err != nil {
		return err
	}
	if account.IsActive {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeConflict, "活动账号不能删除")
	}
	if err := service.uow.WithinTransaction(ctx, func(daos dao.DAOs) error {
		return daos.Accounts.Delete(ctx, providerID, accountID)
	}); err != nil {
		return err
	}
	return registeredProvider.RemoveAccountData(ctx, account)
}

// StartLogin 启动 provider 登录任务。
func (service *AccountService) StartLogin(ctx context.Context, providerID string) (*provider.LoginTask, error) {
	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return nil, err
	}
	task, err := registeredProvider.StartLogin(ctx)
	if err != nil {
		return nil, err
	}
	service.loginMu.Lock()
	service.loginTasks[task.ID] = providerID
	service.loginMu.Unlock()
	return task, nil
}

// PollLogin 查询登录任务。
func (service *AccountService) PollLogin(ctx context.Context, taskID string) (*provider.LoginStatus, error) {
	providerID, err := service.providerIDForTask(taskID)
	if err != nil {
		return nil, err
	}
	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return nil, err
	}
	status, err := registeredProvider.PollLogin(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if status.State != provider.LoginStateCompleted || status.Account == nil {
		return status, nil
	}

	normalizedAccount := service.normalizeAccount(providerID, *status.Account)
	if err := service.uow.WithinTransaction(ctx, func(daos dao.DAOs) error {
		accountForUpsert := normalizedAccount
		accountForUpsert.IsActive = false
		if err := daos.Accounts.Upsert(ctx, accountForUpsert); err != nil {
			return err
		}
		return daos.Accounts.SetActive(ctx, normalizedAccount.ProviderID, normalizedAccount.AccountID, service.now().UTC().UnixMilli())
	}); err != nil {
		_ = registeredProvider.RemoveAccountData(context.Background(), normalizedAccount)
		return nil, err
	}

	service.loginMu.Lock()
	delete(service.loginTasks, taskID)
	service.loginMu.Unlock()
	status.Account = &normalizedAccount
	return status, nil
}

// CancelLogin 取消登录任务。
func (service *AccountService) CancelLogin(ctx context.Context, taskID string) error {
	providerID, err := service.providerIDForTask(taskID)
	if err != nil {
		return err
	}
	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return err
	}
	if err := registeredProvider.CancelLogin(ctx, taskID); err != nil {
		return err
	}
	service.loginMu.Lock()
	delete(service.loginTasks, taskID)
	service.loginMu.Unlock()
	return nil
}

// RefreshAllUsage 刷新全部账号 usage。单账号失败不会阻断其它账号。
func (service *AccountService) RefreshAllUsage(ctx context.Context) ([]RefreshResult, error) {
	service.refreshMu.Lock()
	if service.refreshing {
		service.refreshMu.Unlock()
		return nil, entity.NewAppError(entity.ErrorCodeOperationInProgress)
	}
	service.refreshing = true
	service.refreshMu.Unlock()
	defer func() {
		service.refreshMu.Lock()
		service.refreshing = false
		service.refreshMu.Unlock()
	}()

	accounts, err := service.daos.Accounts.ListAll(ctx, defaultAccountListLimit)
	if err != nil {
		return nil, err
	}

	results := make([]RefreshResult, 0, len(accounts))
	if len(accounts) == 0 {
		return results, nil
	}
	results = make([]RefreshResult, len(accounts))
	semaphore := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for index, account := range accounts {
		index, account := index, account
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() {
					<-semaphore
				}()
				results[index] = service.refreshOne(ctx, account)
			case <-ctx.Done():
				code := entity.ErrorCodeUnavailable
				results[index] = RefreshResult{
					ProviderID: account.ProviderID,
					AccountID:  account.AccountID,
					ErrorCode:  &code,
				}
			}
		}()
	}
	wg.Wait()
	return results, nil
}

func (service *AccountService) refreshOne(ctx context.Context, account entity.Account) RefreshResult {
	result := RefreshResult{
		ProviderID: account.ProviderID,
		AccountID:  account.AccountID,
	}
	registeredProvider, err := service.getProvider(account.ProviderID)
	if err != nil {
		result.ErrorCode = errorCodePtr(err)
		_ = service.persistFailedUsage(ctx, account, result.ErrorCode)
		return result
	}

	snapshot, err := registeredProvider.RefreshAccount(ctx, account)
	if err != nil {
		result.ErrorCode = errorCodePtr(err)
		_ = service.persistFailedUsage(ctx, account, result.ErrorCode)
		return result
	}
	normalizedSnapshot := normalizeUsageSnapshot(account, *snapshot)
	if err := service.daos.UsageSnapshots.Upsert(ctx, normalizedSnapshot); err != nil {
		result.ErrorCode = errorCodePtr(err)
		return result
	}
	result.Usage = &normalizedSnapshot
	return result
}

func (service *AccountService) persistFailedUsage(ctx context.Context, account entity.Account, code *entity.ErrorCode) error {
	snapshot := entity.UsageSnapshot{
		ProviderID:  account.ProviderID,
		AccountID:   account.AccountID,
		Status:      entity.UsageStatusUnavailable,
		ErrorCode:   code,
		RefreshedAt: service.now().UTC().UnixMilli(),
	}
	return service.daos.UsageSnapshots.Upsert(ctx, snapshot)
}

func (service *AccountService) normalizeAccount(providerID string, account entity.Account) entity.Account {
	now := service.now().UTC().UnixMilli()
	account.ProviderID = providerID
	if account.StorageID == "" {
		account.StorageID = entity.StorageIDForAccount(account.ProviderID, account.AccountID)
	}
	if account.Label == "" {
		account.Label = account.AccountID
	}
	if account.CreatedAt == 0 {
		account.CreatedAt = now
	}
	account.UpdatedAt = now
	return account
}

func normalizeUsageSnapshot(account entity.Account, snapshot entity.UsageSnapshot) entity.UsageSnapshot {
	snapshot.ProviderID = account.ProviderID
	snapshot.AccountID = account.AccountID
	if snapshot.Status == "" {
		snapshot.Status = entity.UsageStatusReady
	}
	return snapshot
}

func (service *AccountService) getProvider(providerID string) (provider.Provider, error) {
	registeredProvider, ok := service.providers.Get(providerID)
	if !ok {
		return nil, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return registeredProvider, nil
}

func (service *AccountService) providerIDForTask(taskID string) (string, error) {
	service.loginMu.Lock()
	defer service.loginMu.Unlock()

	providerID, ok := service.loginTasks[taskID]
	if !ok {
		return "", entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return providerID, nil
}

func errorCodePtr(err error) *entity.ErrorCode {
	code := entity.ErrorCodeInternal
	if appErr, ok := entity.AsAppError(err); ok {
		code = appErr.ErrorCode()
	}
	return &code
}

func accountKey(providerID string, accountID string) string {
	return providerID + "\x00" + accountID
}
