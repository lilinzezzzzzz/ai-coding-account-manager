package controller

import (
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httpcontract"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
)

// HealthController 处理健康检查相关请求。
type HealthController struct{}

// NewHealthController 创建健康检查 controller。
func NewHealthController() HealthController {
	return HealthController{}
}

// GetHealth 返回当前服务健康状态。
func (HealthController) GetHealth(w http.ResponseWriter, _ *http.Request) error {
	httptransport.WriteOK(w, httpcontract.HealthResponse{Status: "ok"})
	return nil
}
