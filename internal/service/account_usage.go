package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

// RefreshResult 表示单账号状态刷新结果。
type RefreshResult struct {
	ProviderID   string
	AccountID    string
	Account      *AccountWithUsage
	ErrorCode    *string
	ErrorMessage *string
}

// AccountMetadataUsageRefresher 描述刷新 usage 时一并返回账号元数据的 provider 可选能力。
type AccountMetadataUsageRefresher interface {
	RefreshAccountWithMetadata(context.Context, entity.Account) (*entity.Account, *entity.UsageSnapshot, error)
}

// AccountRateLimitResetter 描述支持消费 reset credit 的 provider 可选能力。
type AccountRateLimitResetter interface {
	ResetAccountRateLimit(context.Context, entity.Account, string) (provider.RateLimitResetResult, error)
}

// ResetRateLimitResult 表示账号额度重置结果和最新账号视图。
type ResetRateLimitResult struct {
	Outcome provider.RateLimitResetOutcome
	Account AccountWithUsage
}

// RefreshAccount 刷新单个账号状态。
func (service *AccountService) RefreshAccount(ctx context.Context, providerID string, accountID string) (RefreshResult, error) {
	release, err := service.beginAccountOperation(providerID, accountID)
	if err != nil {
		return RefreshResult{}, err
	}
	defer release()
	account, err := service.daos.Accounts.Get(ctx, providerID, accountID)
	if err != nil {
		return RefreshResult{}, err
	}
	return service.refreshOne(ctx, account), nil
}

func (service *AccountService) refreshOne(ctx context.Context, account entity.Account) RefreshResult {
	registeredProvider, err := service.getProvider(account.ProviderID)
	if err != nil {
		return service.recordUnavailableUsage(ctx, account, err)
	}

	logRefreshStarted(ctx, account)

	var refreshedAccount *entity.Account
	var snapshot *entity.UsageSnapshot
	if refresher, ok := registeredProvider.(AccountMetadataUsageRefresher); ok {
		refreshedAccount, snapshot, err = refresher.RefreshAccountWithMetadata(ctx, account)
	} else {
		snapshot, err = registeredProvider.RefreshAccount(ctx, account)
	}
	if err != nil {
		return service.recordUnavailableUsage(ctx, account, err)
	}
	if err := validateRefreshedAccount(account, refreshedAccount); err != nil {
		return service.recordUnavailableUsage(ctx, account, err)
	}
	if snapshot == nil {
		err := entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "账号状态刷新未返回 usage")
		return service.recordUnavailableUsage(ctx, account, err)
	}
	normalizedSnapshot := normalizeUsageSnapshot(account, *snapshot)
	now := service.now().UTC().UnixMilli()
	if err := service.persistRefreshSuccess(ctx, account, refreshedAccount, normalizedSnapshot, now); err != nil {
		return failedRefresh(ctx, account, err)
	}
	currentAccount, err := service.syncRefreshedActiveAccount(ctx, registeredProvider, account.ProviderID, account.AccountID)
	if err != nil {
		return failedRefresh(ctx, account, err)
	}
	return RefreshResult{
		ProviderID: account.ProviderID,
		AccountID:  account.AccountID,
		Account:    &AccountWithUsage{Account: currentAccount, Usage: &normalizedSnapshot},
	}
}

// Provider 读取失败时记录不可用状态。持久化或同步失败时保留已有额度快照。
func (service *AccountService) recordUnavailableUsage(ctx context.Context, account entity.Account, err error) RefreshResult {
	result := failedRefresh(ctx, account, err)
	_ = service.persistFailedUsage(ctx, account, errorCodePtr(err))
	return result
}

func failedRefresh(ctx context.Context, account entity.Account, err error) RefreshResult {
	logRefreshFailure(ctx, account, errorCodePtr(err), err)
	return RefreshResult{
		ProviderID:   account.ProviderID,
		AccountID:    account.AccountID,
		ErrorCode:    responseErrorCodePtr(err),
		ErrorMessage: errorMessagePtr(err),
	}
}

