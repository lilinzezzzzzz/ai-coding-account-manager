package controller

import (
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
	httptransport.WriteOK(w, response)
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
	httptransport.WriteOK(w, httpcontract.AccountViewResponse(account))
	return nil
}

// ImportAccountAuthJSON 为已有账号导入 auth.json。
func (controller AccountController) ImportAccountAuthJSON(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := httpcontract.ProviderAndAccountID(r)
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
	account, err := controller.accounts.ImportAccountAuthJSON(r.Context(), providerID, accountID, authJSON)
	if err != nil {
		return err
	}
	httptransport.WriteOK(w, httpcontract.AccountEntityResponse(account, nil))
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
	httptransport.WriteOK(w, httpcontract.AccountViewResponse(account))
	return nil
}

// RenameAccount 更新账号展示名称。
func (controller AccountController) RenameAccount(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := httpcontract.ProviderAndAccountID(r)
	if err != nil {
		return err
	}
	var request httpcontract.RenameAccountRequest
	if err := httptransport.DecodeStrictJSON(r, &request); err != nil {
		return err
	}
	label, err := request.NormalizedLabel()
	if err != nil {
		return err
	}
	account, err := controller.accounts.RenameAccount(r.Context(), providerID, accountID, label)
	if err != nil {
		return err
	}
	httptransport.WriteOK(w, httpcontract.AccountEntityResponse(account, nil))
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
	httptransport.WriteOK(w, httpcontract.AccountEntityResponse(account, nil))
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
	httptransport.WriteOK(w, httpcontract.AccountEntityResponse(account, nil))
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
	httptransport.WriteOK(w, map[string]bool{"deleted": true})
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
	httptransport.WriteOK(w, httpcontract.RefreshResultHTTPResponse(result))
	return nil
}
