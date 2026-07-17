package router_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/codexruntime"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/loginrunner"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/provider/fake"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/router"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

func TestAccountAPIListAllowsLocalRequest(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	request.Host = "127.0.0.1:43127"
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
}

func TestAccountAPIListRenameActivateDeleteAndRefreshOne(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	listResponse := authenticatedRequest(t, handler, http.MethodGet, "/api/accounts", "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	if !strings.Contains(listResponse.Body.String(), `"accountId":"acct-1"`) || !strings.Contains(listResponse.Body.String(), `"usage"`) {
		t.Fatalf("list body = %s, want seeded account with usage", listResponse.Body.String())
	}
	if !strings.Contains(listResponse.Body.String(), `"rateLimitResetCredits":{"availableCount":3}`) {
		t.Fatalf("list body = %s, want reset credits", listResponse.Body.String())
	}

	resetResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-1/rate-limit/reset", `{"idempotencyKey":"reset-attempt-1"}`)
	if resetResponse.Code != http.StatusOK {
		t.Fatalf("reset status = %d, body = %s", resetResponse.Code, resetResponse.Body.String())
	}
	if !strings.Contains(resetResponse.Body.String(), `"outcome":"reset"`) ||
		!strings.Contains(resetResponse.Body.String(), `"rateLimitResetCredits":{"availableCount":2}`) ||
		!strings.Contains(resetResponse.Body.String(), `"usedPercent":0`) {
		t.Fatalf("reset body = %s, want reset outcome and refreshed usage", resetResponse.Body.String())
	}
	retryResetResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-1/rate-limit/reset", `{"idempotencyKey":"reset-attempt-1"}`)
	if !strings.Contains(retryResetResponse.Body.String(), `"outcome":"alreadyRedeemed"`) ||
		!strings.Contains(retryResetResponse.Body.String(), `"rateLimitResetCredits":{"availableCount":2}`) {
		t.Fatalf("retry reset body = %s, want idempotent outcome", retryResetResponse.Body.String())
	}

	deleteActiveResponse := authenticatedRequest(t, handler, http.MethodDelete, "/api/providers/codex/accounts/acct-1", "")
	if deleteActiveResponse.Code != http.StatusOK {
		t.Fatalf("delete active http status = %d, want 200 envelope", deleteActiveResponse.Code)
	}
	if !strings.Contains(deleteActiveResponse.Body.String(), `"code":"CONFLICT"`) {
		t.Fatalf("delete active body = %s, want conflict", deleteActiveResponse.Body.String())
	}

	activateResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-2/activate", `{}`)
	if activateResponse.Code != http.StatusOK {
		t.Fatalf("activate status = %d, body = %s", activateResponse.Code, activateResponse.Body.String())
	}
	if !strings.Contains(activateResponse.Body.String(), `"accountId":"acct-2"`) || !strings.Contains(activateResponse.Body.String(), `"isActive":true`) {
		t.Fatalf("activate body = %s, want acct-2 active", activateResponse.Body.String())
	}

	deleteInactiveResponse := authenticatedRequest(t, handler, http.MethodDelete, "/api/providers/codex/accounts/acct-1", "")
	if deleteInactiveResponse.Code != http.StatusOK {
		t.Fatalf("delete inactive status = %d, body = %s", deleteInactiveResponse.Code, deleteInactiveResponse.Body.String())
	}
	if !strings.Contains(deleteInactiveResponse.Body.String(), `"deleted":true`) {
		t.Fatalf("delete inactive body = %s, want deleted", deleteInactiveResponse.Body.String())
	}

	refreshResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-2/refresh", `{}`)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %s", refreshResponse.Code, refreshResponse.Body.String())
	}
	if !strings.Contains(refreshResponse.Body.String(), `"account":{"providerId":"codex","accountId":"acct-2"`) ||
		!strings.Contains(refreshResponse.Body.String(), `"status":"ready"`) ||
		!strings.Contains(refreshResponse.Body.String(), `"planType":"plus"`) ||
		!strings.Contains(refreshResponse.Body.String(), `"planExpiresAt":null`) {
		t.Fatalf("refresh body = %s, want acct-2 refreshed account state", refreshResponse.Body.String())
	}

	updatePlanExpirationResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-2/plan-expiration/update", `{"planExpiresAt":1767225600}`)
	if updatePlanExpirationResponse.Code != http.StatusOK {
		t.Fatalf("update plan expiration status = %d, body = %s", updatePlanExpirationResponse.Code, updatePlanExpirationResponse.Body.String())
	}
	if !strings.Contains(updatePlanExpirationResponse.Body.String(), `"planExpiresAt":1767225600000`) {
		t.Fatalf("update plan expiration body = %s, want normalized plan expiration", updatePlanExpirationResponse.Body.String())
	}

	refreshAgainResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-2/refresh", `{}`)
	if refreshAgainResponse.Code != http.StatusOK {
		t.Fatalf("refresh again status = %d, body = %s", refreshAgainResponse.Code, refreshAgainResponse.Body.String())
	}
	if !strings.Contains(refreshAgainResponse.Body.String(), `"planExpiresAt":1767225600000`) {
		t.Fatalf("refresh again body = %s, want manual plan expiration preserved", refreshAgainResponse.Body.String())
	}

	updatedListResponse := authenticatedRequest(t, handler, http.MethodGet, "/api/accounts", "")
	if !strings.Contains(updatedListResponse.Body.String(), `"accountId":"acct-2"`) ||
		!strings.Contains(updatedListResponse.Body.String(), `"planType":"plus"`) ||
		!strings.Contains(updatedListResponse.Body.String(), `"planExpiresAt":1767225600000`) {
		t.Fatalf("updated list body = %s, want acct-2 plan metadata", updatedListResponse.Body.String())
	}

	clearPlanExpirationResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-2/plan-expiration/update", `{"planExpiresAt":null}`)
	if clearPlanExpirationResponse.Code != http.StatusOK {
		t.Fatalf("clear plan expiration status = %d, body = %s", clearPlanExpirationResponse.Code, clearPlanExpirationResponse.Body.String())
	}
	if !strings.Contains(clearPlanExpirationResponse.Body.String(), `"planExpiresAt":null`) {
		t.Fatalf("clear plan expiration body = %s, want null plan expiration", clearPlanExpirationResponse.Body.String())
	}
}

