package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLineHandlerWritesValuesWithoutFieldNames(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewLineHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.InfoContext(context.Background(),
		"account refreshed",
		"provider_id", "codex",
		"account_id", "chatgpt-9637eecd7f956ff97717",
		"usage_status", "rate_limit_reached",
		"trace_id", "96fcf47142ed4a473237cd286756e742",
	)

	got := strings.TrimSpace(output.String())
	if !strings.Contains(got, " | INFO     | ") {
		t.Fatalf("log output = %q, want padded level", got)
	}
	if !strings.Contains(got, "logging.TestLineHandlerWritesValuesWithoutFieldNames:") {
		t.Fatalf("log output = %q, want log location", got)
	}
	if !strings.Contains(got, " - 96fcf47142ed4a473237cd286756e742 account refreshed codex chatgpt-9637eecd7f956ff97717 rate_limit_reached") {
		t.Fatalf("log output = %q, want values after separator", got)
	}
	for _, name := range []string{"time=", "level=", "trace_id=", "msg=", "provider_id=", "account_id=", "usage_status="} {
		if strings.Contains(got, name) {
			t.Fatalf("log output = %q, want no field name %q", got, name)
		}
	}
}

func TestLineHandlerKeepsCurrentTimeFormat(t *testing.T) {
	var output bytes.Buffer
	record := slog.NewRecord(time.Date(2026, 7, 8, 22, 8, 51, 197_000_000, time.FixedZone("CST", 8*60*60)), slog.LevelInfo, "provider failed", 0)
	record.AddAttrs(slog.String("trace_id", "trace-1"))

	if err := NewLineHandler(&output, nil).Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	wantPrefix := "2026-07-08T22:08:51.197+08:00 | INFO     | unknown:0 - trace-1 provider failed"
	if got := strings.TrimSpace(output.String()); got != wantPrefix {
		t.Fatalf("log output = %q, want %q", got, wantPrefix)
	}
}
