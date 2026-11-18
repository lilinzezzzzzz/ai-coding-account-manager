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

// CreateManualAccountInput 表示手动创建账号所需输入。
type CreateManualAccountInput struct {
	ProviderID string
	Email      string
}

// AccountService 编排账号生命周期、DAO 事务和 provider 调用。
type AccountService struct {
	uow        dao.UnitOfWork
	daos       dao.DAOs
	providers  *provider.Registry
	now        func() time.Time
	activation *AccountActivationCoordinator
	// accountOperations 防止删除与同账号的凭据刷新、额度重置交错执行。
	accountOperations sync.Map
}

// NewAccountService 创建账号 service。
func NewAccountService(uow dao.UnitOfWork, daos dao.DAOs, providers *provider.Registry, activation *AccountActivationCoordinator) *AccountService {
	return &AccountService{
		uow:        uow,
		daos:       daos,
		providers:  providers,
		now:        time.Now,
		activation: activation,
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
	account := normalizeAccount(input.ProviderID, entity.Account{
		AccountID: accountID,
		Label:     email,
		Email:     &email,
	}, service.now().UTC().UnixMilli())
	if err := service.daos.Accounts.Upsert(ctx, account); err != nil {
		return AccountWithUsage{}, err
	}
	persisted, err := service.daos.Accounts.Get(ctx, account.ProviderID, account.AccountID)
	if err != nil {
		return AccountWithUsage{}, err
	}
	return AccountWithUsage{Account: persisted}, nil
}

// UpdatePlan 更新人工维护的续期方式和对应日期。
func (service *AccountService) UpdatePlan(ctx context.Context, providerID string, accountID string, autoRenew bool, planDate *int64) (entity.Account, error) {
	now := service.now().UTC().UnixMilli()
	if err := service.daos.Accounts.UpdatePlan(ctx, providerID, accountID, autoRenew, planDate, now); err != nil {
		return entity.Account{}, err
	}
	return service.daos.Accounts.Get(ctx, providerID, accountID)
}

func normalizeAccount(providerID string, account entity.Account, now int64) entity.Account {
	account.ProviderID = providerID
	account.PlanAutoRenew = true
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

func (service *AccountService) getProvider(providerID string) (provider.Provider, error) {
	registeredProvider, ok := service.providers.Get(providerID)
	if !ok {
		return nil, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return registeredProvider, nil
}

func isAppErrorCode(err error, code entity.ErrorCode) bool {
	appErr, ok := entity.AsAppError(err)
	return ok && appErr.ErrorCode() == code
}

func accountKey(providerID string, accountID string) string {
	return providerID + "\x00" + accountID
}

func (service *AccountService) beginAccountOperation(providerID string, accountID string) (func(), error) {
	key := accountKey(providerID, accountID)
	if _, busy := service.accountOperations.LoadOrStore(key, struct{}{}); busy {
		return nil, entity.NewAppError(entity.ErrorCodeOperationInProgress)
	}
	return func() { service.accountOperations.Delete(key) }, nil
}
