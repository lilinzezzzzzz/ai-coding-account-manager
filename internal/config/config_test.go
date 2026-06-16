package config_test

import (
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/config"
)

func TestLoadUsesDefaultBindAddr(t *testing.T) {
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_BIND_ADDR", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BindAddr != "127.0.0.1:43127" {
		t.Fatalf("BindAddr = %q, want 127.0.0.1:43127", cfg.BindAddr)
	}
}

func TestLoadRejectsNonLoopbackBindAddr(t *testing.T) {
	t.Setenv("AI_CODING_ACCOUNT_MANAGER_BIND_ADDR", "0.0.0.0:43127")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}
