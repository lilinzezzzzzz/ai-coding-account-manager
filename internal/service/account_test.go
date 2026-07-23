package service

import (
	"bytes"
	"context"
	"errors"
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

func TestResetAccountRateLimitSkipsProviderWhenNoCreditAvailable(t *testing.T) {
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
	account := entity.Account{
		ProviderID: "codex",
		AccountID:  "acct-no-credit",
		StorageID:  entity.StorageIDForAccount("codex", "acct-no-credit"),
		Label:      "acct-no-credit",
		CreatedAt:  1000,
		UpdatedAt:  1000,
	}
	if err := daos.Accounts.Create(ctx, account); err != nil {
		t.Fatalf("seed account error = %v", err)
	}
	snapshotJSON := `{"rateLimitResetCredits":{"availableCount":0}}`
	usage := entity.UsageSnapshot{
		ProviderID:   account.ProviderID,
		AccountID:    account.AccountID,
		Status:       entity.UsageStatusReady,
		SnapshotJSON: &snapshotJSON,
		RefreshedAt:  2000,
	}
	if err := daos.UsageSnapshots.Upsert(ctx, usage); err != nil {
		t.Fatalf("seed usage snapshot error = %v", err)
	}

	baseProvider := fake.New(fake.Config{
		ID:          "codex",
		DisplayName: "Codex Fake",
	})
	resetProvider := &trackingRateLimitResetProvider{Provider: baseProvider}
	registry := provider.NewRegistry()
	if err := registry.Register(ctx, resetProvider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	accountService := NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, registry, NewAccountActivationCoordinator())
	result, err := accountService.ResetAccountRateLimit(ctx, account.ProviderID, account.AccountID, "reset-attempt-1")
	if err != nil {
		t.Fatalf("ResetAccountRateLimit() error = %v", err)
	}
	if result.Outcome != provider.RateLimitResetOutcomeNoCredit {
		t.Fatalf("outcome = %q, want %q", result.Outcome, provider.RateLimitResetOutcomeNoCredit)
	}
	if resetProvider.resetCalls != 0 {
		t.Fatalf("provider reset calls = %d, want 0", resetProvider.resetCalls)
	}
	if result.Account.Usage == nil || result.Account.Usage.SnapshotJSON == nil || *result.Account.Usage.SnapshotJSON != snapshotJSON {
		t.Fatalf("result usage = %+v, want persisted no-credit snapshot", result.Account.Usage)
	}
}

func TestResetAccountRateLimitPersistsAuthExpiredOnReauthenticationError(t *testing.T) {
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
	account := entity.Account{
		ProviderID: "codex",
		AccountID:  "acct-reset-auth-expired",
		StorageID:  entity.StorageIDForAccount("codex", "acct-reset-auth-expired"),
		Label:      "acct-reset-auth-expired",
		CreatedAt:  1000,
		UpdatedAt:  1000,
	}
	if err := daos.Accounts.Create(ctx, account); err != nil {
		t.Fatalf("seed account error = %v", err)
	}
	snapshotJSON := `{"rateLimitResetCredits":{"availableCount":1}}`
	if err := daos.UsageSnapshots.Upsert(ctx, entity.UsageSnapshot{
		ProviderID:   account.ProviderID,
		AccountID:    account.AccountID,
		Status:       entity.UsageStatusReady,
		SnapshotJSON: &snapshotJSON,
		RefreshedAt:  2000,
	}); err != nil {
		t.Fatalf("seed usage snapshot error = %v", err)
	}

	baseProvider := fake.New(fake.Config{ID: "codex", DisplayName: "Codex Fake"})
	registry := provider.NewRegistry()
	if err := registry.Register(ctx, &failingRateLimitResetProvider{
		Provider: baseProvider,
		err:      entity.NewAppError(entity.ErrorCodeReauthenticationRequired),
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	accountService := NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, registry, NewAccountActivationCoordinator())
	_, err = accountService.ResetAccountRateLimit(ctx, account.ProviderID, account.AccountID, "reset-auth-expired")
	if !isAppErrorCode(err, entity.ErrorCodeReauthenticationRequired) {
		t.Fatalf("ResetAccountRateLimit() error = %v, want %s", err, entity.ErrorCodeReauthenticationRequired)
	}

	snapshot, err := daos.UsageSnapshots.Get(ctx, account.ProviderID, account.AccountID)
	if err != nil {
		t.Fatalf("Get usage snapshot error = %v", err)
	}
	if snapshot.Status != entity.UsageStatusAuthExpired || snapshot.ErrorCode == nil || *snapshot.ErrorCode != entity.ErrorCodeReauthenticationRequired {
		t.Fatalf("snapshot = %+v, want auth_expired reauthentication required", snapshot)
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

	accountService := NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, registry, NewAccountActivationCoordinator())
	result, err := accountService.RefreshAccount(ctx, "codex", account.AccountID)
	if err != nil {
		t.Fatalf("RefreshAccount() error = %v", err)
	}
	if result.ErrorCode == nil || *result.ErrorCode != string(entity.ErrorCodeUnsupported) {
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
		"account refresh started",
		"account refresh failed",
		`"provider_id":"codex"`,
		`"account_id":"acct-unsupported"`,
		`"error_code":"UNSUPPORTED"`,
	} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("log output = %s, want %s", logOutput, want)
		}
	}
	startedIndex := strings.Index(logOutput, "account refresh started")
	failedIndex := strings.Index(logOutput, "account refresh failed")
	if startedIndex < 0 || failedIndex < 0 || startedIndex > failedIndex {
		t.Fatalf("log output = %s, want refresh started before refresh failed", logOutput)
	}
	for _, forbidden := range []string{"access_token", "refresh_token", "auth.json"} {
		if strings.Contains(logOutput, forbidden) {
			t.Fatalf("log output leaked %q: %s", forbidden, logOutput)
		}
	}
}

