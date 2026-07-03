package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/middleware"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/tracing"
)

func TestTraceRequestPreservesValidClientTraceID(t *testing.T) {
	const clientTraceID = "client-request-123"
	handler := middleware.TraceRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(tracing.HeaderName); got != clientTraceID {
			t.Fatalf("request trace ID = %q, want %q", got, clientTraceID)
		}
		if got := tracing.TraceIDFromContext(r.Context()); got != clientTraceID {
			t.Fatalf("context trace ID = %q, want %q", got, clientTraceID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	request.Header.Set(tracing.HeaderName, clientTraceID)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if got := response.Header().Get(tracing.HeaderName); got != clientTraceID {
		t.Fatalf("response trace ID = %q, want %q", got, clientTraceID)
	}
}

func TestTraceRequestGeneratesTraceIDWhenMissingOrInvalid(t *testing.T) {
	for _, supplied := range []string{"", "invalid trace id"} {
		t.Run(supplied, func(t *testing.T) {
			handler := middleware.TraceRequest(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				traceID := tracing.TraceIDFromContext(r.Context())
				if got := r.Header.Get(tracing.HeaderName); got != traceID {
					t.Fatalf("request trace ID = %q, want %q", got, traceID)
				}
				if !tracing.IsValidTraceID(traceID) || traceID == supplied {
					t.Fatalf("generated context trace ID = %q", traceID)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			request.Header.Set(tracing.HeaderName, supplied)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if got := response.Header().Get(tracing.HeaderName); !tracing.IsValidTraceID(got) {
				t.Fatalf("response trace ID = %q, want valid generated value", got)
			}
		})
	}
}
