// Package tracing 提供 HTTP 请求追踪与结构化日志关联能力。
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	// HeaderName 是 HTTP 请求和响应传递 Trace ID 的 header。
	HeaderName = "X-Trace-ID"
	// LogFieldName 是结构化日志中的 Trace ID 字段。
	LogFieldName  = "trace_id"
	systemTraceID = "system"
	maxTraceIDLen = 128
)

type contextKey string

const traceIDContextKey contextKey = "trace_id"

var fallbackSequence atomic.Uint64

// WithTraceID 将 Trace ID 写入 context。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDContextKey, traceID)
}

// TraceIDFromContext 返回 context 中的 Trace ID；非请求上下文返回 system。
func TraceIDFromContext(ctx context.Context) string {
	if ctx != nil {
		if traceID, ok := ctx.Value(traceIDContextKey).(string); ok && traceID != "" {
			return traceID
		}
	}
	return systemTraceID
}

// IsValidTraceID 限制外部 Trace ID 的长度和字符集，避免污染日志。
func IsValidTraceID(traceID string) bool {
	if len(traceID) == 0 || len(traceID) > maxTraceIDLen {
		return false
	}
	for _, char := range traceID {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '.' || char == ':' {
			continue
		}
		return false
	}
	return true
}

// NewTraceID 生成 128-bit 随机 Trace ID。系统随机源不可用时使用时间和原子序列兜底。
func NewTraceID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return strconv.FormatInt(time.Now().UTC().UnixNano(), 16) + "-" +
		strconv.FormatUint(fallbackSequence.Add(1), 16)
}

// Handler 为每条 slog 记录附加 trace_id。
type Handler struct {
	next slog.Handler
}

// NewHandler 包装 slog Handler，使日志可与 HTTP 请求关联。
func NewHandler(next slog.Handler) *Handler {
	return &Handler{next: next}
}

// Enabled 实现 slog.Handler。
func (handler *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.next.Enabled(ctx, level)
}

// Handle 实现 slog.Handler。
func (handler *Handler) Handle(ctx context.Context, record slog.Record) error {
	record.AddAttrs(slog.String(LogFieldName, TraceIDFromContext(ctx)))
	return handler.next.Handle(ctx, record)
}

// WithAttrs 实现 slog.Handler。
func (handler *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{next: handler.next.WithAttrs(attrs)}
}

// WithGroup 实现 slog.Handler。
func (handler *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: handler.next.WithGroup(name)}
}