func TestAccountAPIRefreshReturnsStableReauthenticationCode(t *testing.T) {
	refreshErr := entity.WrapAppErrorWithUpstreamError(
		entity.ErrorCodeReauthenticationRequired,
		"token_revoked",
		"Encountered invalidated oauth token for user, failing request",
		nil,
	)
	handler, cleanup := newAccountAPIHandlerWithRefreshError(t, refreshErr)
	defer cleanup()

	response := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-2/refresh", `{}`)
	if response.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"errorCode":"REAUTHENTICATION_REQUIRED"`) || strings.Contains(body, `"errorCode":"token_revoked"`) {
		t.Fatalf("refresh body = %s, want stable reauthentication code", body)
	}

	listResponse := authenticatedRequest(t, handler, http.MethodGet, "/api/accounts", "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	listBody := listResponse.Body.String()
	if !strings.Contains(listBody, `"status":"auth_expired"`) || !strings.Contains(listBody, `"errorCode":"REAUTHENTICATION_REQUIRED"`) {
		t.Fatalf("list body = %s, want persisted auth_expired state", listBody)
	}
}

func TestAccountAPICreateManualAccountAndRefreshOne(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	createResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/create", `{"email":"new@example.com"}`)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	assertBodyDoesNotLeakSensitiveData(t, createResponse.Body.String())
	accountID := entity.AccountIDFromEmail("new@example.com")
	if !strings.Contains(createResponse.Body.String(), `"accountId":"`+accountID+`"`) || !strings.Contains(createResponse.Body.String(), `"email":"new@example.com"`) {
		t.Fatalf("create body = %s, want manual account", createResponse.Body.String())
	}

	refreshResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/"+accountID+"/refresh", `{}`)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("refresh one status = %d, body = %s", refreshResponse.Code, refreshResponse.Body.String())
	}
	if !strings.Contains(refreshResponse.Body.String(), `"accountId":"`+accountID+`"`) || !strings.Contains(refreshResponse.Body.String(), `"status":"ready"`) {
		t.Fatalf("refresh one body = %s, want refreshed manual account", refreshResponse.Body.String())
	}
}

func TestAccountAPIImportAccountAuthJSONAndRefreshCreatesAccount(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	authJSON := `{"email":"imported@example.com","planType":"team","tokens":{"access_token":"secret","refresh_token":"refresh"}}`
	bodyBytes, err := json.Marshal(map[string]string{"authJson": authJSON})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	importResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/auth-json/import", string(bodyBytes))
	if importResponse.Code != http.StatusOK {
		t.Fatalf("import auth json status = %d, body = %s", importResponse.Code, importResponse.Body.String())
	}
	body := importResponse.Body.String()
	assertBodyDoesNotLeakSensitiveData(t, body)
	accountID := entity.AccountIDFromEmail("imported@example.com")
	if !strings.Contains(body, `"accountId":"`+accountID+`"`) ||
		!strings.Contains(body, `"email":"imported@example.com"`) ||
		!strings.Contains(body, `"planType":"team"`) ||
		!strings.Contains(body, `"status":"ready"`) {
		t.Fatalf("import auth json body = %s, want imported account with usage", body)
	}

	listResponse := authenticatedRequest(t, handler, http.MethodGet, "/api/accounts", "")
	if !strings.Contains(listResponse.Body.String(), `"email":"imported@example.com"`) {
		t.Fatalf("list body = %s, want imported account", listResponse.Body.String())
	}
}

func TestAccountAPIImportAccountAuthJSONAndRefreshFailureDoesNotCreateAccount(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	authJSON := `{"email":"failed@example.com","refreshErrorCode":"UNAVAILABLE","tokens":{"access_token":"secret"}}`
	bodyBytes, err := json.Marshal(map[string]string{"authJson": authJSON})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	importResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/auth-json/import", string(bodyBytes))
	if importResponse.Code != http.StatusOK {
		t.Fatalf("import auth json status = %d, body = %s", importResponse.Code, importResponse.Body.String())
	}
	body := importResponse.Body.String()
	assertBodyDoesNotLeakSensitiveData(t, body)
	if !strings.Contains(body, `"code":"UNAVAILABLE"`) {
		t.Fatalf("import auth json body = %s, want unavailable", body)
	}

	listResponse := authenticatedRequest(t, handler, http.MethodGet, "/api/accounts", "")
	if strings.Contains(listResponse.Body.String(), `"email":"failed@example.com"`) {
		t.Fatalf("list body = %s, want failed import not persisted", listResponse.Body.String())
	}
}

func TestAccountAPIImportAccountAuthJSONRejectsInvalidJSON(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	bodyBytes, err := json.Marshal(map[string]string{"authJson": `not-json`})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	response := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/auth-json/import", string(bodyBytes))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"code":"VALIDATION_FAILED"`) {
		t.Fatalf("body = %s, want validation failed", response.Body.String())
	}
}

