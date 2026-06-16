package controller

import (
	"encoding/json"
	"net/http"
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
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
