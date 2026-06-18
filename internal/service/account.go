package service

import (
	"context"
	"strings"
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

// RefreshResult 表示单账号状态刷新结果。
type RefreshResult struct {
	ProviderID   string
	AccountID    string
	Account      *AccountWithUsage
	ErrorCode    *entity.ErrorCode
	ErrorMessage *string
}

// CreateManualAccountInput 表示手动创建账号所需输入。
type CreateManualAccountInput struct {
	ProviderID string
	Email      string
}

// AccountAuthJSONImporter 描述支持导入 auth.json 的 provider 可选能力。
type AccountAuthJSONImporter interface {
	ImportAccountAuthJSON(context.Context, entity.Account, []byte) error
}

// AccountMetadataUsageRefresher 描述刷新 usage 时一并返回账号元数据的 provider 可选能力。
type AccountMetadataUsageRefresher interface {
	RefreshAccountWithMetadata(context.Context, entity.Account) (*entity.Account, *entity.UsageSnapshot, error)
}

// AccountService 编排账号生命周期、DAO 事务和 provider 调用。
type AccountService struct {
	uow       dao.UnitOfWork
	daos      dao.DAOs
	providers *provider.Registry
	now       func() time.Time

	activateMu sync.Mutex
}

// NewAccountService 创建账号 service。
func NewAccountService(uow dao.UnitOfWork, daos dao.DAOs, providers *provider.Registry) *AccountService {
	return &AccountService{
		uow:       uow,
		daos:      daos,
		providers: providers,
		now:       time.Now,
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

// CreateManualAccount 根据 OpenAI 邮箱创建本地账号元数据。
func (service *AccountService) CreateManualAccount(ctx context.Context, input CreateManualAccountInput) (AccountWithUsage, error) {
	if _, err := service.getProvider(input.ProviderID); err != nil {
		return AccountWithUsage{}, err
	}
	email := strings.TrimSpace(input.Email)
	if email == "" {
		return AccountWithUsage{}, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "email 不能为空")
	}
	accountID := entity.AccountIDFromEmail(email)
	account := service.normalizeAccount(input.ProviderID, entity.Account{
		ProviderID: input.ProviderID,
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount(input.ProviderID, accountID),
		Label:      email,
		Email:      &email,
	})
	if err := service.daos.Accounts.Upsert(ctx, account); err != nil {
		return AccountWithUsage{}, err
	}
	persisted, err := service.daos.Accounts.Get(ctx, account.ProviderID, account.AccountID)
	if err != nil {
		return AccountWithUsage{}, err
	}
	return AccountWithUsage{Account: persisted}, nil
}

// ImportAccountAuthJSON 为已有账号导入 auth.json，不切换当前活动账号。
func (service *AccountService) ImportAccountAuthJSON(ctx context.Context, providerID string, accountID string, authJSON []byte) (entity.Account, error) {
	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return entity.Account{}, err
	}
	importer, ok := registeredProvider.(AccountAuthJSONImporter)
	if !ok {
		return entity.Account{}, entity.NewAppError(entity.ErrorCodeUnsupported)
	}
	account, err := service.daos.Accounts.Get(ctx, providerID, accountID)
	if err != nil {
		return entity.Account{}, err
	}
	if err := importer.ImportAccountAuthJSON(ctx, account, authJSON); err != nil {
		return entity.Account{}, err
	}
	return account, nil
}

// ImportCurrentAccount 从 provider 当前活动登录态导入账号。
func (service *AccountService) ImportCurrentAccount(ctx context.Context, providerID string) (AccountWithUsage, error) {
	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return AccountWithUsage{}, err
	}
	imported, err := registeredProvider.ImportCurrentAccount(ctx)
	if err != nil {
		return AccountWithUsage{}, err
	}
	account := service.normalizeAccount(providerID, *imported)
	account.IsActive = false

	if err := service.uow.WithinTransaction(ctx, func(daos dao.DAOs) error {
		if err := daos.Accounts.Upsert(ctx, account); err != nil {
			return err
		}
		return daos.Accounts.SetActive(ctx, providerID, account.AccountID, service.now().UTC().UnixMilli())
	}); err != nil {
		return AccountWithUsage{}, err
	}

	persisted, err := service.daos.Accounts.Get(ctx, providerID, account.AccountID)
	if err != nil {
		return AccountWithUsage{}, err
	}
	return AccountWithUsage{Account: persisted}, nil
}

