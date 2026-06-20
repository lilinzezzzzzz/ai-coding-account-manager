package service

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/fake"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

func TestNormalizeUsageSnapshotConvertsResetsAtSecondsToMillis(t *testing.T) {
	resetsAtSeconds := int64(1700000000)
	snapshot := normalizeUsageSnapshot(entity.Account{
		ProviderID: "codex",
		AccountID:  "acct-1",
	}, entity.UsageSnapshot{
		ResetsAt: &resetsAtSeconds,
	})

	if snapshot.ResetsAt == nil || *snapshot.ResetsAt != 1700000000000 {
		t.Fatalf("resets at = %v, want milliseconds", snapshot.ResetsAt)
	}
}

func TestRefreshAccountLogsProviderFailure(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	ctx := context.Background()
	appDB, err := database.Open(ctx, database.Config{
		Path: filepath.Join(t.TempDir(), "state.db"),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() {
		if err := appDB.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()

	daos := dao.NewDAOs(appDB.GORM())
	registry := provider.NewRegistry()
	fakeProvider := fake.New(fake.Config{
		ID:          "codex",
		DisplayName: "Codex Fake",
		Capabilities: provider.Capabilities{
			CanRefreshUsage:    false,
			CanActivateAccount: true,
		},
	})
	if err := registry.Register(ctx, fakeProvider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	account := entity.Account{
		ProviderID: "codex",
		AccountID:  "acct-unsupported",
		StorageID:  entity.StorageIDForAccount("codex", "acct-unsupported"),
		Label:      "acct-unsupported",
		CreatedAt:  1000,
		UpdatedAt:  1000,
	}
	if err := daos.Accounts.Create(ctx, account); err != nil {
		t.Fatalf("seed account error = %v", err)
	}

	accountService := NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, registry)
	result, err := accountService.RefreshAccount(ctx, "codex", account.AccountID)
	if err != nil {
		t.Fatalf("RefreshAccount() error = %v", err)
	}
	if result.ErrorCode == nil || *result.ErrorCode != entity.ErrorCodeUnsupported {
		t.Fatalf("error code = %v, want unsupported", result.ErrorCode)
	}

	snapshot, err := daos.UsageSnapshots.Get(ctx, "codex", account.AccountID)
	if err != nil {
		t.Fatalf("Get usage snapshot error = %v", err)
	}
	if snapshot.Status != entity.UsageStatusUnavailable || snapshot.ErrorCode == nil || *snapshot.ErrorCode != entity.ErrorCodeUnsupported {
		t.Fatalf("snapshot = %+v, want unavailable unsupported", snapshot)
	}

	logOutput := logs.String()
	for _, want := range []string{
		"account refresh failed",
		`"provider_id":"codex"`,
		`"account_id":"acct-unsupported"`,
		`"error_code":"UNSUPPORTED"`,
	} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("log output = %s, want %s", logOutput, want)
		}
	}
	for _, forbidden := range []string{"access_token", "refresh_token", "auth.json"} {
		if strings.Contains(logOutput, forbidden) {
			t.Fatalf("log output leaked %q: %s", forbidden, logOutput)
		}
	}
}
