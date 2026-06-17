package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/appserver"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/credentials"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
)

func TestProviderDiscoversRefreshesAndActivatesAccount(t *testing.T) {
	tempDir := t.TempDir()
	activeDir := filepath.Join(tempDir, "active")
	writeAuthFile(t, activeDir, `{"tokens":{"access_token":"active"}}`)
	store := newTestStore(t, tempDir, activeDir)

	codexProvider := newTestProvider(t, store, activeDir, func(_ context.Context, cfg appserver.Config) (appServerClient, error) {
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

	account, err := codexProvider.DiscoverCurrentAccount(context.Background())
	if err != nil {
		t.Fatalf("discover current account: %v", err)
	}
	if account.ProviderID != providerID || account.AccountID == "" || account.StorageID == "" {
		t.Fatalf("mapped account = %+v", account)
	}
	if account.Email == nil || *account.Email != "user@example.com" {
		t.Fatalf("account email = %v", account.Email)
	}

	snapshot, err := codexProvider.RefreshAccount(context.Background(), *account)
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
	if err := codexProvider.ActivateAccount(context.Background(), *account); err != nil {
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

func TestProviderLoginTaskCompletesAndCleansPendingDir(t *testing.T) {
	tempDir := t.TempDir()
	activeDir := filepath.Join(tempDir, "active")
	writeAuthFile(t, activeDir, `{"tokens":{"access_token":"active"}}`)
	store := newTestStore(t, tempDir, activeDir)
	var pendingDir string

	codexProvider := newTestProvider(t, store, activeDir, func(_ context.Context, cfg appserver.Config) (appServerClient, error) {
		if cfg.CodexHome != activeDir {
			pendingDir = cfg.CodexHome
			writeAuthFile(t, pendingDir, `{"tokens":{"access_token":"new"}}`)
		}
		return &fakeCodexClient{responses: map[string]any{
			"account/login/start": loginStartResponse{
				Type:    "chatgpt",
				LoginID: "login-1",
				AuthURL: "https://auth.example.test/start",
			},
			"account/read": accountReadResponse{
				Account: &codexAccount{Type: "chatgpt", Email: "new@example.com", PlanType: "team"},
			},
		}}, nil
	})

	task, err := codexProvider.StartLogin(context.Background())
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	if task.ID != "login-1" || task.AuthURL == "" {
		t.Fatalf("task = %+v", task)
	}

	status, err := codexProvider.PollLogin(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("poll login: %v", err)
	}
	if status.State != provider.LoginStateCompleted || status.Account == nil {
		t.Fatalf("status = %+v", status)
	}
	if _, err := os.Stat(pendingDir); !os.IsNotExist(err) {
		t.Fatalf("pending dir still exists or stat failed: %v", err)
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

func newTestProvider(t *testing.T, store *credentials.Store, activeDir string, factory ClientFactory) *Provider {
	t.Helper()
	codexProvider, err := New(Config{
		CodexHome:     activeDir,
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