func TestRefreshAccountPersistsAuthExpiredWithStableErrorCode(t *testing.T) {
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
	account := entity.Account{
		ProviderID: "codex",
		AccountID:  "acct-auth-expired",
		StorageID:  entity.StorageIDForAccount("codex", "acct-auth-expired"),
		Label:      "acct-auth-expired",
		CreatedAt:  1000,
		UpdatedAt:  1000,
	}
	if err := daos.Accounts.Create(ctx, account); err != nil {
		t.Fatalf("seed account error = %v", err)
	}

	baseProvider := fake.New(fake.Config{ID: "codex", DisplayName: "Codex Fake"})
	refreshErr := entity.WrapAppErrorWithUpstreamError(
		entity.ErrorCodeReauthenticationRequired,
		"token_revoked",
		"Encountered invalidated oauth token for user, failing request",
		nil,
	)
	registry := provider.NewRegistry()
	if err := registry.Register(ctx, &failingMetadataRefreshProvider{Provider: baseProvider, err: refreshErr}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	accountService := NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, registry, NewAccountActivationCoordinator())
	result, err := accountService.RefreshAccount(ctx, account.ProviderID, account.AccountID)
	if err != nil {
		t.Fatalf("RefreshAccount() error = %v", err)
	}
	if result.ErrorCode == nil || *result.ErrorCode != string(entity.ErrorCodeReauthenticationRequired) {
		t.Fatalf("error code = %v, want %s", result.ErrorCode, entity.ErrorCodeReauthenticationRequired)
	}

	snapshot, err := daos.UsageSnapshots.Get(ctx, account.ProviderID, account.AccountID)
	if err != nil {
		t.Fatalf("Get usage snapshot error = %v", err)
	}
	if snapshot.Status != entity.UsageStatusAuthExpired || snapshot.ErrorCode == nil || *snapshot.ErrorCode != entity.ErrorCodeReauthenticationRequired {
		t.Fatalf("snapshot = %+v, want auth_expired reauthentication required", snapshot)
	}

	logOutput := logs.String()
	for _, want := range []string{
		`"error_code":"REAUTHENTICATION_REQUIRED"`,
		`"upstream_error_code":"token_revoked"`,
	} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("log output = %s, want %s", logOutput, want)
		}
	}
}

