package controller

import (
	"log/slog"
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httpcontract"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

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
	response := make([]httpcontract.ProviderResponse, 0, len(descriptions))
	for _, description := range descriptions {
		if description.Status != "" && description.Status != provider.StatusAvailable {
			logProviderUnavailable(description.ID, description.ErrorCode)
		}
		response = append(response, httpcontract.ProviderHTTPResponse(description))
	}
	httptransport.WriteOK(w, response)
	return nil
}

func logProviderUnavailable(providerID string, code *entity.ErrorCode) {
	fields := []any{"provider_id", providerID}
	if code != nil {
		fields = append(fields, "error_code", *code)
	}
	slog.Warn("provider unavailable", fields...)
}
