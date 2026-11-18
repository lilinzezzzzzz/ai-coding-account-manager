package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/appserver"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

func (providerImpl *Provider) readAccount(ctx context.Context, client appServerClient, refreshToken bool) (*entity.Account, error) {
	var response accountReadResponse
	err := client.Call(ctx, "account/read", accountReadParams{RefreshToken: refreshToken}, &response)
	if err != nil {
		return nil, mapAppServerError("read Codex account", err)
	}
	return mapAccount(response)
}

func readRateLimits(ctx context.Context, client appServerClient, errorMessage string) (rateLimitsReadResponse, error) {
	var response rateLimitsReadResponse
	if err := client.Call(ctx, "account/rateLimits/read", map[string]any{}, &response); err != nil {
		return rateLimitsReadResponse{}, mapAppServerError(errorMessage, err)
	}
	return response, nil
}

func mapAccount(response accountReadResponse) (*entity.Account, error) {
	if response.Account == nil {
		if response.RequiresOpenaiAuth {
			return nil, entity.NewAppError(entity.ErrorCodeReauthenticationRequired)
		}
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
		code := entity.ErrorCodeUnavailable
		if isReauthenticationRequiredUpstreamCode(upstream.Code) {
			code = entity.ErrorCodeReauthenticationRequired
		}
		return entity.WrapAppErrorWithUpstreamError(code, upstream.Code, upstream.Message, err)
	}
	return entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, message, err)
}

func isReauthenticationRequiredUpstreamCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "token_expired",
		"token_invalidated",
		"token_revoked",
		"refresh_token_expired",
		"refresh_token_reused",
		"refresh_token_invalidated":
		return true
	default:
		return false
	}
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
	AvailableCount int64                  `json:"availableCount"`
	Credits        []rateLimitResetCredit `json:"credits"`
}

type rateLimitResetCredit struct {
	ID          string  `json:"id"`
	ResetType   string  `json:"resetType"`
	Status      string  `json:"status"`
	GrantedAt   int64   `json:"grantedAt"`
	ExpiresAt   int64   `json:"expiresAt"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

type consumeRateLimitResetCreditParams struct {
	IdempotencyKey string `json:"idempotencyKey"`
	CreditID       string `json:"creditId,omitempty"`
}

type consumeRateLimitResetCreditResponse struct {
	Outcome provider.RateLimitResetOutcome `json:"outcome"`
}
