package router_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/fake"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/router"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

func TestAccountAPIRequiresSession(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	request.Host = "127.0.0.1:43127"
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestAccountAPIImportListRenameActivateDeleteAndRefresh(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()
	cookie, csrf := bootstrapSession(t, handler)

	importResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/import-current", `{}`, cookie, csrf)
	if importResponse.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", importResponse.Code, importResponse.Body.String())
	}
	assertBodyDoesNotLeakSensitiveData(t, importResponse.Body.String())

	listResponse := authenticatedRequest(t, handler, http.MethodGet, "/api/accounts", "", cookie, csrf)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	if !strings.Contains(listResponse.Body.String(), `"accountId":"acct-1"`) || !strings.Contains(listResponse.Body.String(), `"usage"`) {
		t.Fatalf("list body = %s, want imported account with usage", listResponse.Body.String())
	}

	renameResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-1/rename", `{"label":"Primary"}`, cookie, csrf)
	if renameResponse.Code != http.StatusOK {
		t.Fatalf("rename status = %d, body = %s", renameResponse.Code, renameResponse.Body.String())
	}
	if !strings.Contains(renameResponse.Body.String(), `"label":"Primary"`) {
		t.Fatalf("rename body = %s, want updated label", renameResponse.Body.String())
	}

	deleteActiveResponse := authenticatedRequest(t, handler, http.MethodDelete, "/api/providers/codex/accounts/acct-1", "", cookie, csrf)
	if deleteActiveResponse.Code != http.StatusOK {
		t.Fatalf("delete active http status = %d, want 200 envelope", deleteActiveResponse.Code)
	}
	if !strings.Contains(deleteActiveResponse.Body.String(), `"code":"CONFLICT"`) {
		t.Fatalf("delete active body = %s, want conflict", deleteActiveResponse.Body.String())
	}

	activateResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-2/activate", `{}`, cookie, csrf)
	if activateResponse.Code != http.StatusOK {
		t.Fatalf("activate status = %d, body = %s", activateResponse.Code, activateResponse.Body.String())
	}
	if !strings.Contains(activateResponse.Body.String(), `"accountId":"acct-2"`) || !strings.Contains(activateResponse.Body.String(), `"isActive":true`) {
		t.Fatalf("activate body = %s, want acct-2 active", activateResponse.Body.String())
	}

	deleteInactiveResponse := authenticatedRequest(t, handler, http.MethodDelete, "/api/providers/codex/accounts/acct-1", "", cookie, csrf)
	if deleteInactiveResponse.Code != http.StatusOK {
		t.Fatalf("delete inactive status = %d, body = %s", deleteInactiveResponse.Code, deleteInactiveResponse.Body.String())
	}
	if !strings.Contains(deleteInactiveResponse.Body.String(), `"deleted":true`) {
		t.Fatalf("delete inactive body = %s, want deleted", deleteInactiveResponse.Body.String())
	}

	refreshResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/usage/refresh", `{}`, cookie, csrf)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %s", refreshResponse.Code, refreshResponse.Body.String())
	}
	if !strings.Contains(refreshResponse.Body.String(), `"accountId":"acct-2"`) {
		t.Fatalf("refresh body = %s, want acct-2 result", refreshResponse.Body.String())
	}
}

func TestAccountAPILoginTaskFlow(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()
	cookie, csrf := bootstrapSession(t, handler)

	startResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/login-tasks/create", `{}`, cookie, csrf)
	if startResponse.Code != http.StatusOK {
		t.Fatalf("start login status = %d, body = %s", startResponse.Code, startResponse.Body.String())
	}
	taskID := readStringField(t, startResponse.Body.Bytes(), "id")
	if taskID == "" {
		t.Fatalf("task id is empty, body = %s", startResponse.Body.String())
	}

	pollResponse := authenticatedRequest(t, handler, http.MethodGet, "/api/login-tasks/"+taskID, "", cookie, csrf)
	if pollResponse.Code != http.StatusOK {
		t.Fatalf("poll status = %d, body = %s", pollResponse.Code, pollResponse.Body.String())
	}
	if !strings.Contains(pollResponse.Body.String(), `"state":"pending"`) {
		t.Fatalf("poll body = %s, want pending", pollResponse.Body.String())
	}

	cancelResponse := authenticatedRequest(t, handler, http.MethodDelete, "/api/login-tasks/"+taskID, "", cookie, csrf)
	if cancelResponse.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, body = %s", cancelResponse.Code, cancelResponse.Body.String())
	}
	if !strings.Contains(cancelResponse.Body.String(), `"canceled":true`) {
		t.Fatalf("cancel body = %s, want canceled", cancelResponse.Body.String())
	}
}

