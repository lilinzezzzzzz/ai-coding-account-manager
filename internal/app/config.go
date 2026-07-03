package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/config"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/logging"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/tracing"
)

const logFileEnv = "AI_CODING_ACCOUNT_MANAGER_LOG_FILE"

func setupLogger() (*slog.Logger, io.Closer, error) {
	// 进程入口先建立默认 logger，后续启动失败也能输出结构化错误。
	fallbackLogger := newLogger(os.Stderr)
	slog.SetDefault(fallbackLogger)

	logFile := strings.TrimSpace(os.Getenv(logFileEnv))
	if logFile == "" {
		return fallbackLogger, nil, nil
	}
	writer, err := logging.NewDailyWriter(logFile)
	if err != nil {
		return nil, nil, fmt.Errorf("setup rotating log file: %w", err)
	}
	logger := newLogger(writer)
	slog.SetDefault(logger)
	return logger, writer, nil
}

func newLogger(output io.Writer) *slog.Logger {
	return slog.New(tracing.NewHandler(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
}

func loadConfig(args []string) (config.Config, error) {
	flags := flag.NewFlagSet("ai-coding-account-manager", flag.ContinueOnError)
	configFile := flags.String("config", "", "配置文件路径")
	if err := flags.Parse(args); err != nil {
		return config.Config{}, err
	}

	cfg, err := config.LoadFile(*configFile)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func ensureRuntimeDirs(cfg config.Config) error {
	if err := os.MkdirAll(cfg.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	return nil
}

func openDatabase(cfg config.Config) (*database.DB, error) {
	appDB, err := database.Open(context.Background(), database.Config{
		Path: filepath.Join(cfg.DataDir, "state.db"),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return appDB, nil
}

func closeDatabase(appDB *database.DB, logger *slog.Logger) {
	if err := appDB.Close(); err != nil {
		logger.Error("close database failed", "error", err)
	}
}
