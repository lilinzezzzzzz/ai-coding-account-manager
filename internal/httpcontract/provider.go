package httpcontract

import (
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

// ProviderResponse 是 provider 列表的 HTTP response。
type ProviderResponse struct {
	ID           string             `json:"id"`
	DisplayName  string             `json:"displayName"`
	Capabilities CapabilityResponse `json:"capabilities"`
	Status       provider.Status    `json:"status"`
	ErrorCode    *entity.ErrorCode  `json:"errorCode"`
}

// CapabilityResponse 是 provider 能力声明的 HTTP response。
type CapabilityResponse struct {
	CanRefreshUsage                   bool `json:"canRefreshUsage"`
	CanActivateAccount                bool `json:"canActivateAccount"`
	RequiresClientReloadAfterActivate bool `json:"requiresClientReloadAfterActivate"`
}

// ProviderHTTPResponse 将 provider 描述转换为 HTTP response。
func ProviderHTTPResponse(description provider.Description) ProviderResponse {
	return ProviderResponse{
		ID:          description.ID,
		DisplayName: description.DisplayName,
		Capabilities: CapabilityResponse{
			CanRefreshUsage:                   description.Capabilities.CanRefreshUsage,
			CanActivateAccount:                description.Capabilities.CanActivateAccount,
			RequiresClientReloadAfterActivate: description.Capabilities.RequiresClientReloadAfterActivate,
		},
		Status:    description.Status,
		ErrorCode: description.ErrorCode,
	}
}
