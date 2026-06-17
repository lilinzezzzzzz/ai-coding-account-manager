package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/middleware"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/security"
)

func TestMutationBoundaryAllowsSameOriginCSRFRequest(t *testing.T) {
	manager, session := newSecurityManagerWithSession(t)
	handler := mutationBoundary(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/accounts/import-current", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Origin", "http://127.0.0.1:43127")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", session.CSRFToken)
	request.AddCookie(security.CookieForSession(session))
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusNoContent, response.Body.String())
	}
}

func TestMutationBoundaryRejectsInvalidOrigin(t *testing.T) {
	manager, session := newSecurityManagerWithSession(t)
	handler := mutationBoundary(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not run")
	}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/accounts/import-current", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Origin", "http://evil.test:43127")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", session.CSRFToken)
	request.AddCookie(security.CookieForSession(session))
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestMutationBoundaryRejectsAllowedButMismatchedOrigin(t *testing.T) {
	manager, session := newSecurityManagerWithSession(t)
	handler := mutationBoundary(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not run")
	}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/accounts/import-current", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Origin", "http://localhost:43127")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", session.CSRFToken)
	request.AddCookie(security.CookieForSession(session))
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestMutationBoundaryRejectsInvalidCSRFToken(t *testing.T) {
	manager, session := newSecurityManagerWithSession(t)
	handler := mutationBoundary(manager)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("handler should not run")
	}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/accounts/import-current", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Origin", "http://127.0.0.1:43127")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", "invalid-csrf-token")
	request.AddCookie(security.CookieForSession(session))
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func mutationBoundary(manager *security.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return middleware.RequireHost(manager)(
			middleware.RequireSession(manager)(
				middleware.RequireOrigin(manager)(
					middleware.RequireCSRF(manager)(
						middleware.RequireJSONContentType(
							middleware.LimitBodySize(next),
						),
					),
				),
			),
		)
	}
}

func newSecurityManagerWithSession(t *testing.T) (*security.Manager, security.Session) {
	t.Helper()

	manager, err := security.NewManager(security.Config{
		BindAddr:       "127.0.0.1:43127",
		BootstrapToken: "test-bootstrap-token",
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	session, err := manager.ExchangeBootstrap("test-bootstrap-token", time.Now())
	if err != nil {
		t.Fatalf("ExchangeBootstrap() error = %v", err)
	}
	return manager, session
}
