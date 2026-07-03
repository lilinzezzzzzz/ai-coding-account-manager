package tracing

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestHandlerAddsRequestTraceID(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewHandler(slog.NewJSONHandler(&output, nil)))
	ctx := WithTraceID(context.Background(), "trace-test-123")

	logger.InfoContext(ctx, "request completed")

	if got := output.String(); !strings.Contains(got, `"trace_id":"trace-test-123"`) {
		t.Fatalf("log output = %s, want request trace ID", got)
	}
}

func TestHandlerAddsSystemTraceIDWithoutRequestContext(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewHandler(slog.NewJSONHandler(&output, nil)))

	logger.Info("server started")

	if got := output.String(); !strings.Contains(got, `"trace_id":"system"`) {
		t.Fatalf("log output = %s, want system trace ID", got)
	}
}

func TestIsValidTraceID(t *testing.T) {
	tests := []struct {
		name    string
		traceID string
		want    bool
	}{
		{name: "generated style", traceID: "0123456789abcdef", want: true},
		{name: "common delimiters", traceID: "web:request_1.2-3", want: true},
		{name: "empty", traceID: "", want: false},
		{name: "space", traceID: "trace id", want: false},
		{name: "newline", traceID: "trace\nid", want: false},
		{name: "too long", traceID: strings.Repeat("a", 129), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsValidTraceID(test.traceID); got != test.want {
				t.Fatalf("IsValidTraceID(%q) = %t, want %t", test.traceID, got, test.want)
			}
		})
	}
}