func TestRefreshAccountSyncsOnlyAccountThatIsStillActive(t *testing.T) {
	tests := []struct {
		name                string
		switchDuringRefresh bool
		activateErr         error
		wantActivateCalls   int
		wantActive          bool
		wantErrorCode       entity.ErrorCode
	}{
		{name: "active account remains active", wantActivateCalls: 1, wantActive: true},
		{name: "active account switched during refresh", switchDuringRefresh: true, wantActivateCalls: 0, wantActive: false},
		{name: "active credential sync fails", activateErr: errors.New("write auth.json"), wantActivateCalls: 1, wantErrorCode: entity.ErrorCodeUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			appDB, err := database.Open(ctx, database.Config{Path: filepath.Join(t.TempDir(), "state.db")})
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			t.Cleanup(func() {
				if err := appDB.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			})

			daos := dao.NewDAOs(appDB.GORM())
			activeAccount := entity.Account{
				ProviderID: "codex",
				AccountID:  "acct-active",
				StorageID:  entity.StorageIDForAccount("codex", "acct-active"),
				Label:      "acct-active",
				IsActive:   true,
				CreatedAt:  1000,
				UpdatedAt:  1000,
			}
			otherAccount := entity.Account{
				ProviderID: "codex",
				AccountID:  "acct-other",
				StorageID:  entity.StorageIDForAccount("codex", "acct-other"),
				Label:      "acct-other",
				CreatedAt:  1000,
				UpdatedAt:  1000,
			}
			for _, account := range []entity.Account{activeAccount, otherAccount} {
				if err := daos.Accounts.Create(ctx, account); err != nil {
					t.Fatalf("seed account %s error = %v", account.AccountID, err)
				}
			}

			baseProvider := fake.New(fake.Config{
				ID:          "codex",
				DisplayName: "Codex Fake",
				Accounts: []fake.AccountState{{
					Account: activeAccount,
					Usage: entity.UsageSnapshot{
						ProviderID:  "codex",
						AccountID:   activeAccount.AccountID,
						Status:      entity.UsageStatusReady,
						RefreshedAt: 2000,
					},
				}},
			})
			trackingProvider := &trackingAccountActivationProvider{Provider: baseProvider, activateErr: test.activateErr}
			if test.switchDuringRefresh {
				trackingProvider.afterRefresh = func() {
					if err := daos.Accounts.SetActive(ctx, "codex", otherAccount.AccountID, 3000); err != nil {
						t.Fatalf("switch active account error = %v", err)
					}
				}
			}
			registry := provider.NewRegistry()
			if err := registry.Register(ctx, trackingProvider); err != nil {
				t.Fatalf("Register() error = %v", err)
			}

			accountService := NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, registry, NewAccountActivationCoordinator())
			result, err := accountService.RefreshAccount(ctx, activeAccount.ProviderID, activeAccount.AccountID)
			if err != nil {
				t.Fatalf("RefreshAccount() error = %v", err)
			}
			if trackingProvider.activateCalls != test.wantActivateCalls {
				t.Fatalf("activate calls = %d, want %d", trackingProvider.activateCalls, test.wantActivateCalls)
			}
			if test.wantErrorCode != "" {
				if result.ErrorCode == nil || *result.ErrorCode != string(test.wantErrorCode) || result.Account != nil {
					t.Fatalf("refresh result = %+v, want %s", result, test.wantErrorCode)
				}
				return
			}
			if result.ErrorCode != nil || result.Account == nil {
				t.Fatalf("refresh result = %+v, want success", result)
			}
			if result.Account.Account.IsActive != test.wantActive {
				t.Fatalf("result active = %t, want %t", result.Account.Account.IsActive, test.wantActive)
			}
		})
	}
}

func TestResponseErrorCodePrefersUpstreamCode(t *testing.T) {
	err := entity.WrapAppErrorWithUpstreamError(
		entity.ErrorCodeUnavailable,
		"token_invalidated",
		"Your authentication token has been invalidated. Please try signing in again.",
		nil,
	)

	code := responseErrorCodePtr(err)
	if code == nil || *code != "token_invalidated" {
		t.Fatalf("response error code = %v, want token_invalidated", code)
	}
	message := errorMessagePtr(err)
	if message == nil || *message != "Your authentication token has been invalidated. Please try signing in again." {
		t.Fatalf("response error message = %v", message)
	}
}

type trackingRateLimitResetProvider struct {
	provider.Provider
	resetCalls int
}

type trackingAccountActivationProvider struct {
	*fake.Provider
	afterRefresh  func()
	activateCalls int
	activateErr   error
}

func (tracking *trackingAccountActivationProvider) RefreshAccountWithMetadata(ctx context.Context, account entity.Account) (*entity.Account, *entity.UsageSnapshot, error) {
	refreshedAccount, snapshot, err := tracking.Provider.RefreshAccountWithMetadata(ctx, account)
	if err == nil && tracking.afterRefresh != nil {
		tracking.afterRefresh()
	}
	return refreshedAccount, snapshot, err
}

func (tracking *trackingAccountActivationProvider) ActivateAccount(context.Context, entity.Account) error {
	tracking.activateCalls++
	return tracking.activateErr
}

type failingMetadataRefreshProvider struct {
	provider.Provider
	err error
}

func (failing *failingMetadataRefreshProvider) RefreshAccountWithMetadata(context.Context, entity.Account) (*entity.Account, *entity.UsageSnapshot, error) {
	return nil, nil, failing.err
}

type failingRateLimitResetProvider struct {
	provider.Provider
	err error
}

func (failing *failingRateLimitResetProvider) ResetAccountRateLimit(context.Context, entity.Account, string) (provider.RateLimitResetResult, error) {
	return provider.RateLimitResetResult{}, failing.err
}

func (tracking *trackingRateLimitResetProvider) ResetAccountRateLimit(_ context.Context, account entity.Account, _ string) (provider.RateLimitResetResult, error) {
	tracking.resetCalls++
	return provider.RateLimitResetResult{
		Outcome: provider.RateLimitResetOutcomeReset,
		Account: &account,
	}, nil
}