// ResetAccountRateLimit 消耗一次 reset credit，并持久化重置后的 usage。
func (service *AccountService) ResetAccountRateLimit(ctx context.Context, providerID string, accountID string, idempotencyKey string) (ResetRateLimitResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 128 {
		return ResetRateLimitResult{}, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "idempotencyKey 无效")
	}
	release, err := service.beginAccountOperation(providerID, accountID)
	if err != nil {
		return ResetRateLimitResult{}, err
	}
	defer release()
	registeredProvider, err := service.getProvider(providerID)
	if err != nil {
		return ResetRateLimitResult{}, err
	}
	resetter, ok := registeredProvider.(AccountRateLimitResetter)
	if !ok {
		return ResetRateLimitResult{}, entity.NewAppError(entity.ErrorCodeUnsupported)
	}
	account, err := service.daos.Accounts.Get(ctx, providerID, accountID)
	if err != nil {
		return ResetRateLimitResult{}, err
	}
	usage, err := service.daos.UsageSnapshots.Get(ctx, providerID, accountID)
	if err != nil {
		if isAppErrorCode(err, entity.ErrorCodeNotFound) {
			return ResetRateLimitResult{}, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "重置次数不可用，请先刷新账号状态")
		}
		return ResetRateLimitResult{}, err
	}
	availableCount, err := rateLimitResetCreditAvailableCount(usage)
	if err != nil {
		return ResetRateLimitResult{}, err
	}
	if availableCount <= 0 {
		return ResetRateLimitResult{
			Outcome: provider.RateLimitResetOutcomeNoCredit,
			Account: AccountWithUsage{
				Account: account,
				Usage:   &usage,
			},
		}, nil
	}
	providerResult, err := resetter.ResetAccountRateLimit(ctx, account, idempotencyKey)
	if err != nil {
		if isAppErrorCode(err, entity.ErrorCodeReauthenticationRequired) {
			_ = service.persistFailedUsage(ctx, account, errorCodePtr(err))
		}
		return ResetRateLimitResult{}, err
	}
	if err := validateRefreshedAccount(account, providerResult.Account); err != nil {
		return ResetRateLimitResult{}, err
	}
	if providerResult.Usage == nil {
		return ResetRateLimitResult{}, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "额度重置后未返回最新 usage")
	}
	normalizedSnapshot := normalizeUsageSnapshot(account, *providerResult.Usage)
	now := service.now().UTC().UnixMilli()
	refreshedAccount := mergeRefreshedAccount(account, providerResult.Account, now)
	if err := service.persistRefreshSuccess(ctx, account, providerResult.Account, normalizedSnapshot, now); err != nil {
		return ResetRateLimitResult{}, err
	}
	return ResetRateLimitResult{
		Outcome: providerResult.Outcome,
		Account: AccountWithUsage{
			Account: refreshedAccount,
			Usage:   &normalizedSnapshot,
		},
	}, nil
}

func rateLimitResetCreditAvailableCount(snapshot entity.UsageSnapshot) (int64, error) {
	if snapshot.SnapshotJSON == nil || strings.TrimSpace(*snapshot.SnapshotJSON) == "" {
		return 0, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "重置次数不可用，请先刷新账号状态")
	}
	var payload struct {
		RateLimitResetCredits *struct {
			AvailableCount int64 `json:"availableCount"`
		} `json:"rateLimitResetCredits"`
	}
	if err := json.Unmarshal([]byte(*snapshot.SnapshotJSON), &payload); err != nil || payload.RateLimitResetCredits == nil {
		return 0, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "重置次数不可用，请先刷新账号状态")
	}
	return payload.RateLimitResetCredits.AvailableCount, nil
}

func (service *AccountService) persistFailedUsage(ctx context.Context, account entity.Account, code *entity.ErrorCode) error {
	status := entity.UsageStatusUnavailable
	if code != nil && *code == entity.ErrorCodeReauthenticationRequired {
		status = entity.UsageStatusAuthExpired
	}
	snapshot := entity.UsageSnapshot{
		ProviderID:  account.ProviderID,
		AccountID:   account.AccountID,
		Status:      status,
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

func normalizeUsageSnapshot(account entity.Account, snapshot entity.UsageSnapshot) entity.UsageSnapshot {
	snapshot.ProviderID = account.ProviderID
	snapshot.AccountID = account.AccountID
	if snapshot.Status == "" {
		snapshot.Status = entity.UsageStatusReady
	}
	if snapshot.ResetsAt != nil {
		resetsAt := normalizeEpochMillis(*snapshot.ResetsAt)
		snapshot.ResetsAt = &resetsAt
	}
	return snapshot
}

func normalizeEpochMillis(value int64) int64 {
	if value > 0 && value < 100000000000 {
		return value * 1000
	}
	return value
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

func errorCodePtr(err error) *entity.ErrorCode {
	code := entity.ErrorCodeInternal
	if appErr, ok := entity.AsAppError(err); ok {
		code = appErr.ErrorCode()
	}
	return &code
}

func responseErrorCodePtr(err error) *string {
	if appErr, ok := entity.AsAppError(err); ok {
		if appErr.ErrorCode() == entity.ErrorCodeReauthenticationRequired {
			code := string(entity.ErrorCodeReauthenticationRequired)
			return &code
		}
		if appErr.UpstreamCode != "" {
			code := appErr.UpstreamCode
			return &code
		}
	}
	code := string(*errorCodePtr(err))
	return &code
}

func errorMessagePtr(err error) *string {
	if appErr, ok := entity.AsAppError(err); ok {
		message := appErr.DisplayMessage()
		return &message
	}
	return nil
}

func logRefreshStarted(ctx context.Context, account entity.Account) {
	slog.InfoContext(ctx,
		"account refresh started",
		"provider_id", account.ProviderID,
		"account_id", account.AccountID,
	)
}

func logRefreshFailure(ctx context.Context, account entity.Account, code *entity.ErrorCode, err error) {
	fields := []any{
		"provider_id", account.ProviderID,
		"account_id", account.AccountID,
		"error", err,
	}
	if code != nil {
		fields = append(fields, "error_code", *code)
	}
	if appErr, ok := entity.AsAppError(err); ok {
		if appErr.UpstreamCode != "" {
			fields = append(fields, "upstream_error_code", appErr.UpstreamCode)
		}
		if appErr.Cause != nil {
			fields = append(fields, "cause", appErr.Cause, "cause_type", fmt.Sprintf("%T", appErr.Cause))
		}
	}
	slog.WarnContext(ctx, "account refresh failed", fields...)
}