func TestAccountAPIImportCurrentAccount(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	importResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/import-current", `{}`)
	if importResponse.Code != http.StatusOK {
		t.Fatalf("import current status = %d, body = %s", importResponse.Code, importResponse.Body.String())
	}
	body := importResponse.Body.String()
	assertBodyDoesNotLeakSensitiveData(t, body)
	if !strings.Contains(body, `"accountId":"acct-1"`) || !strings.Contains(body, `"isActive":true`) {
		t.Fatalf("import current body = %s, want acct-1 active", body)
	}
}

func TestAccountAPILoginTaskCreateAndImport(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	createResponse := authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/login-tasks/create", `{"expectedEmail":"login@example.com","mode":"browser"}`)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create login task status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	body := createResponse.Body.String()
	assertBodyDoesNotLeakSensitiveData(t, body)
	taskID := extractStringField(t, body, "taskId")

	getResponse := waitForLoginTaskImported(t, handler, taskID)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get login task status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	getBody := getResponse.Body.String()
	assertBodyDoesNotLeakSensitiveData(t, getBody)
	if !strings.Contains(getBody, `"status":"imported"`) || !strings.Contains(getBody, `"email":"login@example.com"`) {
		t.Fatalf("get login task body = %s, want imported login@example.com", getBody)
	}

	listResponse := authenticatedRequest(t, handler, http.MethodGet, "/api/accounts", "")
	if !strings.Contains(listResponse.Body.String(), `"email":"login@example.com"`) || strings.Contains(listResponse.Body.String(), `"email":"login@example.com","planType":"plus","isActive":true`) {
		t.Fatalf("list body = %s, want imported account but not active", listResponse.Body.String())
	}
}

