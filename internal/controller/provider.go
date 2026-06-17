package controller

import (
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

type providerResponse struct {
	ID           string             `json:"id"`
	DisplayName  string             `json:"displayName"`
	Capabilities capabilityResponse `json:"capabilities"`
	Status       provider.Status    `json:"status"`
	ErrorCode    *entity.ErrorCode  `json:"errorCode"`
}

type capabilityResponse struct {
	CanImportCurrentAccount           bool `json:"canImportCurrentAccount"`
	CanLogin                          bool `json:"canLogin"`
	CanRefreshUsage                   bool `json:"canRefreshUsage"`
	CanActivateAccount                bool `json:"canActivateAccount"`
	RequiresClientReloadAfterActivate bool `json:"requiresClientReloadAfterActivate"`
}

// ProviderController 处理 provider 查询 API。
type ProviderController struct {
	providers service.ProviderService
}

// NewProviderController 创建 provider controller。
func NewProviderController(providers service.ProviderService) ProviderController {
	return ProviderController{providers: providers}
}

// ListProviders 返回 provider 列表。
func (controller ProviderController) ListProviders(w http.ResponseWriter, r *http.Request) error {
	descriptions := controller.providers.ListProviders(r.Context())
	response := make([]providerResponse, 0, len(descriptions))
	for _, description := range descriptions {
		response = append(response, providerToResponse(description))
	}
	httptransport.WriteOK(w, response)
	return nil
}

func providerToResponse(description provider.Description) providerResponse {
	return providerResponse{
		ID:          description.ID,
		DisplayName: description.DisplayName,
		Capabilities: capabilityResponse{
			CanImportCurrentAccount:           description.Capabilities.CanImportCurrentAccount,
			CanLogin:                          description.Capabilities.CanLogin,
			CanRefreshUsage:                   description.Capabilities.CanRefreshUsage,
			CanActivateAccount:                description.Capabilities.CanActivateAccount,
			RequiresClientReloadAfterActivate: description.Capabilities.RequiresClientReloadAfterActivate,
		},
		Status:    description.Status,
		ErrorCode: description.ErrorCode,
	}
}
