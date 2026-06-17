package httptransport_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
)

type strictRequest struct {
	Name string `json:"name"`
}

func TestDecodeStrictJSONRejectsUnknownField(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ok","extra":true}`))
	var body strictRequest

	err := httptransport.DecodeStrictJSON(request, &body)
	assertAppErrorCode(t, err, entity.ErrorCodeValidationFailed)
}

func TestDecodeStrictJSONRejectsMultipleValues(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ok"}{"name":"again"}`))
	var body strictRequest

	err := httptransport.DecodeStrictJSON(request, &body)
	assertAppErrorCode(t, err, entity.ErrorCodeValidationFailed)
}

func TestDecodeStrictJSONRejectsOversizedBody(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"too-large"}`))
	request.Body = http.MaxBytesReader(response, request.Body, 4)
	var body strictRequest

	err := httptransport.DecodeStrictJSON(request, &body)
	assertAppErrorCode(t, err, entity.ErrorCodePayloadTooLarge)
}

func assertAppErrorCode(t *testing.T, err error, want entity.ErrorCode) {
	t.Helper()

	appErr, ok := entity.AsAppError(err)
	if !ok {
		t.Fatalf("err = %v, want AppError", err)
	}
	if appErr.ErrorCode() != want {
		t.Fatalf("ErrorCode() = %q, want %q", appErr.ErrorCode(), want)
	}
}