func TestAccountAPIRemovedRoutesReturnNotFound(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	for _, response := range []*httptest.ResponseRecorder{
		authenticatedRequest(t, handler, http.MethodGet, "/api/login-tasks/fake-login-1", ""),
		authenticatedRequest(t, handler, http.MethodDelete, "/api/login-tasks/fake-login-1", ""),
		authenticatedJSONRequest(t, handler, http.MethodPost, "/api/usage/refresh", `{}`),
		authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-1/rename", `{"label":"Primary"}`),
		authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-1/auth-json/import", `{"authJson":"{}"}`),
		authenticatedJSONRequest(t, handler, http.MethodPost, "/api/providers/codex/accounts/acct-2/usage/refresh", `{}`),
	} {
		body := response.Body.String()
		if response.Code != http.StatusOK || !(strings.Contains(body, `"code":"NOT_FOUND"`) || strings.Contains(body, `"code":"METHOD_NOT_ALLOWED"`)) {
			t.Fatalf("removed route response status = %d, body = %s; want unavailable route envelope", response.Code, body)
		}
	}
}

func TestAccountAPIMutationRejectsMissingOrigin(t *testing.T) {
	handler, cleanup := newAccountAPIHandler(t)
	defer cleanup()

	request := httptest.NewRequest(http.MethodPost, "/api/providers/codex/accounts/create", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func newAccountAPIHandler(t *testing.T) (http.Handler, func()) {
	return newAccountAPIHandlerWithRefreshError(t, nil)
}

func newAccountAPIHandlerWithRefreshError(t *testing.T, refreshErr error) (http.Handler, func()) {
	t.Helper()

	securityManager, err := security.NewManager(security.Config{
		BindAddr: "127.0.0.1:43127",
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
	fakeAcct2 := testAPIAccount("acct-2")
	planType := "plus"
	planExpiresAt := int64(1767225600000)
	fakeAcct2.PlanType = &planType
	fakeAcct2.PlanExpiresAt = &planExpiresAt
	fakeProvider := fake.New(fake.Config{
		ID:          "codex",
		DisplayName: "Codex Fake",
		Accounts: []fake.AccountState{
			{Account: testAPIAccount("acct-1"), Usage: testAPIUsage("acct-1", entity.UsageStatusReady)},
			{Account: fakeAcct2, Usage: testAPIUsage("acct-2", entity.UsageStatusReady)},
		},
	})
	var registeredProvider provider.Provider = fakeProvider
	if refreshErr != nil {
		registeredProvider = &testFailingMetadataRefreshProvider{Provider: fakeProvider, err: refreshErr}
	}
	if err := providerRegistry.Register(context.Background(), registeredProvider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	activeAccount := testAPIAccount("acct-1")
	activeAccount.IsActive = true
	if err := daos.Accounts.Create(context.Background(), activeAccount); err != nil {
		t.Fatalf("seed acct-1 error = %v", err)
	}
	if err := daos.UsageSnapshots.Upsert(context.Background(), testAPIUsage("acct-1", entity.UsageStatusReady)); err != nil {
		t.Fatalf("seed acct-1 usage error = %v", err)
	}
	if err := daos.Accounts.Create(context.Background(), testAPIAccount("acct-2")); err != nil {
		t.Fatalf("seed acct-2 error = %v", err)
	}

	accountService := service.NewAccountService(dao.NewUnitOfWork(appDB.GORM()), daos, providerRegistry)
	providerService := service.NewProviderService(providerRegistry)
	testLoginImported = make(chan struct{}, 1)
	loginTaskService, err := service.NewLoginTaskService(service.LoginTaskConfig{
		UnitOfWork: dao.NewUnitOfWork(appDB.GORM()),
		DAOs:       daos,
		Resolver: codexruntime.NewResolver(codexruntime.Config{
			ConfiguredBin: "/tmp/fake-codex",
			Validator:     testRuntimeValidator{},
		}),
		Runner:   testLoginRunner{},
		Importer: testLoginImporter{},
		RootDir:  filepath.Join(t.TempDir(), "login-tasks"),
		TaskTTL:  time.Minute,
	})
	if err != nil {
		t.Fatalf("NewLoginTaskService() error = %v", err)
	}

	handler := router.NewHandler(router.Config{
		SecurityManager:  securityManager,
		ProviderService:  providerService,
		AccountService:   accountService,
		LoginTaskService: loginTaskService,
	})
	return handler, func() {
		if err := appDB.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
}

var testLoginImported chan struct{}

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
	snapshotJSON := `{"rateLimitResetCredits":{"availableCount":3},"rateLimits":{"primary":{"usedPercent":50,"resetsAt":1700000000000},"secondary":null}}`
	return entity.UsageSnapshot{
		ProviderID:   "codex",
		AccountID:    accountID,
		Status:       status,
		UsedPercent:  &usedPercent,
		SnapshotJSON: &snapshotJSON,
		RefreshedAt:  2000,
	}
}

func authenticatedJSONRequest(t *testing.T, handler http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		request.Header.Set("Origin", "http://127.0.0.1:43127")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func authenticatedRequest(t *testing.T, handler http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = "127.0.0.1:43127"
	if method != http.MethodGet {
		request.Header.Set("Origin", "http://127.0.0.1:43127")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertBodyDoesNotLeakSensitiveData(t *testing.T, body string) {
	t.Helper()

	for _, forbidden := range []string{"access_token", "refresh_token", "auth.json"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
}

type testRuntimeValidator struct{}

func (testRuntimeValidator) Validate(context.Context, string) error {
	return nil
}

type testFailingMetadataRefreshProvider struct {
	provider.Provider
	err error
}

func (failing *testFailingMetadataRefreshProvider) RefreshAccountWithMetadata(context.Context, entity.Account) (*entity.Account, *entity.UsageSnapshot, error) {
	return nil, nil, failing.err
}

type testLoginRunner struct{}

func (testLoginRunner) Run(_ context.Context, input loginrunner.Input) (loginrunner.Result, error) {
	if input.OnProgress != nil {
		loginURL := "https://chatgpt.com/codex/login"
		input.OnProgress(loginrunner.Progress{LoginURL: &loginURL})
	}
	return loginrunner.Result{CodexHome: input.CodexHome}, nil
}

type testLoginImporter struct{}

func (testLoginImporter) ReadAccountFromCodexDir(context.Context, string) (*entity.Account, error) {
	email := "login@example.com"
	planType := "plus"
	accountID := entity.AccountIDFromEmail(email)
	return &entity.Account{
		ProviderID: "codex",
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount("codex", accountID),
		Label:      email,
		Email:      &email,
		PlanType:   &planType,
	}, nil
}

func (testLoginImporter) ImportAccountAuthFromCodexDir(context.Context, entity.Account, string) error {
	select {
	case testLoginImported <- struct{}{}:
	default:
	}
	return nil
}

func waitForLoginTaskImported(t *testing.T, handler http.Handler, taskID string) *httptest.ResponseRecorder {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response := authenticatedRequest(t, handler, http.MethodGet, "/api/providers/codex/login-tasks/"+taskID, "")
		if strings.Contains(response.Body.String(), `"status":"imported"`) {
			return response
		}
		time.Sleep(10 * time.Millisecond)
	}
	return authenticatedRequest(t, handler, http.MethodGet, "/api/providers/codex/login-tasks/"+taskID, "")
}

func extractStringField(t *testing.T, body string, field string) string {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	value, ok := envelope.Data[field].(string)
	if !ok || value == "" {
		t.Fatalf("field %s missing in body %s", field, body)
	}
	return value
}
