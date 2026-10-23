package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/credentials"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/codex"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/router"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

func TestAccountAuthJSONExport(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := credentials.NewStore(credentials.Config{
		RootDir: filepath.Join(dir, "credentials"), ActiveCodexDir: filepath.Join(dir, "active"),
	})
	if err != nil {
		t.Fatal(err)
	}
	appDB, err := database.Open(ctx, database.Config{Path: filepath.Join(dir, "state.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = appDB.Close() })
	daos := dao.NewDAOs(appDB.GORM())
	payload := "{\n  \"tokens\": {\"access_token\": \"test-export-access\", \"refresh_token\": \"test-export-refresh\"},\n  \"future_field\": [1, true]\n}\n"
	for _, id := range []string{"acct-1", "acct-2", "missing-file", "invalid-file"} {
		account := testAPIAccount(id)
		account.IsActive = id == "acct-1"
		if err := daos.Accounts.Create(ctx, account); err != nil {
			t.Fatal(err)
		}
		if id == "missing-file" {
			continue
		}
		accountDir, err := store.AccountCodexDir("codex", account.StorageID)
		if err != nil {
			t.Fatal(err)
		}
		content := payload
		if id == "acct-1" {
			content = `{"tokens":{"access_token":"other-account-secret"}}`
		}
		if err := store.ImportAuthJSONToCodexDir(ctx, accountDir, []byte(content)); err != nil {
			t.Fatal(err)
		}
		if id == "invalid-file" {
			if err := os.WriteFile(filepath.Join(accountDir, "auth.json"), []byte(`{"secret":"test-export-refresh", broken`), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	activePayload := []byte(`{"tokens":{"access_token":"active-home-secret"}}`)
	if err := store.ImportAuthJSONToCodexDir(ctx, store.ActiveCodexDir(), activePayload); err != nil {
		t.Fatal(err)
	}
	registry := provider.NewRegistry()
	codexProvider, err := codex.New(codex.Config{Credentials: store})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ctx, codexProvider); err != nil {
		t.Fatal(err)
	}
	securityManager, err := security.NewManager(security.Config{BindAddr: "127.0.0.1:43127"})
	if err != nil {
		t.Fatal(err)
	}
	handler := router.NewHandler(router.Config{
		SecurityManager: securityManager,
		ProviderService: service.NewProviderService(registry),
		AccountService:  service.NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, registry, service.NewAccountActivationCoordinator()),
	})
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	path := "/api/providers/codex/accounts/acct-2/auth-json/export"
	response := authenticatedJSONRequest(t, handler, http.MethodPost, path, `{}`)
	var result struct {
		Code string `json:"code"`
		Data struct {
			AuthJSON string `json:"authJson"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result.Code != "SUCCESS" || result.Data.AuthJSON != payload {
		t.Fatal("export must preserve the selected account's complete file, including formatting and unknown fields")
	}
	assertSecurityHeaders(t, response.Header())
	for _, tc := range []struct{ accountID, code string }{
		{"missing-account", "NOT_FOUND"}, {"missing-file", "NOT_FOUND"}, {"invalid-file", "UNAVAILABLE"},
	} {
		t.Run(tc.accountID, func(t *testing.T) {
			response := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/"+tc.accountID+"/auth-json/export", `{}`)
			if !strings.Contains(response.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("expected %s", tc.code)
			}
			assertBodyDoesNotLeakSensitiveData(t, response.Body.String())
			if strings.Contains(response.Body.String(), "test-export-") || strings.Contains(response.Body.String(), dir) {
				t.Fatal("export error leaked sensitive details")
			}
		})
	}
	for _, tc := range []struct {
		name, method, host, origin, contentType, body string
		status                                        int
	}{
		{"cross-origin", "POST", "127.0.0.1:43127", "https://example.com", "application/json", `{}`, 403},
		{"missing-origin", "POST", "127.0.0.1:43127", "", "application/json", `{}`, 403},
		{"wrong-host", "POST", "example.com", "http://127.0.0.1:43127", "application/json", `{}`, 403},
		{"not-json", "POST", "127.0.0.1:43127", "http://127.0.0.1:43127", "text/plain", `{}`, 400},
		{"path-in-body", "POST", "127.0.0.1:43127", "http://127.0.0.1:43127", "application/json", `{"path":"/tmp/auth.json"}`, 400},
		{"get", "GET", "127.0.0.1:43127", "", "", "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, path, strings.NewReader(tc.body))
			request.Host = tc.host
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("Content-Type", tc.contentType)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status || strings.Contains(response.Body.String(), `"code":"SUCCESS"`) || strings.Contains(response.Body.String(), "test-export-") {
				t.Fatalf("request must be rejected, status = %d", response.Code)
			}
		})
	}
	list := authenticatedRequest(t, handler, http.MethodGet, "/api/accounts", "")
	assertBodyDoesNotLeakSensitiveData(t, list.Body.String())
	if strings.Contains(list.Body.String(), "test-export-") {
		t.Fatal("list leaked credentials")
	}
	providers := authenticatedRequest(t, handler, http.MethodGet, "/api/providers", "")
	if !strings.Contains(providers.Body.String(), `"canExportAuthJson":true`) {
		t.Fatal("Codex must advertise credential export")
	}
	activeAfter, err := os.ReadFile(filepath.Join(store.ActiveCodexDir(), "auth.json"))
	if err != nil || !bytes.Equal(activeAfter, activePayload) {
		t.Fatal("export must not change the active credentials")
	}
	for _, secret := range []string{"test-export-", "other-account-secret", "active-home-secret", "access_token", "refresh_token"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("logs leaked credential content")
		}
	}
}

func TestAccountAuthJSONExportUnsupportedProvider(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()
	response := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-1/auth-json/export", `{}`)
	if !strings.Contains(response.Body.String(), `"code":"UNSUPPORTED"`) {
		t.Fatal("provider without credential export must reject the request")
	}
}
