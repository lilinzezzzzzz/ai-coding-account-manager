package httptransport

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

type responseEnvelope struct {
	Data    any    `json:"data"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

const successCode = "SUCCESS"

// WriteOK 写入统一成功响应。
func WriteOK(ctx context.Context, w http.ResponseWriter, data any) {
	writeJSON(ctx, w, http.StatusOK, responseEnvelope{
		Data:    data,
		Code:    successCode,
		Message: "成功",
	})
}

// WriteError 将业务错误映射为统一错误响应。
func WriteError(ctx context.Context, w http.ResponseWriter, err error) {
	WriteErrorWithStatus(ctx, w, err, statusForError(err))
}

// WriteErrorWithStatus 将业务错误按指定 HTTP status 写为统一错误响应。
func WriteErrorWithStatus(ctx context.Context, w http.ResponseWriter, err error, statusCode int) {
	appErr := normalizeError(err)
	writeJSON(ctx, w, statusCode, responseEnvelope{
		Data:    nil,
		Code:    string(appErr.ErrorCode()),
		Message: appErr.DisplayMessage(),
	})
}

func statusForError(err error) int {
	appErr := normalizeError(err)
	switch appErr.ErrorCode() {
	case entity.ErrorCodeUnauthorized:
		return http.StatusUnauthorized
	case entity.ErrorCodeForbidden:
		return http.StatusForbidden
	case entity.ErrorCodeValidationFailed:
		return http.StatusBadRequest
	case entity.ErrorCodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case entity.ErrorCodeOperationInProgress:
		return http.StatusConflict
	default:
		return http.StatusOK
	}
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

func writeJSON(ctx context.Context, w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.ErrorContext(ctx, "write json response failed", "error", err)
	}
}
