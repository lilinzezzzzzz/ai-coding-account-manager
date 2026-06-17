package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/middleware"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
)

func TestMutationBoundaryAllowsSameOriginJSONRequest(t *testing.T) {
	manager := newSecurityManager(t)
	handler := mutationBoundary(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/codex/accounts/create", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Origin", "http://127.0.0.1:43127")
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusNoContent, response.Body.String())
	}
}

func TestMutationBoundaryRejectsInvalidOrigin(t *testing.T) {
	manager := newSecurityManager(t)
	handler := mutationBoundary(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not run")
	}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/codex/accounts/create", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Origin", "http://evil.test:43127")
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestMutationBoundaryRejectsAllowedButMismatchedOrigin(t *testing.T) {
	manager := newSecurityManager(t)
	handler := mutationBoundary(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not run")
	}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/codex/accounts/create", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Origin", "http://localhost:43127")
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestMutationBoundaryRejectsMissingOrigin(t *testing.T) {
	manager := newSecurityManager(t)
	handler := mutationBoundary(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not run")
	}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/codex/accounts/create", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func mutationBoundary(manager *security.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return middleware.RequireHost(manager)(
			middleware.RequireOrigin(manager)(
				middleware.RequireJSONContentType(
					middleware.LimitBodySize(next),
				),
			),
		)
	}
}

func newSecurityManager(t *testing.T) *security.Manager {
	t.Helper()

	manager, err := security.NewManager(security.Config{
		BindAddr: "127.0.0.1:43127",
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}
