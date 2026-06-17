package httpcontract

import (
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// ProviderID 从 route path 中解析 providerId。
func ProviderID(r *http.Request) (string, error) {
	providerID := chi.URLParam(r, "providerId")
	if !idPattern.MatchString(providerID) {
		return "", entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "providerId 无效")
	}
	return providerID, nil
}

// ProviderAndAccountID 从 route path 中解析 providerId 和 accountId。
func ProviderAndAccountID(r *http.Request) (string, string, error) {
	providerID, err := ProviderID(r)
	if err != nil {
		return "", "", err
	}
	accountID := chi.URLParam(r, "accountId")
	if !idPattern.MatchString(accountID) {
		return "", "", entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "accountId 无效")
	}
	return providerID, accountID, nil
}