func TestAccountAPIMutationRejectsInvalidCSRF(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()
	cookie, _ := bootstrapSession(t, handler)

	response := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/import-current", `{}`, cookie, "bad-csrf")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func newAccountAPIHandler(t *testing.T) (http.Handler, func()) {
	t.Helper()

	securityManager, err := security.NewManager(security.Config{
		BindAddr:       "127.0.0.1:43127",
		BootstrapToken: "test-bootstrap-token",
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	appDB, err := database.Open(context.Background(), database.Config{
		Path: filepath.Join(t.TempDir(), "state.db"),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	daos := dao.NewDAOs(appDB.GORM())
	providerRegistry := provider.NewRegistry()
	fakeProvider := fake.New(fake.Config{
		ID:          "codex",
		DisplayName: "Codex Fake",
		Accounts: []fake.AccountState{
			{Account: testAPIAccount("acct-1"), IsCurrent: true, Usage: testAPIUsage("acct-1", entity.UsageStatusReady)},
			{Account: testAPIAccount("acct-2"), Usage: testAPIUsage("acct-2", entity.UsageStatusReady)},
		},
	})
	if err := providerRegistry.Register(context.Background(), fakeProvider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := daos.Accounts.Create(context.Background(), testAPIAccount("acct-2")); err != nil {
		t.Fatalf("seed acct-2 error = %v", err)
	}

	accountService := service.NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, providerRegistry)
	providerService := service.NewProviderService(providerRegistry)

	handler := router.NewHandler(router.Config{
		SecurityManager: securityManager,
		ProviderService: providerService,
		AccountService:  accountService,
	})
	return handler, func() {
		if err := appDB.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
}

func testAPIAccount(accountID string) entity.Account {
	return entity.Account{
		ProviderID: "codex",
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount("codex", accountID),
		Label:      accountID,
		CreatedAt:  1000,
		UpdatedAt:  1000,
	}
}

func testAPIUsage(accountID string, status entity.UsageStatus) entity.UsageSnapshot {
	usedPercent := 50.0
	return entity.UsageSnapshot{
		ProviderID:  "codex",
		AccountID:   accountID,
		Status:      status,
		UsedPercent: &usedPercent,
		RefreshedAt: 2000,
	}
}

func bootstrapSession(t *testing.T, handler http.Handler) (*http.Cookie, string) {
	t.Helper()

	response := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/session/bootstrap", `{"bootstrapToken":"test-bootstrap-token"}`, nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d, body = %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}
	csrf := readStringField(t, response.Body.Bytes(), "csrfToken")
	if csrf == "" {
		t.Fatalf("csrfToken is empty, body = %s", response.Body.String())
	}
	return cookies[0], csrf
}

func authenticatedJSONRequest(t *testing.T, handler http.Handler, method string, path string, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
		request.Header.Set("Origin", "http://127.0.0.1:43127")
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func authenticatedRequest(t *testing.T, handler http.Handler, method string, path string, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = "127.0.0.1:43127"
	if cookie != nil {
		request.AddCookie(cookie)
		if method != http.MethodGet {
			request.Header.Set("Origin", "http://127.0.0.1:43127")
			request.Header.Set("X-CSRF-Token", csrf)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func readStringField(t *testing.T, body []byte, field string) string {
	t.Helper()

	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	value, _ := envelope.Data[field].(string)
	return value
}

func assertBodyDoesNotLeakSensitiveData(t *testing.T, body string) {
	t.Helper()

	for _, forbidden := range []string{"access_token", "refresh_token", "auth.json", "bootstrapToken"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
}
