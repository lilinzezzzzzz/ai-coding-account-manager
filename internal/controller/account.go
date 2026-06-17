package controller

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type accountResponse struct {
	ProviderID string                 `json:"providerId"`
	AccountID  string                 `json:"accountId"`
	StorageID  string                 `json:"storageId"`
	Label      string                 `json:"label"`
	Email      *string                `json:"email"`
	PlanType   *string                `json:"planType"`
	IsActive   bool                   `json:"isActive"`
	CreatedAt  int64                  `json:"createdAt"`
	UpdatedAt  int64                  `json:"updatedAt"`
	LastUsedAt *int64                 `json:"lastUsedAt"`
	Usage      *usageSnapshotResponse `json:"usage"`
}

type usageSnapshotResponse struct {
	Status       entity.UsageStatus `json:"status"`
	UsedPercent  *float64           `json:"usedPercent"`
	ResetsAt     *int64             `json:"resetsAt"`
	SnapshotJSON *string            `json:"snapshotJson"`
	ErrorCode    *entity.ErrorCode  `json:"errorCode"`
	RefreshedAt  int64              `json:"refreshedAt"`
}

type renameAccountRequest struct {
	Label string `json:"label"`
}

type createAccountRequest struct {
	Email string `json:"email"`
}

type refreshResultResponse struct {
	ProviderID string                 `json:"providerId"`
	AccountID  string                 `json:"accountId"`
	Usage      *usageSnapshotResponse `json:"usage"`
	ErrorCode  *entity.ErrorCode      `json:"errorCode"`
}

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
	response := make([]accountResponse, 0, len(accounts))
	for _, account := range accounts {
		response = append(response, accountViewToResponse(account))
	}
	httptransport.WriteOK(w, response)
	return nil
}

// CreateAccount 根据 OpenAI 账号邮箱创建本地账号。
func (controller AccountController) CreateAccount(w http.ResponseWriter, r *http.Request) error {
	providerID, err := providerIDFromRequest(r)
	if err != nil {
		return err
	}
	var request createAccountRequest
	if err := httptransport.DecodeStrictJSON(r, &request); err != nil {
		return err
	}
	email := strings.TrimSpace(request.Email)
	if email == "" || len(email) > 254 || !strings.Contains(email, "@") {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "email 无效")
	}
	account, err := controller.accounts.CreateManualAccount(r.Context(), service.CreateManualAccountInput{
		ProviderID: providerID,
		Email:      email,
	})
	if err != nil {
		return err
	}
	httptransport.WriteOK(w, accountViewToResponse(account))
	return nil
}

// RenameAccount 更新账号展示名称。
func (controller AccountController) RenameAccount(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := providerAndAccountIDFromRequest(r)
	if err != nil {
		return err
	}
	var request renameAccountRequest
	if err := httptransport.DecodeStrictJSON(r, &request); err != nil {
		return err
	}
	label := strings.TrimSpace(request.Label)
	if label == "" || len(label) > 120 {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "label 长度必须在 1 到 120 之间")
	}
	account, err := controller.accounts.RenameAccount(r.Context(), providerID, accountID, label)
	if err != nil {
		return err
	}
	httptransport.WriteOK(w, accountToResponse(account, nil))
	return nil
}

// ReloginAccount 是 post-MVP 能力，当前返回稳定 unsupported。
func (controller AccountController) ReloginAccount(_ http.ResponseWriter, _ *http.Request) error {
	return entity.NewAppError(entity.ErrorCodeUnsupported)
}

// ActivateAccount 激活账号。
func (controller AccountController) ActivateAccount(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := providerAndAccountIDFromRequest(r)
	if err != nil {
		return err
	}
	account, err := controller.accounts.ActivateAccount(r.Context(), providerID, accountID)
	if err != nil {
		return err
	}
	httptransport.WriteOK(w, accountToResponse(account, nil))
	return nil
}

// DeleteAccount 删除非 active 账号。
func (controller AccountController) DeleteAccount(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := providerAndAccountIDFromRequest(r)
	if err != nil {
		return err
	}
	if err := controller.accounts.DeleteAccount(r.Context(), providerID, accountID); err != nil {
		return err
	}
	httptransport.WriteOK(w, map[string]bool{"deleted": true})
	return nil
}

// RefreshAccountUsage 刷新单个账号 usage。
func (controller AccountController) RefreshAccountUsage(w http.ResponseWriter, r *http.Request) error {
	providerID, accountID, err := providerAndAccountIDFromRequest(r)
	if err != nil {
		return err
	}
	result, err := controller.accounts.RefreshAccountUsage(r.Context(), providerID, accountID)
	if err != nil {
		return err
	}
	httptransport.WriteOK(w, refreshResultToResponse(result))
	return nil
}

func accountViewToResponse(view service.AccountWithUsage) accountResponse {
	return accountToResponse(view.Account, view.Usage)
}

func accountToResponse(account entity.Account, usage *entity.UsageSnapshot) accountResponse {
	response := accountResponse{
		ProviderID: account.ProviderID,
		AccountID:  account.AccountID,
		StorageID:  account.StorageID,
		Label:      account.Label,
		Email:      account.Email,
		PlanType:   account.PlanType,
		IsActive:   account.IsActive,
		CreatedAt:  account.CreatedAt,
		UpdatedAt:  account.UpdatedAt,
		LastUsedAt: account.LastUsedAt,
	}
	if usage != nil {
		usageResponse := usageToResponse(*usage)
		response.Usage = &usageResponse
	}
	return response
}

func usageToResponse(usage entity.UsageSnapshot) usageSnapshotResponse {
	return usageSnapshotResponse{
		Status:       usage.Status,
		UsedPercent:  usage.UsedPercent,
		ResetsAt:     usage.ResetsAt,
		SnapshotJSON: usage.SnapshotJSON,
		ErrorCode:    usage.ErrorCode,
		RefreshedAt:  usage.RefreshedAt,
	}
}

func refreshResultToResponse(result service.RefreshResult) refreshResultResponse {
	response := refreshResultResponse{
		ProviderID: result.ProviderID,
		AccountID:  result.AccountID,
		ErrorCode:  result.ErrorCode,
	}
	if result.Usage != nil {
		usage := usageToResponse(*result.Usage)
		response.Usage = &usage
	}
	return response
}

func providerAndAccountIDFromRequest(r *http.Request) (string, string, error) {
	providerID, err := providerIDFromRequest(r)
	if err != nil {
		return "", "", err
	}
	accountID := chi.URLParam(r, "accountId")
	if !idPattern.MatchString(accountID) {
		return "", "", entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "accountId 无效")
	}
	return providerID, accountID, nil
}

func providerIDFromRequest(r *http.Request) (string, error) {
	providerID := chi.URLParam(r, "providerId")
	if !idPattern.MatchString(providerID) {
		return "", entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "providerId 无效")
	}
	return providerID, nil
}
