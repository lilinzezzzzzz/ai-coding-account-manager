package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupLoggerUsesConfiguredRotatingFile(t *testing.T) {
	previousLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	path := filepath.Join(t.TempDir(), "logs", "server.log")
	t.Setenv(logFileEnv, path)

	logger, logFile, err := setupLogger()
	if err != nil {
		t.Fatalf("setupLogger() error = %v", err)
	}
	logger.Info("test log")
	if err := logFile.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(content)
	if !strings.Contains(got, " | INFO     | ") || !strings.Contains(got, " - system test log") {
		t.Fatalf("log content = %q, want line log with trace ID", got)
	}
	if strings.Contains(got, "time=") || strings.Contains(got, "level=") ||
		strings.Contains(got, "trace_id=") || strings.Contains(got, "msg=") {
		t.Fatalf("log content = %q, want values without field names", got)
	}
}