// UpdatePlanExpiration 更新人工维护的套餐到期时间。
func (service *AccountService) UpdatePlanExpiration(ctx context.Context, providerID string, accountID string, planExpiresAt *int64) (entity.Account, error) {
	now := service.now().UTC().UnixMilli()
	if err := service.daos.Accounts.UpdatePlanExpiresAt(ctx, providerID, accountID, planExpiresAt, now); err != nil {
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

// RefreshAccount 刷新单个账号状态。
func (service *AccountService) RefreshAccount(ctx context.Context, providerID string, accountID string) (RefreshResult, error) {
	account, err := service.daos.Accounts.Get(ctx, providerID, accountID)
	if err != nil {
		return RefreshResult{}, err
	}
	return service.refreshOne(ctx, account), nil
}

func (service *AccountService) refreshOne(ctx context.Context, account entity.Account) RefreshResult {
	result := RefreshResult{
		ProviderID: account.ProviderID,
		AccountID:  account.AccountID,
	}
	registeredProvider, err := service.getProvider(account.ProviderID)
	if err != nil {
		result.ErrorCode = errorCodePtr(err)
		result.ErrorMessage = errorMessagePtr(err)
		_ = service.persistFailedUsage(ctx, account, result.ErrorCode)
		return result
	}

	var refreshedAccount *entity.Account
	var snapshot *entity.UsageSnapshot
	if refresher, ok := registeredProvider.(AccountMetadataUsageRefresher); ok {
		refreshedAccount, snapshot, err = refresher.RefreshAccountWithMetadata(ctx, account)
	} else {
		snapshot, err = registeredProvider.RefreshAccount(ctx, account)
	}
	if err != nil {
		result.ErrorCode = errorCodePtr(err)
		result.ErrorMessage = errorMessagePtr(err)
		_ = service.persistFailedUsage(ctx, account, result.ErrorCode)
		return result
	}
	if err := validateRefreshedAccount(account, refreshedAccount); err != nil {
		result.ErrorCode = errorCodePtr(err)
		result.ErrorMessage = errorMessagePtr(err)
		_ = service.persistFailedUsage(ctx, account, result.ErrorCode)
		return result
	}
	normalizedSnapshot := normalizeUsageSnapshot(account, *snapshot)
	refreshedViewAccount := mergeRefreshedAccount(account, refreshedAccount, service.now().UTC().UnixMilli())
	if err := service.persistRefreshSuccess(ctx, account, refreshedAccount, normalizedSnapshot, refreshedViewAccount.UpdatedAt); err != nil {
		result.ErrorCode = errorCodePtr(err)
		result.ErrorMessage = errorMessagePtr(err)
		return result
	}
	result.Account = &AccountWithUsage{
		Account: refreshedViewAccount,
		Usage:   &normalizedSnapshot,
	}
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

func (service *AccountService) persistRefreshSuccess(ctx context.Context, account entity.Account, refreshedAccount *entity.Account, snapshot entity.UsageSnapshot, now int64) error {
	if refreshedAccount == nil {
		return service.daos.UsageSnapshots.Upsert(ctx, snapshot)
	}
	return service.uow.WithinTransaction(ctx, func(daos dao.DAOs) error {
		if err := daos.Accounts.UpdateProviderMetadata(ctx, account.ProviderID, account.AccountID, refreshedAccount.Email, refreshedAccount.PlanType, now); err != nil {
			return err
		}
		return daos.UsageSnapshots.Upsert(ctx, snapshot)
	})
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

func mergeRefreshedAccount(account entity.Account, refreshed *entity.Account, now int64) entity.Account {
	if refreshed == nil {
		return account
	}
	account.Email = refreshed.Email
	account.PlanType = refreshed.PlanType
	account.UpdatedAt = now
	return account
}

func validateRefreshedAccount(account entity.Account, refreshed *entity.Account) error {
	if refreshed == nil {
		return nil
	}
	if refreshed.ProviderID != "" && refreshed.ProviderID != account.ProviderID {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeConflict, "auth.json 对应 provider 与当前账号不一致")
	}
	if refreshed.AccountID != "" && refreshed.AccountID != account.AccountID {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeConflict, "auth.json 对应账号与当前账号不一致")
	}
	return nil
}

func (service *AccountService) getProvider(providerID string) (provider.Provider, error) {
	registeredProvider, ok := service.providers.Get(providerID)
	if !ok {
		return nil, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return registeredProvider, nil
}

func errorCodePtr(err error) *entity.ErrorCode {
	code := entity.ErrorCodeInternal
	if appErr, ok := entity.AsAppError(err); ok {
		code = appErr.ErrorCode()
	}
	return &code
}

func errorMessagePtr(err error) *string {
	if appErr, ok := entity.AsAppError(err); ok {
		message := appErr.DisplayMessage()
		return &message
	}
	return nil
}

func accountKey(providerID string, accountID string) string {
	return providerID + "\x00" + accountID
}
