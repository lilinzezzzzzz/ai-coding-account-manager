package httptransport

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

// HandlerFunc 是返回业务错误的 HTTP API handler。
type HandlerFunc func(http.ResponseWriter, *http.Request) error

// Handle 将返回 error 的 API handler 适配成标准 http.HandlerFunc。
func Handle(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			logRequestFailure(r, err)
			WriteError(r.Context(), w, err)
			return
		}
	}
}

func logRequestFailure(r *http.Request, err error) {
	appErr := normalizeError(err)
	fields := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"error_code", appErr.ErrorCode(),
	}
	if appErr.Cause != nil {
		fields = append(fields, "cause_type", fmt.Sprintf("%T", appErr.Cause))
	}

	if appErr.ErrorCode() == entity.ErrorCodeInternal {
		slog.ErrorContext(r.Context(), "api request failed", fields...)
		return
	}
	slog.WarnContext(r.Context(), "api request rejected", fields...)
}
