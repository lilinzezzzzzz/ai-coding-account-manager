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
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/provider"
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
			RateLimitResetCredits: &rateLimitResetCreditsSummary{AvailableCount: 2},
			RateLimits: rateLimitSnapshot{
				Primary: &rateLimitWindow{
					UsedPercent:        floatPtr(42.5),
					ResetsAt:           int64Ptr(1700000000000),
					WindowDurationMins: int64Ptr(7 * 24 * 60),
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
	var snapshotPayload rateLimitsReadResponse
	if err := json.Unmarshal([]byte(*snapshot.SnapshotJSON), &snapshotPayload); err != nil {
		t.Fatalf("unmarshal snapshot json: %v", err)
	}
	if snapshotPayload.RateLimitResetCredits == nil || snapshotPayload.RateLimitResetCredits.AvailableCount != 2 {
		t.Fatalf("reset credits = %#v, want available count 2", snapshotPayload.RateLimitResetCredits)
	}
	if snapshotPayload.RateLimits.Primary == nil || snapshotPayload.RateLimits.Primary.WindowDurationMins == nil || *snapshotPayload.RateLimits.Primary.WindowDurationMins != 7*24*60 {
		t.Fatalf("primary window duration = %#v, want 10080 minutes", snapshotPayload.RateLimits.Primary)
	}
	if snapshotPayload.RateLimits.Secondary != nil {
		t.Fatalf("secondary limit = %#v, want nil", snapshotPayload.RateLimits.Secondary)
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

func TestProviderResetsAccountRateLimitAndReturnsLatestUsage(t *testing.T) {
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

	fakeClient := &fakeCodexClient{
		responses: map[string]any{
			"account/read": accountReadResponse{
				Account: &codexAccount{Type: "chatgpt", Email: "user@example.com"},
			},
			"account/rateLimitResetCredit/consume": consumeRateLimitResetCreditResponse{
				Outcome: provider.RateLimitResetOutcomeReset,
			},
		},
		responseSequences: map[string][]any{
			"account/rateLimits/read": {
				rateLimitsReadResponse{
					RateLimitResetCredits: &rateLimitResetCreditsSummary{
						AvailableCount: 3,
						Credits: []rateLimitResetCredit{
							{ID: "credit-latest", Status: "available", ExpiresAt: 3000},
							{ID: "credit-expired", Status: "expired", ExpiresAt: 500},
							{ID: "credit-earliest", Status: "available", ExpiresAt: 1000},
						},
					},
					RateLimits: rateLimitSnapshot{
						Primary: &rateLimitWindow{UsedPercent: floatPtr(100)},
					},
				},
				rateLimitsReadResponse{
					RateLimitResetCredits: &rateLimitResetCreditsSummary{AvailableCount: 2},
					RateLimits: rateLimitSnapshot{
						Primary: &rateLimitWindow{UsedPercent: floatPtr(0)},
					},
				},
			},
		},
	}
	codexProvider := newTestProvider(t, store, func(context.Context, appserver.Config) (appServerClient, error) {
		return fakeClient, nil
	})

	result, err := codexProvider.ResetAccountRateLimit(context.Background(), account, "reset-attempt-1")
	if err != nil {
		t.Fatalf("reset account rate limit: %v", err)
	}
	if result.Outcome != provider.RateLimitResetOutcomeReset {
		t.Fatalf("outcome = %q, want reset", result.Outcome)
	}
	if result.Usage == nil || result.Usage.SnapshotJSON == nil {
		t.Fatal("reset result usage is empty")
	}
	var snapshot rateLimitsReadResponse
	if err := json.Unmarshal([]byte(*result.Usage.SnapshotJSON), &snapshot); err != nil {
		t.Fatalf("unmarshal reset snapshot: %v", err)
	}
	if snapshot.RateLimitResetCredits == nil || snapshot.RateLimitResetCredits.AvailableCount != 2 {
		t.Fatalf("reset credits = %#v, want available count 2", snapshot.RateLimitResetCredits)
	}

	foundConsume := false
	for _, call := range fakeClient.calls {
		if call.method != "account/rateLimitResetCredit/consume" {
			continue
		}
		foundConsume = true
		params, ok := call.params.(consumeRateLimitResetCreditParams)
		if !ok || params.IdempotencyKey != "reset-attempt-1" || params.CreditID != "credit-earliest" {
			t.Fatalf("consume params = %#v, want idempotency key and earliest expiring credit", call.params)
		}
	}
	if !foundConsume {
		t.Fatal("consume method was not called")
	}
}

func TestEarliestExpiringAvailableCreditID(t *testing.T) {
	tests := []struct {
		name    string
		summary *rateLimitResetCreditsSummary
		want    string
	}{
		{name: "missing details", summary: &rateLimitResetCreditsSummary{AvailableCount: 3}},
		{
			name: "earliest available",
			summary: &rateLimitResetCreditsSummary{Credits: []rateLimitResetCredit{
				{ID: "later", Status: "available", ExpiresAt: 3000},
				{ID: "expired", Status: "expired", ExpiresAt: 500},
				{ID: "earliest", Status: "AVAILABLE", ExpiresAt: 1000},
			}},
			want: "earliest",
		},
		{
			name: "stable tie break",
			summary: &rateLimitResetCreditsSummary{Credits: []rateLimitResetCredit{
				{ID: "credit-b", Status: "available", ExpiresAt: 1000},
				{ID: "credit-a", Status: "available", ExpiresAt: 1000},
			}},
			want: "credit-a",
		},
		{
			name: "invalid details",
			summary: &rateLimitResetCreditsSummary{Credits: []rateLimitResetCredit{
				{ID: "", Status: "available", ExpiresAt: 1000},
				{ID: "missing-expiry", Status: "available"},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := earliestExpiringAvailableCreditID(test.summary); got != test.want {
				t.Fatalf("earliestExpiringAvailableCreditID() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestConsumeRateLimitResetCreditParamsOmitsUnavailableCreditID(t *testing.T) {
	payload, err := json.Marshal(consumeRateLimitResetCreditParams{IdempotencyKey: "reset-attempt-1"})
	if err != nil {
		t.Fatalf("marshal consume params: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal consume params: %v", err)
	}
	if _, ok := decoded["creditId"]; ok {
		t.Fatalf("consume params = %s, want creditId omitted", payload)
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

func TestProviderImportsAuthJSONAndRefreshesAccount(t *testing.T) {
	tempDir := t.TempDir()
	store := newTestStore(t, tempDir, filepath.Join(tempDir, "active"))
	authJSON := []byte(`{"tokens":{"access_token":"raw-secret"}}`)
	fakeClient := &fakeCodexClient{responses: map[string]any{
		"account/read": accountReadResponse{
			Account: &codexAccount{Type: "chatgpt", Email: "raw@example.com"},
		},
		"account/rateLimits/read": rateLimitsReadResponse{
			RateLimits: rateLimitSnapshot{
				Primary: &rateLimitWindow{
					UsedPercent: floatPtr(12.5),
					ResetsAt:    int64Ptr(1700000000000),
				},
				PlanType: stringPtr("team"),
			},
		},
	}}
	codexProvider := newTestProvider(t, store, func(_ context.Context, cfg appserver.Config) (appServerClient, error) {
		content, err := os.ReadFile(filepath.Join(cfg.CodexHome, "auth.json"))
		if err != nil {
			t.Fatalf("read runtime auth: %v", err)
		}
		if string(content) != string(authJSON) {
			t.Fatalf("runtime auth content = %s, want %s", content, authJSON)
		}
		return fakeClient, nil
	})

	account, snapshot, err := codexProvider.ImportAccountAuthJSONAndRefresh(context.Background(), authJSON)
	if err != nil {
		t.Fatalf("import auth json and refresh: %v", err)
	}
	fakeClient.assertAccountReadRefreshToken(t, false)
	if account.AccountID != entity.AccountIDFromEmail("raw@example.com") {
		t.Fatalf("account id = %s, want email-derived id", account.AccountID)
	}
	if account.PlanType == nil || *account.PlanType != "team" {
		t.Fatalf("plan type = %v, want team", account.PlanType)
	}
	if snapshot.UsedPercent == nil || *snapshot.UsedPercent != 12.5 {
		t.Fatalf("used percent = %v, want 12.5", snapshot.UsedPercent)
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

func TestMapAccountRequiresReauthenticationWhenAccountIsMissing(t *testing.T) {
	_, err := mapAccount(accountReadResponse{RequiresOpenaiAuth: true})
	if err == nil {
		t.Fatal("mapAccount() error = nil, want reauthentication required")
	}
	appErr, ok := entity.AsAppError(err)
	if !ok || appErr.ErrorCode() != entity.ErrorCodeReauthenticationRequired {
		t.Fatalf("mapAccount() error = %v, want %s", err, entity.ErrorCodeReauthenticationRequired)
	}
}

func TestReauthenticationRequiredUpstreamCodes(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{code: "token_expired", want: true},
		{code: "token_invalidated", want: true},
		{code: "token_revoked", want: true},
		{code: "refresh_token_expired", want: true},
		{code: "refresh_token_reused", want: true},
		{code: "refresh_token_invalidated", want: true},
		{code: " TOKEN_REVOKED ", want: true},
		{code: "invalid_grant", want: false},
		{code: "rate_limit_exceeded", want: false},
		{code: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			if got := isReauthenticationRequiredUpstreamCode(test.code); got != test.want {
				t.Fatalf("isReauthenticationRequiredUpstreamCode(%q) = %t, want %t", test.code, got, test.want)
			}
		})
	}
}

type fakeCodexClient struct {
	responses         map[string]any
	responseSequences map[string][]any
	calls             []fakeCodexCall
	closed            bool
}

type fakeCodexCall struct {
	method string
	params any
}

func (client *fakeCodexClient) Call(_ context.Context, method string, params any, result any) error {
	client.calls = append(client.calls, fakeCodexCall{method: method, params: params})
	if responses := client.responseSequences[method]; len(responses) > 0 {
		response := responses[0]
		client.responseSequences[method] = responses[1:]
		return copyCodexResponse(response, result)
	}
	response, ok := client.responses[method]
	if !ok {
		return nil
	}
	return copyCodexResponse(response, result)
}

func copyCodexResponse(response any, result any) error {
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
