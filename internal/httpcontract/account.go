package httpcontract

import (
	"encoding/json"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
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
	PlanExpiresAt *int64                 `json:"planExpiresAt"`
	IsActive      bool                   `json:"isActive"`
	CreatedAt     int64                  `json:"createdAt"`
	UpdatedAt     int64                  `json:"updatedAt"`
	LastUsedAt    *int64                 `json:"lastUsedAt"`
	Usage         *UsageSnapshotResponse `json:"usage"`
}

// UsageSnapshotResponse 是 usage snapshot 的 HTTP response。
type UsageSnapshotResponse struct {
	Status       entity.UsageStatus `json:"status"`
	UsedPercent  *float64           `json:"usedPercent"`
	ResetsAt     *int64             `json:"resetsAt"`
	SnapshotJSON *string            `json:"snapshotJson"`
	ErrorCode    *entity.ErrorCode  `json:"errorCode"`
	RefreshedAt  int64              `json:"refreshedAt"`
}

// UpdatePlanExpirationRequest 是更新人工维护套餐到期时间的 HTTP request。
type UpdatePlanExpirationRequest struct {
	PlanExpiresAt json.RawMessage `json:"planExpiresAt"`
}

// NormalizedPlanExpiresAt 返回已校验和标准化的套餐到期时间。
func (request UpdatePlanExpirationRequest) NormalizedPlanExpiresAt() (*int64, error) {
	if request.PlanExpiresAt == nil {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "planExpiresAt 字段不能为空")
	}
	if string(request.PlanExpiresAt) == "null" {
		return nil, nil
	}
	var value int64
	if err := json.Unmarshal(request.PlanExpiresAt, &value); err != nil {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "planExpiresAt 必须为空或正整数时间戳")
	}
	if value <= 0 {
		return nil, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "planExpiresAt 必须为空或正整数时间戳")
	}
	if value < 100000000000 {
		value *= 1000
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

// ImportAccountAuthJSONRequest 是为已有账号导入 auth.json 的 HTTP request。
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

// RefreshResultResponse 是单账号状态刷新结果的 HTTP response。
type RefreshResultResponse struct {
	ProviderID   string            `json:"providerId"`
	AccountID    string            `json:"accountId"`
	Account      *AccountResponse  `json:"account"`
	ErrorCode    *entity.ErrorCode `json:"errorCode"`
	ErrorMessage *string           `json:"errorMessage"`
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
		PlanExpiresAt: account.PlanExpiresAt,
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
		Status:       usage.Status,
		UsedPercent:  usage.UsedPercent,
		ResetsAt:     usage.ResetsAt,
		SnapshotJSON: usage.SnapshotJSON,
		ErrorCode:    usage.ErrorCode,
		RefreshedAt:  usage.RefreshedAt,
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
