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
	writeJSON(w, statusCodeForError(appErr.Code), responseEnvelope{
		Data: nil,
		Error: &errorResponse{
			Code:    appErr.Code,
			Message: appErr.Message,
		},
	})
}

func normalizeError(err error) *entity.AppError {
	if err == nil {
		return entity.NewAppError(entity.ErrorCodeInternal, "服务内部错误")
	}

	appErr, ok := entity.AsAppError(err)
	if !ok {
		return entity.WrapAppError(entity.ErrorCodeInternal, "服务内部错误", err)
	}
	normalized := *appErr
	if normalized.Code == "" {
		normalized.Code = entity.ErrorCodeInternal
	}
	if normalized.Message == "" {
		normalized.Message = defaultMessageForCode(normalized.Code)
	}
	return &normalized
}

func statusCodeForError(code entity.ErrorCode) int {
	switch code {
	case entity.ErrorCodeValidationFailed:
		return http.StatusBadRequest
	case entity.ErrorCodeNotFound:
		return http.StatusNotFound
	case entity.ErrorCodeMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case entity.ErrorCodeUnsupported:
		return http.StatusNotImplemented
	case entity.ErrorCodeConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func defaultMessageForCode(code entity.ErrorCode) string {
	switch code {
	case entity.ErrorCodeValidationFailed:
		return "请求参数无效"
	case entity.ErrorCodeNotFound:
		return "资源不存在"
	case entity.ErrorCodeMethodNotAllowed:
		return "请求方法不支持"
	case entity.ErrorCodeUnsupported:
		return "当前操作不支持"
	case entity.ErrorCodeConflict:
		return "资源状态冲突"
	default:
		return "服务内部错误"
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write json response failed", "error", err)
	}
}
