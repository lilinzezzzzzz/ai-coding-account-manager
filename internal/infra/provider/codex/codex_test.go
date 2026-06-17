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

	codexProvider := newTestProvider(t, store, func(_ context.Context, cfg appserver.Config) (appServerClient, error) {
		return &fakeCodexClient{responses: map[string]any{
			"account/read": accountReadResponse{
				Account: &codexAccount{Type: "chatgpt", Email: "user@example.com", PlanType: "plus"},
			},
			"account/rateLimits/read": rateLimitsReadResponse{
				RateLimits: rateLimitSnapshot{Primary: &rateLimitWindow{
					UsedPercent: floatPtr(42.5),
					ResetsAt:    int64Ptr(1700000000000),
				}},
			},
		}}, nil
	})

	snapshot, err := codexProvider.RefreshAccount(context.Background(), account)
	if err != nil {
		t.Fatalf("refresh account: %v", err)
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

type fakeCodexClient struct {
	responses map[string]any
	closed    bool
}

func (client *fakeCodexClient) Call(_ context.Context, method string, _ any, result any) error {
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
