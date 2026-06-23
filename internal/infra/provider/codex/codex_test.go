package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/appserver"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/credentials"
)

func TestProviderRefreshesAndActivatesAccount(t *testing.T) {
	tempDir := t.TempDir()
	activeDir := filepath.Join(tempDir, "active")
	writeAuthFile(t, activeDir, `{"tokens":{"access_token":"active"}}`)
	store := newTestStore(t, tempDir, activeDir)
	accountID := entity.AccountIDFromEmail("user@example.com")
	account := entity.Account{
		ProviderID: providerID,
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount(providerID, accountID),
		Label:      "user@example.com",
	}
	if err := store.ImportFromCodexDir(context.Background(), providerID, account.StorageID, activeDir); err != nil {
		t.Fatalf("import account credentials: %v", err)
	}

	fakeClient := &fakeCodexClient{responses: map[string]any{
		"account/read": accountReadResponse{
			Account: &codexAccount{Type: "chatgpt", Email: "user@example.com"},
		},
		"account/rateLimits/read": rateLimitsReadResponse{
			RateLimits: rateLimitSnapshot{
				Primary: &rateLimitWindow{
					UsedPercent: floatPtr(42.5),
					ResetsAt:    int64Ptr(1700000000000),
				},
				PlanType: stringPtr("plus"),
			},
		},
	}}
	codexProvider := newTestProvider(t, store, func(_ context.Context, cfg appserver.Config) (appServerClient, error) {
		return fakeClient, nil
	})

	refreshedAccount, snapshot, err := codexProvider.RefreshAccountWithMetadata(context.Background(), account)
	if err != nil {
		t.Fatalf("refresh account: %v", err)
	}
	fakeClient.assertAccountReadRefreshToken(t, false)
	if refreshedAccount.PlanType == nil || *refreshedAccount.PlanType != "plus" {
		t.Fatalf("plan type = %v, want plus", refreshedAccount.PlanType)
	}
	if refreshedAccount.PlanExpiresAt != nil {
		t.Fatalf("plan expires at = %v, want nil", refreshedAccount.PlanExpiresAt)
	}
	if snapshot.UsedPercent == nil || *snapshot.UsedPercent != 42.5 {
		t.Fatalf("used percent = %v", snapshot.UsedPercent)
	}
	if snapshot.SnapshotJSON == nil || *snapshot.SnapshotJSON == "" {
		t.Fatal("snapshot json is empty")
	}

	writeAuthFile(t, activeDir, `{"tokens":{"access_token":"old"}}`)
	if err := codexProvider.ActivateAccount(context.Background(), account); err != nil {
		t.Fatalf("activate account: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(activeDir, "auth.json"))
	if err != nil {
		t.Fatalf("read active auth: %v", err)
	}
	if string(content) != `{"tokens":{"access_token":"active"}}` {
		t.Fatalf("active auth content = %s", content)
	}
}

func TestProviderRejectsMismatchedAccountAuthOnRefresh(t *testing.T) {
	tempDir := t.TempDir()
	activeDir := filepath.Join(tempDir, "active")
	writeAuthFile(t, activeDir, `{"tokens":{"access_token":"active"}}`)
	store := newTestStore(t, tempDir, activeDir)
	accountID := entity.AccountIDFromEmail("user@example.com")
	account := entity.Account{
		ProviderID: providerID,
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount(providerID, accountID),
		Label:      "user@example.com",
	}
	if err := store.ImportFromCodexDir(context.Background(), providerID, account.StorageID, activeDir); err != nil {
		t.Fatalf("import account credentials: %v", err)
	}

	codexProvider := newTestProvider(t, store, func(context.Context, appserver.Config) (appServerClient, error) {
		return &fakeCodexClient{responses: map[string]any{
			"account/read": accountReadResponse{
				Account: &codexAccount{Type: "chatgpt", Email: "other@example.com", PlanType: "plus"},
			},
		}}, nil
	})

	_, _, err := codexProvider.RefreshAccountWithMetadata(context.Background(), account)
	if err == nil {
		t.Fatal("RefreshAccountWithMetadata accepted mismatched auth account")
	}
	appErr, ok := entity.AsAppError(err)
	if !ok || appErr.ErrorCode() != entity.ErrorCodeConflict {
		t.Fatalf("error = %v, want CONFLICT", err)
	}
}

func TestProviderImportsCurrentAccountAuth(t *testing.T) {
	tempDir := t.TempDir()
	activeDir := filepath.Join(tempDir, "active")
	writeAuthFile(t, activeDir, `{"tokens":{"access_token":"current"}}`)
	store := newTestStore(t, tempDir, activeDir)

	fakeClient := &fakeCodexClient{responses: map[string]any{
		"account/read": accountReadResponse{
			Account: &codexAccount{Type: "chatgpt", Email: "current@example.com", PlanType: "plus"},
		},
	}}
	codexProvider := newTestProvider(t, store, func(_ context.Context, cfg appserver.Config) (appServerClient, error) {
		if cfg.CodexHome != activeDir {
			t.Fatalf("CodexHome = %s, want active dir", cfg.CodexHome)
		}
		return fakeClient, nil
	})

	account, err := codexProvider.ImportCurrentAccount(context.Background())
	if err != nil {
		t.Fatalf("import current account: %v", err)
	}
	fakeClient.assertAccountReadRefreshToken(t, false)
	if account.AccountID != entity.AccountIDFromEmail("current@example.com") {
		t.Fatalf("account id = %s, want email-derived id", account.AccountID)
	}

	accountDir, err := store.AccountCodexDir(providerID, account.StorageID)
	if err != nil {
		t.Fatalf("account dir: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(accountDir, "auth.json"))
	if err != nil {
		t.Fatalf("read imported auth: %v", err)
	}
	if string(content) != `{"tokens":{"access_token":"current"}}` {
		t.Fatalf("imported auth content = %s", content)
	}
}

func TestProviderImportsAccountAuthJSON(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t, tempDir, filepath.Join(tempDir, "active"))
	codexProvider := newTestProvider(t, store, nil)
	accountID := entity.AccountIDFromEmail("raw@example.com")
	account := entity.Account{
		ProviderID: providerID,
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount(providerID, accountID),
		Label:      "raw@example.com",
	}

	authJSON := []byte(`{"tokens":{"access_token":"raw-secret"}}`)
	if err := codexProvider.ImportAccountAuthJSON(context.Background(), account, authJSON); err != nil {
		t.Fatalf("import account auth json: %v", err)
	}
	accountDir, err := store.AccountCodexDir(providerID, account.StorageID)
	if err != nil {
		t.Fatalf("account dir: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(accountDir, "auth.json"))
	if err != nil {
		t.Fatalf("read imported auth: %v", err)
	}
	if string(content) != string(authJSON) {
		t.Fatalf("imported auth content = %s, want %s", content, authJSON)
	}
}

func TestProviderAcceptsAccountWhenOpenAIAuthStillRequired(t *testing.T) {
	account, err := mapAccount(accountReadResponse{
		Account: &codexAccount{
			Type:     "chatgpt",
			Email:    "user@example.com",
			PlanType: "plus",
		},
		RequiresOpenaiAuth: true,
	})
	if err != nil {
		t.Fatalf("map account: %v", err)
	}
	if account.Email == nil || *account.Email != "user@example.com" {
		t.Fatalf("email = %v, want user@example.com", account.Email)
	}
}

type fakeCodexClient struct {
	responses map[string]any
	calls     []fakeCodexCall
	closed    bool
}

type fakeCodexCall struct {
	method string
	params any
}

func (client *fakeCodexClient) Call(_ context.Context, method string, params any, result any) error {
	client.calls = append(client.calls, fakeCodexCall{method: method, params: params})
	response, ok := client.responses[method]
	if !ok {
		return nil
	}
	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, result)
}

func (client *fakeCodexClient) Close(context.Context) error {
	client.closed = true
	return nil
}

func (client *fakeCodexClient) assertAccountReadRefreshToken(t *testing.T, want bool) {
	t.Helper()

	for _, call := range client.calls {
		if call.method != "account/read" {
			continue
		}
		params, ok := call.params.(accountReadParams)
		if !ok {
			t.Fatalf("account/read params type = %T, want accountReadParams", call.params)
		}
		if params.RefreshToken != want {
			t.Fatalf("account/read refreshToken = %v, want %v", params.RefreshToken, want)
		}
		return
	}
	t.Fatal("account/read was not called")
}

func newTestProvider(t *testing.T, store *credentials.Store, factory ClientFactory) *Provider {
	t.Helper()
	codexProvider, err := New(Config{
		Credentials:   store,
		ClientFactory: factory,
		Now: func() time.Time {
			return time.UnixMilli(1700000000000)
		},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return codexProvider
}

func newTestStore(t *testing.T, tempDir string, activeDir string) *credentials.Store {
	t.Helper()
	store, err := credentials.NewStore(credentials.Config{
		RootDir:        filepath.Join(tempDir, "credentials"),
		ActiveCodexDir: activeDir,
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	return store
}

func writeAuthFile(t *testing.T, dir string, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create auth dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
}

func floatPtr(value float64) *float64 {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func stringPtr(value string) *string {
	return &value
}
