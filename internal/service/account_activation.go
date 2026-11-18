package service

import (
	"context"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

// AccountActivationCoordinator 串行化活动账号状态与活动 CODEX_HOME 凭据的更新。
type AccountActivationCoordinator struct {
	token chan struct{}
}

// NewAccountActivationCoordinator 创建活动账号协调器。
func NewAccountActivationCoordinator() *AccountActivationCoordinator {
	coordinator := &AccountActivationCoordinator{token: make(chan struct{}, 1)}
	coordinator.token <- struct{}{}
	return coordinator
}

func (coordinator *AccountActivationCoordinator) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-coordinator.token:
		return nil
	}
}

func (coordinator *AccountActivationCoordinator) tryAcquire() bool {
	select {
	case <-coordinator.token:
		return true
	default:
		return false
	}
}

func (coordinator *AccountActivationCoordinator) release() {
	coordinator.token <- struct{}{}
}

// ActivateAccount 激活账号，并要求同一时间只有一个 activate 操作。
func (service *AccountService) ActivateAccount(ctx context.Context, providerID string, accountID string) (entity.Account, error) {
	if !service.activation.tryAcquire() {
		return entity.Account{}, entity.NewAppError(entity.ErrorCodeOperationInProgress)
	}
	defer service.activation.release()

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

func (service *AccountService) syncRefreshedActiveAccount(ctx context.Context, registeredProvider provider.Provider, providerID string, accountID string) (entity.Account, error) {
	if err := service.activation.acquire(ctx); err != nil {
		return entity.Account{}, err
	}
	defer service.activation.release()

	account, err := service.daos.Accounts.Get(ctx, providerID, accountID)
	if err != nil {
		return entity.Account{}, err
	}
	if !account.IsActive {
		return account, nil
	}
	if err := registeredProvider.ActivateAccount(ctx, account); err != nil {
		return entity.Account{}, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "同步当前 Codex 登录态失败", err)
	}
	return account, nil
}
