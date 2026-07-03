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
	if got := string(content); !strings.Contains(got, `msg="test log"`) || !strings.Contains(got, "trace_id=system") {
		t.Fatalf("log content = %q, want text log with trace ID", got)
	}
}
