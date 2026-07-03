package middleware

import (
	"net/http"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/tracing"
)

// TraceRequest 为每个 HTTP 请求建立 Trace ID，并在响应 header 中返回。
func TraceRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := strings.TrimSpace(r.Header.Get(tracing.HeaderName))
		if !tracing.IsValidTraceID(traceID) {
			traceID = tracing.NewTraceID()
		}

		if r.Header == nil {
			r.Header = make(http.Header)
		}
		r.Header.Set(tracing.HeaderName, traceID)
		w.Header().Set(tracing.HeaderName, traceID)
		next.ServeHTTP(w, r.WithContext(tracing.WithTraceID(r.Context(), traceID)))
	})
}
