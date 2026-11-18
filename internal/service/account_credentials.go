package service

import (
	"context"
	"log/slog"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

// AccountAuthJSONRefreshImporter 描述支持导入 auth.json 后识别并刷新账号的 provider 可选能力。
type AccountAuthJSONRefreshImporter interface {
	ImportAccountAuthJSONAndRefresh(context.Context, []byte) (*entity.Account, *entity.UsageSnapshot, error)
}

// AccountAuthJSONExporter 描述支持用户主动导出单账号凭据的 provider 可选能力。
type AccountAuthJSONExporter interface {
	ExportAccountAuthJSON(context.Context, entity.Account) ([]byte, error)
}

// ImportAccountAuthJSONAndRefresh 导入 auth.json，刷新成功后才写入账号和 usage。
func (service *AccountService) ImportAccountAuthJSONAndRefresh(ctx context.Context, providerID string, authJSON []byte) (AccountWithUsage, error) {
	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return AccountWithUsage{}, err
	}
	importer, ok := registeredProvider.(AccountAuthJSONRefreshImporter)
	if !ok {
		return AccountWithUsage{}, entity.NewAppError(entity.ErrorCodeUnsupported)
	}

	importedAccount, snapshot, err := importer.ImportAccountAuthJSONAndRefresh(ctx, authJSON)
	if err != nil {
		return AccountWithUsage{}, err
	}
	if importedAccount == nil || strings.TrimSpace(importedAccount.AccountID) == "" {
		return AccountWithUsage{}, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "无法识别 auth.json 对应账号")
	}
	if snapshot == nil {
		return AccountWithUsage{}, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "账号状态刷新失败")
	}

	account := normalizeAccount(providerID, *importedAccount, service.now().UTC().UnixMilli())
	existing, existed, err := service.getExistingAccount(ctx, providerID, account.AccountID)
	if err != nil {
		return AccountWithUsage{}, err
	}
	if existed {
		account.IsActive = existing.IsActive
		account.LastUsedAt = existing.LastUsedAt
	}
	normalizedSnapshot := normalizeUsageSnapshot(account, *snapshot)

	if err := service.uow.WithinTransaction(ctx, func(daos dao.DAOs) error {
		if err := daos.Accounts.Upsert(ctx, account); err != nil {
			return err
		}
		return daos.UsageSnapshots.Upsert(ctx, normalizedSnapshot)
	}); err != nil {
		if !existed {
			service.cleanupImportedAccountData(ctx, registeredProvider, account)
		}
		return AccountWithUsage{}, err
	}

	persisted, err := service.daos.Accounts.Get(ctx, account.ProviderID, account.AccountID)
	if err != nil {
		return AccountWithUsage{}, err
	}
	return AccountWithUsage{Account: persisted, Usage: &normalizedSnapshot}, nil
}

// ExportAccountAuthJSON 根据已保存的账号定位凭据，不接受客户端传入文件路径。
func (service *AccountService) ExportAccountAuthJSON(ctx context.Context, providerID string, accountID string) ([]byte, error) {
	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return nil, err
	}
	account, err := service.daos.Accounts.Get(ctx, providerID, accountID)
	if err != nil {
		return nil, err
	}
	exporter, ok := registeredProvider.(AccountAuthJSONExporter)
	if !ok {
		return nil, entity.NewAppError(entity.ErrorCodeUnsupported)
	}
	return exporter.ExportAccountAuthJSON(ctx, account)
}

// ImportCurrentAccount 从 provider 当前活动登录态导入账号。
func (service *AccountService) ImportCurrentAccount(ctx context.Context, providerID string) (AccountWithUsage, error) {
	if err := service.activation.acquire(ctx); err != nil {
		return AccountWithUsage{}, err
	}
	defer service.activation.release()

	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return AccountWithUsage{}, err
	}
	imported, err := registeredProvider.ImportCurrentAccount(ctx)
	if err != nil {
		return AccountWithUsage{}, err
	}
	if imported == nil || strings.TrimSpace(imported.AccountID) == "" {
		return AccountWithUsage{}, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "无法识别当前登录账号")
	}
	account := normalizeAccount(providerID, *imported, service.now().UTC().UnixMilli())
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

// DeleteAccount 删除管理器中的账号及隔离凭据，保留客户端当前登录态。
func (service *AccountService) DeleteAccount(ctx context.Context, providerID string, accountID string) error {
	release, err := service.beginAccountOperation(providerID, accountID)
	if err != nil {
		return err
	}
	defer release()
	if err := service.activation.acquire(ctx); err != nil {
		return err
	}
	defer service.activation.release()

	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return err
	}
	account, err := service.daos.Accounts.Get(ctx, providerID, accountID)
	if err != nil {
		return err
	}
	// 先清理隔离数据，失败时保留账号记录，用户可以重试。
	if err := registeredProvider.RemoveAccountData(ctx, account); err != nil {
		return err
	}
	return service.uow.WithinTransaction(ctx, func(daos dao.DAOs) error {
		return daos.Accounts.Delete(ctx, providerID, accountID)
	})
}

func (service *AccountService) getExistingAccount(ctx context.Context, providerID string, accountID string) (entity.Account, bool, error) {
	account, err := service.daos.Accounts.Get(ctx, providerID, accountID)
	if err == nil {
		return account, true, nil
	}
	if isAppErrorCode(err, entity.ErrorCodeNotFound) {
		return entity.Account{}, false, nil
	}
	return entity.Account{}, false, err
}

func (service *AccountService) cleanupImportedAccountData(ctx context.Context, registeredProvider provider.Provider, account entity.Account) {
	if err := registeredProvider.RemoveAccountData(ctx, account); err != nil {
		slog.WarnContext(ctx,
			"cleanup imported account credentials failed",
			"provider_id", account.ProviderID,
			"account_id", account.AccountID,
			"error_code", errorCodePtr(err),
		)
	}
}
