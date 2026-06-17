package httpcontract

// HealthResponse 是健康检查的 HTTP response。
type HealthResponse struct {
	Status string `json:"status"`
}
