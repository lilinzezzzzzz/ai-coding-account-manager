package controller

import (
	"log/slog"
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httpcontract"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

// AccountController 处理账号核心 API。
type AccountController struct {
	accounts *service.AccountService
}

// NewAccountController 创建账号 controller。
func NewAccountController(accounts *service.AccountService) AccountController {
	return AccountController{accounts: accounts}
}

// ListAccounts 返回账号列表和最近 usage snapshot。
func (controller AccountController) ListAccounts(w http.ResponseWriter, r *http.Request) error {
	accounts, err := controller.accounts.ListAccounts(r.Context())
	if err != nil {
		return err
	}
	response := make([]httpcontract.AccountResponse, 0, len(accounts))
	for _, account := range accounts {
		response = append(response, httpcontract.AccountViewResponse(account))
	}
	httptransport.WriteOK(r.Context(), w, response)
	return nil
}

// CreateAccount 根据 OpenAI 账号邮箱创建本地账号。
func (controller AccountController) CreateAccount(w http.ResponseWriter, r *http.Request) error {
	providerID, err := httpcontract.ProviderID(r)
	if err != nil {
		return err
	}
	var request httpcontract.CreateAccountRequest
	if err := httptransport.DecodeStrictJSON(r, &request); err != nil {
		return err
	}
	email, err := request.NormalizedEmail()
	if err != nil {
		return err
	}
	account, err := controller.accounts.CreateManualAccount(r.Context(), service.CreateManualAccountInput{
		ProviderID: providerID,
		Email:      email,
	})
	if err != nil {
		return err
	}
	slog.InfoContext(r.Context(), "account created", "provider_id", account.Account.ProviderID, "account_id", account.Account.AccountID)
	httptransport.WriteOK(r.Context(), w, httpcontract.AccountViewResponse(account))
	return nil
}

// ImportAccountAuthJSONAndRefresh 导入 auth.json，自动识别账号并刷新状态。
func (controller AccountController) ImportAccountAuthJSONAndRefresh(w http.ResponseWriter, r *http.Request) error {
	providerID, err := httpcontract.ProviderID(r)
	if err != nil {
		return err
	}
	var request httpcontract.ImportAccountAuthJSONRequest
	if err := httptransport.DecodeStrictJSON(r, &request); err != nil {
		return err
	}
	authJSON, err := request.NormalizedAuthJSON()
	if err != nil {
		return err
	}
	account, err := controller.accounts.ImportAccountAuthJSONAndRefresh(r.Context(), providerID, authJSON)
	if err != nil {
		return err
	}
	slog.InfoContext(r.Context(), "account auth imported and refreshed", "provider_id", account.Account.ProviderID, "account_id", account.Account.AccountID)
	httptransport.WriteOK(r.Context(), w, httpcontract.AccountViewResponse(account))
	return nil
}

// ImportCurrentAccount 从当前活动 Codex 登录态导入账号。
func (controller AccountController) ImportCurrentAccount(w http.ResponseWriter, r *http.Request) error {
	providerID, err := httpcontract.ProviderID(r)
	if err != nil {
		return err
	}
	account, err := controller.accounts.ImportCurrentAccount(r.Context(), providerID)
	if err != nil {
		return err
	}
	slog.InfoContext(r.Context(), "current account imported", "provider_id", account.Account.ProviderID, "account_id", account.Account.AccountID)
	httptransport.WriteOK(r.Context(), w, httpcontract.AccountViewResponse(account))
	return nil
}

// UpdatePlanExpiration 更新人工维护的套餐到期时间。
func (controller AccountController) UpdatePlanExpiration(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := httpcontract.ProviderAndAccountID(r)
	if err != nil {
		return err
	}
	var request httpcontract.UpdatePlanExpirationRequest
	if err := httptransport.DecodeStrictJSON(r, &request); err != nil {
		return err
	}
	planExpiresAt, err := request.NormalizedPlanExpiresAt()
	if err != nil {
		return err
	}
	account, err := controller.accounts.UpdatePlanExpiration(r.Context(), providerID, accountID, planExpiresAt)
	if err != nil {
		return err
	}
	slog.InfoContext(r.Context(),
		"account plan expiration updated",
		"provider_id", account.ProviderID,
		"account_id", account.AccountID,
		"plan_expires_at_set", account.PlanExpiresAt != nil,
	)
	httptransport.WriteOK(r.Context(), w, httpcontract.AccountEntityResponse(account, nil))
	return nil
}

// ReloginAccount 是 post-MVP 能力，当前返回稳定 unsupported。
func (controller AccountController) ReloginAccount(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppError(entity.ErrorCodeUnsupported)
}

// ActivateAccount 激活账号。
func (controller AccountController) ActivateAccount(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := httpcontract.ProviderAndAccountID(r)
	if err != nil {
		return err
	}
	account, err := controller.accounts.ActivateAccount(r.Context(), providerID, accountID)
	if err != nil {
		return err
	}
	slog.InfoContext(r.Context(), "account activated", "provider_id", account.ProviderID, "account_id", account.AccountID)
	httptransport.WriteOK(r.Context(), w, httpcontract.AccountEntityResponse(account, nil))
	return nil
}

// DeleteAccount 删除非 active 账号。
func (controller AccountController) DeleteAccount(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := httpcontract.ProviderAndAccountID(r)
	if err != nil {
		return err
	}
	if err := controller.accounts.DeleteAccount(r.Context(), providerID, accountID); err != nil {
		return err
	}
	slog.InfoContext(r.Context(), "account deleted", "provider_id", providerID, "account_id", accountID)
	httptransport.WriteOK(r.Context(), w, map[string]bool{"deleted": true})
	return nil
}

// RefreshAccount 刷新单个账号状态。
func (controller AccountController) RefreshAccount(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := httpcontract.ProviderAndAccountID(r)
	if err != nil {
		return err
	}
	result, err := controller.accounts.RefreshAccount(r.Context(), providerID, accountID)
	if err != nil {
		return err
	}
	if result.Account != nil {
		var usageStatus entity.UsageStatus
		if result.Account.Usage != nil {
			usageStatus = result.Account.Usage.Status
		}
		slog.InfoContext(r.Context(),
			"account refreshed",
			"provider_id", result.ProviderID,
			"account_id", result.AccountID,
			"usage_status", usageStatus,
		)
	}
	httptransport.WriteOK(r.Context(), w, httpcontract.RefreshResultHTTPResponse(result))
	return nil
}

// ResetAccountRateLimit 消耗一次 reset credit 并返回最新账号状态。
func (controller AccountController) ResetAccountRateLimit(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := httpcontract.ProviderAndAccountID(r)
	if err != nil {
		return err
	}
	var request httpcontract.ResetRateLimitRequest
	if err := httptransport.DecodeStrictJSON(r, &request); err != nil {
		return err
	}
	idempotencyKey, err := request.NormalizedIdempotencyKey()
	if err != nil {
		return err
	}
	result, err := controller.accounts.ResetAccountRateLimit(r.Context(), providerID, accountID, idempotencyKey)
	if err != nil {
		return err
	}
	slog.InfoContext(r.Context(),
		"account rate limit reset attempted",
		"provider_id", providerID,
		"account_id", accountID,
		"outcome", result.Outcome,
	)
	httptransport.WriteOK(r.Context(), w, httpcontract.ResetRateLimitHTTPResponse(result))
	return nil
}
