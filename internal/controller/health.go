package controller

import (
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/transport/httpapi"
)

type healthResponse struct {
	Status string `json:"status"`
}

// HealthController 处理健康检查相关请求。
type HealthController struct{}

// NewHealthController 创建健康检查 controller。
func NewHealthController() HealthController {
	return HealthController{}
}

// Show 返回当前服务健康状态。
func (HealthController) Show(w http.ResponseWriter, _ *http.Request) {
	httpapi.WriteOK(w, healthResponse{Status: "ok"})
}
