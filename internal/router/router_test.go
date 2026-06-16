package router_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/router"
)

func TestHealthEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", got)
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"data":{"status":"ok"},"error":null}` {
		t.Fatalf("body = %q, want health envelope", body)
	}
	assertSecurityHeaders(t, response.Result().Header)
}

func TestStaticIndexEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "AI Coding Account Manager") {
		t.Fatalf("index response does not contain expected title: %q", response.Body.String())
	}
	assertSecurityHeaders(t, response.Result().Header)
}

func TestMissingStaticAsset(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	assertSecurityHeaders(t, response.Result().Header)
}

func TestMissingAPIEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"data":null,"error":{"code":"NOT_FOUND","message":"接口不存在"}}` {
		t.Fatalf("body = %q, want not found envelope", body)
	}
	assertSecurityHeaders(t, response.Result().Header)
}

func TestMethodNotAllowedAPIEndpoint(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"data":null,"error":{"code":"METHOD_NOT_ALLOWED","message":"请求方法不支持"}}` {
		t.Fatalf("body = %q, want method not allowed envelope", body)
	}
	assertSecurityHeaders(t, response.Result().Header)
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	testFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("<!doctype html><title>AI Coding Account Manager</title>"),
			Mode: fs.ModePerm,
		},
		"app.css": &fstest.MapFile{
			Data: []byte("body { margin: 0; }"),
			Mode: fs.ModePerm,
		},
	}

	handler, err := router.NewHandler(testFS)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func assertSecurityHeaders(t *testing.T, header http.Header) {
	t.Helper()

	expected := map[string]string{
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"Cache-Control":           "no-store",
	}
	for name, want := range expected {
		if got := header.Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}
