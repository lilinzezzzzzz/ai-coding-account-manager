package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

type responseEnvelope struct {
	Data  any            `json:"data"`
	Error *errorResponse `json:"error"`
}

type errorResponse struct {
	Code    entity.ErrorCode `json:"code"`
	Message string           `json:"message"`
}

// WriteOK 写入统一成功响应。
func WriteOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, responseEnvelope{
		Data:  data,
		Error: nil,
	})
}

// WriteError 将业务错误映射为统一错误响应。
func WriteError(w http.ResponseWriter, err error) {
	appErr := normalizeError(err)
	writeJSON(w, http.StatusOK, responseEnvelope{
		Data: nil,
		Error: &errorResponse{
			Code:    appErr.ErrorCode(),
			Message: appErr.DisplayMessage(),
		},
	})
}

func normalizeError(err error) *entity.AppError {
	if err == nil {
		return entity.NewAppError(entity.ErrorCodeInternal)
	}

	appErr, ok := entity.AsAppError(err)
	if !ok {
		return entity.WrapAppError(entity.ErrorCodeInternal, err)
	}
	return appErr
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write json response failed", "error", err)
	}
}
