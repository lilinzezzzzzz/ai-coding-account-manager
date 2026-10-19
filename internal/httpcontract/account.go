package httpcontract

import (
	"encoding/json"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

const maxAuthJSONBytes = 2 * 1024 * 1024

// AccountResponse 是账号列表和账号操作返回的 HTTP response。
type AccountResponse struct {
	ProviderID    string                 `json:"providerId"`
	AccountID     string                 `json:"accountId"`
	StorageID     string                 `json:"storageId"`
	Label         string                 `json:"label"`
	Email         *string                `json:"email"`
	PlanType      *string                `json:"planType"`
	PlanAutoRenew bool                   `json:"planAutoRenew"`
	PlanDate      *int64                 `json:"planDate"`
	IsActive      bool                   `json:"isActive"`
	CreatedAt     int64                  `json:"createdAt"`
	UpdatedAt     int64                  `json:"updatedAt"`
	LastUsedAt    *int64                 `json:"lastUsedAt"`
	Usage         *UsageSnapshotResponse `json:"usage"`
}

// UsageSnapshotResponse 是 usage snapshot 的 HTTP response。
type UsageSnapshotResponse struct {
	Status                entity.UsageStatus             `json:"status"`
	UsedPercent           *float64                       `json:"usedPercent"`
	ResetsAt              *int64                         `json:"resetsAt"`
	RateLimitResetCredits *RateLimitResetCreditsResponse `json:"rateLimitResetCredits"`
	SnapshotJSON          *string                        `json:"snapshotJson"`
	ErrorCode             *entity.ErrorCode              `json:"errorCode"`
	RefreshedAt           int64                          `json:"refreshedAt"`
}

// RateLimitResetCreditsResponse 是 Codex rate limit reset credits 摘要和可见明细。
type RateLimitResetCreditsResponse struct {
	AvailableCount int64                          `json:"availableCount"`
	Credits        []RateLimitResetCreditResponse `json:"credits,omitempty"`
}

// RateLimitResetCreditResponse 是单次 Codex rate limit reset credit 的展示信息。
type RateLimitResetCreditResponse struct {
	ID          string  `json:"id"`
	ResetType   string  `json:"resetType"`
	Status      string  `json:"status"`
	GrantedAt   int64   `json:"grantedAt"`
	ExpiresAt   int64   `json:"expiresAt"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

// UpdatePlanRequest 更新人工维护的自动续期状态和对应日期；日期可为空。
type UpdatePlanRequest struct {
	PlanAutoRenew json.RawMessage `json:"planAutoRenew"`
	PlanDate      json.RawMessage `json:"planDate"`
}

// NormalizedPlanAutoRenew 校验布尔值，省略时默认自动续期。
func (request UpdatePlanRequest) NormalizedPlanAutoRenew() (bool, error) {
	if request.PlanAutoRenew == nil {
		return true, nil
	}
	var value bool
	if err := json.Unmarshal(request.PlanAutoRenew, &value); err != nil || strings.TrimSpace(string(request.PlanAutoRenew)) == "null" {
		return false, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "planAutoRenew 必须为布尔值")
	}
	return value, nil
}

// NormalizedPlanDate 返回用户录入的 Unix 毫秒时间戳。
func (request UpdatePlanRequest) NormalizedPlanDate() (*int64, error) {
	return parsePlanTimestampMillis(request.PlanDate, "planDate")
}

func parsePlanTimestampMillis(raw json.RawMessage, field string) (*int64, error) {
	if raw == nil {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, field+" 字段不能为空")
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, field+" 必须为空或正整数毫秒时间戳")
	}
	if value <= 0 {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, field+" 必须为空或正整数毫秒时间戳")
	}
	return &value, nil
}

// CreateAccountRequest 是根据 OpenAI 邮箱新增账号的 HTTP request。
type CreateAccountRequest struct {
	Email string `json:"email"`
}

// NormalizedEmail 返回已校验和清理的 OpenAI 账号邮箱。
func (request CreateAccountRequest) NormalizedEmail() (string, error) {
	email := strings.TrimSpace(request.Email)
	if email == "" || len(email) > 254 || !strings.Contains(email, "@") {
		return "", entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "email 无效")
	}
	return email, nil
}

// ImportAccountAuthJSONRequest 是导入 auth.json 的 HTTP request。
type ImportAccountAuthJSONRequest struct {
	AuthJSON string `json:"authJson"`
}

// NormalizedAuthJSON 返回已校验的 auth.json 内容。
func (request ImportAccountAuthJSONRequest) NormalizedAuthJSON() ([]byte, error) {
	authJSON := strings.TrimSpace(request.AuthJSON)
	if authJSON == "" {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "auth.json 内容不能为空")
	}
	if len(authJSON) > maxAuthJSONBytes {
		return nil, entity.NewAppError(entity.ErrorCodePayloadTooLarge)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal([]byte(authJSON), &value); err != nil {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "auth.json 不是有效 JSON")
	}
	if len(value) == 0 {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "auth.json 内容不能为空对象")
	}
	return []byte(authJSON), nil
}

// ResetRateLimitRequest 是消费 rate limit reset credit 的 HTTP request。
type ResetRateLimitRequest struct {
	IdempotencyKey string `json:"idempotencyKey"`
}

// NormalizedIdempotencyKey 返回已校验的幂等键。
func (request ResetRateLimitRequest) NormalizedIdempotencyKey() (string, error) {
	idempotencyKey := strings.TrimSpace(request.IdempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 128 {
		return "", entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "idempotencyKey 无效")
	}
	return idempotencyKey, nil
}

// ResetRateLimitResponse 是账号额度重置结果的 HTTP response。
type ResetRateLimitResponse struct {
	Outcome provider.RateLimitResetOutcome `json:"outcome"`
	Account AccountResponse                `json:"account"`
}

// RefreshResultResponse 是单账号状态刷新结果的 HTTP response。
type RefreshResultResponse struct {
	ProviderID   string           `json:"providerId"`
	AccountID    string           `json:"accountId"`
	Account      *AccountResponse `json:"account"`
	ErrorCode    *string          `json:"errorCode"`
	ErrorMessage *string          `json:"errorMessage"`
}

// AccountViewResponse 将 service 账号视图转换为 HTTP response。
func AccountViewResponse(view service.AccountWithUsage) AccountResponse {
	return AccountEntityResponse(view.Account, view.Usage)
}

// AccountEntityResponse 将账号实体和 usage 转换为 HTTP response。
func AccountEntityResponse(account entity.Account, usage *entity.UsageSnapshot) AccountResponse {
	response := AccountResponse{
		ProviderID:    account.ProviderID,
		AccountID:     account.AccountID,
		StorageID:     account.StorageID,
		Label:         account.Label,
		Email:         account.Email,
		PlanType:      account.PlanType,
		PlanAutoRenew: account.PlanAutoRenew,
		PlanDate:      account.PlanDate,
		IsActive:      account.IsActive,
		CreatedAt:     account.CreatedAt,
		UpdatedAt:     account.UpdatedAt,
		LastUsedAt:    account.LastUsedAt,
	}
	if usage != nil {
		usageResponse := UsageSnapshotHTTPResponse(*usage)
		response.Usage = &usageResponse
	}
	return response
}

// UsageSnapshotHTTPResponse 将 usage snapshot 转换为 HTTP response。
func UsageSnapshotHTTPResponse(usage entity.UsageSnapshot) UsageSnapshotResponse {
	return UsageSnapshotResponse{
		Status:                usage.Status,
		UsedPercent:           usage.UsedPercent,
		ResetsAt:              usage.ResetsAt,
		RateLimitResetCredits: rateLimitResetCreditsResponse(usage.SnapshotJSON),
		SnapshotJSON:          usage.SnapshotJSON,
		ErrorCode:             usage.ErrorCode,
		RefreshedAt:           usage.RefreshedAt,
	}
}

func rateLimitResetCreditsResponse(snapshotJSON *string) *RateLimitResetCreditsResponse {
	if snapshotJSON == nil || strings.TrimSpace(*snapshotJSON) == "" {
		return nil
	}
	var snapshot struct {
		RateLimitResetCredits *RateLimitResetCreditsResponse `json:"rateLimitResetCredits"`
	}
	if err := json.Unmarshal([]byte(*snapshotJSON), &snapshot); err != nil {
		return nil
	}
	return snapshot.RateLimitResetCredits
}

// ResetRateLimitHTTPResponse 将 service 重置结果转换为 HTTP response。
func ResetRateLimitHTTPResponse(result service.ResetRateLimitResult) ResetRateLimitResponse {
	return ResetRateLimitResponse{
		Outcome: result.Outcome,
		Account: AccountViewResponse(result.Account),
	}
}

// RefreshResultHTTPResponse 将 service 刷新结果转换为 HTTP response。
func RefreshResultHTTPResponse(result service.RefreshResult) RefreshResultResponse {
	response := RefreshResultResponse{
		ProviderID:   result.ProviderID,
		AccountID:    result.AccountID,
		ErrorCode:    result.ErrorCode,
		ErrorMessage: result.ErrorMessage,
	}
	if result.Account != nil {
		account := AccountViewResponse(*result.Account)
		response.Account = &account
	}
	return response
}
