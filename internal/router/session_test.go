package router_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExchangeBootstrapCreatesSessionOnce(t *testing.T) {
	handler := newTestHandler(t)

	first := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodPost, "/api/session/bootstrap", strings.NewReader(`{"bootstrapToken":"test-bootstrap-token"}`))
	firstRequest.Host = "127.0.0.1:43127"
	firstRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(first, firstRequest)

	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d, body = %s", first.Code, http.StatusOK, first.Body.String())
	}
	if len(first.Result().Cookies()) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(first.Result().Cookies()))
	}
	if !strings.Contains(first.Body.String(), `"authenticated":true`) {
		t.Fatalf("body = %q, want authenticated session", first.Body.String())
	}
	if strings.Contains(first.Body.String(), "test-bootstrap-token") {
		t.Fatal("response leaked bootstrap token")
	}

	second := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodPost, "/api/session/bootstrap", strings.NewReader(`{"bootstrapToken":"test-bootstrap-token"}`))
	secondRequest.Host = "127.0.0.1:43127"
	secondRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(second, secondRequest)

	if second.Code != http.StatusForbidden {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusForbidden)
	}
	if body := strings.TrimSpace(second.Body.String()); body != `{"data":null,"error":{"code":"FORBIDDEN","message":"请求被拒绝"}}` {
		t.Fatalf("second body = %q, want forbidden envelope", body)
	}
}

func TestGetSessionRequiresValidCookie(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.Host = "127.0.0.1:43127"
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"data":null,"error":{"code":"UNAUTHORIZED","message":"未登录或会话已失效"}}` {
		t.Fatalf("body = %q, want unauthorized envelope", body)
	}
}

func TestGetSessionReturnsCSRFTokenForAuthenticatedSession(t *testing.T) {
	handler := newTestHandler(t)
	cookie := exchangeBootstrapForCookie(t, handler)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.Host = "127.0.0.1:43127"
	request.AddCookie(cookie)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"authenticated":true`) || !strings.Contains(body, `"csrfToken":`) {
		t.Fatalf("body = %q, want authenticated session with csrf token", body)
	}
}

func TestRejectsInvalidHost(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	request.Host = "evil.test:43127"
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"data":null,"error":{"code":"FORBIDDEN","message":"请求被拒绝"}}` {
		t.Fatalf("body = %q, want forbidden envelope", body)
	}
}

func TestExchangeBootstrapRequiresJSONContentType(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/session/bootstrap", strings.NewReader(`{"bootstrapToken":"test-bootstrap-token"}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Content-Type", "text/plain")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"data":null,"error":{"code":"VALIDATION_FAILED","message":"Content-Type 必须是 application/json"}}` {
		t.Fatalf("body = %q, want validation envelope", body)
	}
}

func exchangeBootstrapForCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/session/bootstrap", strings.NewReader(`{"bootstrapToken":"test-bootstrap-token"}`))
	request.Host = "127.0.0.1:43127"
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}
	return cookies[0]
}
