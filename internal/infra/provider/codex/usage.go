package codex

import (
	"context"
	"os"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

// RefreshAccount 使用账号隔离 CODEX_HOME 刷新 usage。
func (providerImpl *Provider) RefreshAccount(ctx context.Context, account entity.Account) (*entity.UsageSnapshot, error) {
	_, snapshot, err := providerImpl.RefreshAccountWithMetadata(ctx, account)
	return snapshot, err
}

// RefreshAccountWithMetadata 使用账号隔离 CODEX_HOME 刷新 usage，并返回最新账号元数据。
func (providerImpl *Provider) RefreshAccountWithMetadata(ctx context.Context, account entity.Account) (*entity.Account, *entity.UsageSnapshot, error) {
	runtimeDir, err := providerImpl.prepareAccountCodexHome(ctx, account)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = os.RemoveAll(runtimeDir) }()

	refreshedAccount, snapshot, err := providerImpl.refreshCodexHome(ctx, runtimeDir, account.AccountID)
	if err != nil {
		return nil, nil, err
	}
	if err := providerImpl.saveRefreshedCredentials(ctx, account.StorageID, runtimeDir); err != nil {
		return nil, nil, err
	}
	return refreshedAccount, snapshot, nil
}

// ResetAccountRateLimit 消耗一次 reset credit，并返回重置后的 usage。
func (providerImpl *Provider) ResetAccountRateLimit(ctx context.Context, account entity.Account, idempotencyKey string) (provider.RateLimitResetResult, error) {
	runtimeDir, err := providerImpl.prepareAccountCodexHome(ctx, account)
	if err != nil {
		return provider.RateLimitResetResult{}, err
	}
	defer func() { _ = os.RemoveAll(runtimeDir) }()

	var outcome provider.RateLimitResetOutcome
	refreshedAccount, snapshot, err := providerImpl.fetchCodexUsage(ctx, runtimeDir, &account.AccountID, func(client appServerClient) (rateLimitsReadResponse, error) {
		before, err := readRateLimits(ctx, client, "read Codex rate limits before reset failed")
		if err != nil {
			return rateLimitsReadResponse{}, err
		}
		var consumed consumeRateLimitResetCreditResponse
		if err := client.Call(ctx, "account/rateLimitResetCredit/consume", consumeRateLimitResetCreditParams{
			IdempotencyKey: idempotencyKey,
			CreditID:       earliestExpiringAvailableCreditID(before.RateLimitResetCredits),
		}, &consumed); err != nil {
			return rateLimitsReadResponse{}, mapAppServerError("consume Codex rate limit reset credit failed", err)
		}
		if !validRateLimitResetOutcome(consumed.Outcome) {
			return rateLimitsReadResponse{}, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "Codex 返回了未知的额度重置结果")
		}
		outcome = consumed.Outcome
		return readRateLimits(ctx, client, "read Codex rate limits after reset failed")
	})
	if err != nil {
		return provider.RateLimitResetResult{}, err
	}
	if err := providerImpl.saveRefreshedCredentials(ctx, account.StorageID, runtimeDir); err != nil {
		return provider.RateLimitResetResult{}, err
	}
	return provider.RateLimitResetResult{Outcome: outcome, Account: refreshedAccount, Usage: snapshot}, nil
}

func (providerImpl *Provider) refreshCodexHome(ctx context.Context, codexHome string, expectedAccountID string) (*entity.Account, *entity.UsageSnapshot, error) {
	var expectedID *string
	if expectedAccountID != "" {
		expectedID = &expectedAccountID
	}
	return providerImpl.fetchCodexUsage(ctx, codexHome, expectedID, func(client appServerClient) (rateLimitsReadResponse, error) {
		return readRateLimits(ctx, client, "read Codex rate limits failed")
	})
}

func (providerImpl *Provider) fetchCodexUsage(ctx context.Context, codexHome string, expectedAccountID *string, readLimits func(appServerClient) (rateLimitsReadResponse, error)) (*entity.Account, *entity.UsageSnapshot, error) {
	account, limits, err := providerImpl.readAccountAndRateLimits(ctx, codexHome, expectedAccountID, readLimits)
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := mapUsageSnapshot(*account, limits, providerImpl.now().UTC().UnixMilli())
	if err != nil {
		return nil, nil, err
	}
	return account, snapshot, nil
}

// readAccountAndRateLimits 在关闭 app-server 后返回，调用方随后才能保存刷新后的凭据。
func (providerImpl *Provider) readAccountAndRateLimits(ctx context.Context, codexHome string, expectedAccountID *string, readLimits func(appServerClient) (rateLimitsReadResponse, error)) (_ *entity.Account, _ rateLimitsReadResponse, err error) {
	client, err := providerImpl.startClient(ctx, codexHome)
	if err != nil {
		return nil, rateLimitsReadResponse{}, err
	}
	defer func() {
		closeErr := client.Close(context.Background())
		if err == nil && closeErr != nil {
			err = entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "关闭 Codex app-server 失败", closeErr)
		}
	}()

	account, err := providerImpl.readAccount(ctx, client, false)
	if err != nil {
		return nil, rateLimitsReadResponse{}, err
	}
	if expectedAccountID != nil && account.AccountID != *expectedAccountID {
		return nil, rateLimitsReadResponse{}, entity.NewAppErrorWithMessage(entity.ErrorCodeConflict, "auth.json 对应账号与当前账号不一致")
	}
	limits, err := readLimits(client)
	if err != nil {
		return nil, rateLimitsReadResponse{}, err
	}
	if limits.RateLimits.PlanType != nil {
		planType := strings.TrimSpace(*limits.RateLimits.PlanType)
		if planType != "" {
			account.PlanType = &planType
		}
	}
	return account, limits, nil
}

func earliestExpiringAvailableCreditID(summary *rateLimitResetCreditsSummary) string {
	if summary == nil {
		return ""
	}

	var selectedID string
	var selectedExpiresAt int64
	for _, credit := range summary.Credits {
		creditID := strings.TrimSpace(credit.ID)
		if creditID == "" || !strings.EqualFold(strings.TrimSpace(credit.Status), "available") || credit.ExpiresAt <= 0 {
			continue
		}
		if selectedID == "" || credit.ExpiresAt < selectedExpiresAt || credit.ExpiresAt == selectedExpiresAt && creditID < selectedID {
			selectedID = creditID
			selectedExpiresAt = credit.ExpiresAt
		}
	}
	return selectedID
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
