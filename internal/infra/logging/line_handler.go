package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	traceIDFieldName = "trace_id"
	logTimeLayout    = "2006-01-02T15:04:05.000Z07:00"
	modulePathPrefix = "github.com/lilinzezzzzzz/ai-coding-account-manager/"
)

// LineHandler 将 slog 记录输出为易读单行日志，并避免在日志正文中输出参数名。
type LineHandler struct {
	mu      *sync.Mutex
	output  io.Writer
	options slog.HandlerOptions
	attrs   []slog.Attr
}

// NewLineHandler 创建单行日志 handler。
func NewLineHandler(output io.Writer, options *slog.HandlerOptions) *LineHandler {
	handlerOptions := slog.HandlerOptions{}
	if options != nil {
		handlerOptions = *options
	}
	return &LineHandler{
		mu:      &sync.Mutex{},
		output:  output,
		options: handlerOptions,
	}
}

// Enabled 实现 slog.Handler。
func (handler *LineHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= handler.options.Level.Level()
}

// Handle 实现 slog.Handler。
func (handler *LineHandler) Handle(_ context.Context, record slog.Record) error {
	values := []string{record.Message}
	traceID := ""
	for _, attr := range handler.attrs {
		handler.appendAttr(&values, &traceID, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		handler.appendAttr(&values, &traceID, attr)
		return true
	})
	if traceID != "" {
		values = append([]string{traceID}, values...)
	}

	line := fmt.Sprintf("%s | %-8s | %s - %s\n",
		formatLogTime(record.Time),
		record.Level.String(),
		logLocation(record.PC),
		strings.Join(values, " "),
	)

	handler.mu.Lock()
	defer handler.mu.Unlock()
	_, err := io.WriteString(handler.output, line)
	return err
}

// WithAttrs 实现 slog.Handler。
func (handler *LineHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *handler
	next.attrs = append(append([]slog.Attr{}, handler.attrs...), attrs...)
	return &next
}

// WithGroup 实现 slog.Handler。
func (handler *LineHandler) WithGroup(_ string) slog.Handler {
	return handler
}

func (handler *LineHandler) appendAttr(values *[]string, traceID *string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Key == traceIDFieldName {
		*traceID = attrValueString(attr.Value)
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		for _, groupAttr := range attr.Value.Group() {
			handler.appendAttr(values, traceID, groupAttr)
		}
		return
	}
	*values = append(*values, attrValueString(attr.Value))
}

func formatLogTime(value time.Time) string {
	if value.IsZero() {
		value = time.Now()
	}
	return value.Format(logTimeLayout)
}

func logLocation(pc uintptr) string {
	if pc == 0 {
		return "unknown:0"
	}
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	function := strings.TrimPrefix(frame.Function, modulePathPrefix)
	function = strings.ReplaceAll(function, "(*", "")
	function = strings.ReplaceAll(function, ")", "")
	if function == "" {
		function = "unknown"
	}
	return fmt.Sprintf("%s:%d", function, frame.Line)
}

func attrValueString(value slog.Value) string {
	switch value.Kind() {
	case slog.KindString:
		return sanitizeLogValue(value.String())
	case slog.KindBool:
		return fmt.Sprint(value.Bool())
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindFloat64:
		return fmt.Sprint(value.Float64())
	case slog.KindInt64:
		return fmt.Sprint(value.Int64())
	case slog.KindTime:
		return value.Time().Format(logTimeLayout)
	case slog.KindUint64:
		return fmt.Sprint(value.Uint64())
	default:
		return sanitizeLogValue(fmt.Sprint(value.Any()))
	}
}

func sanitizeLogValue(value string) string {
	value = strings.ReplaceAll(value, "\r", `\r`)
	return strings.ReplaceAll(value, "\n", `\n`)
}
