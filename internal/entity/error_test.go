package entity_test

import (
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

func TestAppErrorUsesDefaultMessageFromCode(t *testing.T) {
	err := entity.NewAppError(entity.ErrorCodeMethodNotAllowed)

	if err.ErrorCode() != "METHOD_NOT_ALLOWED" {
		t.Fatalf("ErrorCode() = %q, want METHOD_NOT_ALLOWED", err.ErrorCode())
	}
	if err.DisplayMessage() != "请求方法不支持" {
		t.Fatalf("DisplayMessage() = %q, want 请求方法不支持", err.DisplayMessage())
	}
}

func TestAppErrorAllowsMessageOverride(t *testing.T) {
	err := entity.NewAppErrorWithMessage(entity.ErrorCodeNotFound, "接口不存在")

	if err.ErrorCode() != "NOT_FOUND" {
		t.Fatalf("ErrorCode() = %q, want NOT_FOUND", err.ErrorCode())
	}
	if err.DisplayMessage() != "接口不存在" {
		t.Fatalf("DisplayMessage() = %q, want 接口不存在", err.DisplayMessage())
	}
}

func TestReauthenticationRequiredUsesStableDefaultMessage(t *testing.T) {
	err := entity.NewAppError(entity.ErrorCodeReauthenticationRequired)

	if err.ErrorCode() != entity.ErrorCodeReauthenticationRequired {
		t.Fatalf("ErrorCode() = %q, want %q", err.ErrorCode(), entity.ErrorCodeReauthenticationRequired)
	}
	if err.DisplayMessage() != "登录态已失效，请重新登录" {
		t.Fatalf("DisplayMessage() = %q, want 登录态已失效，请重新登录", err.DisplayMessage())
	}
}
