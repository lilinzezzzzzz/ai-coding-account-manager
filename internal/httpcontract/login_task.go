package httpcontract

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/loginrunner"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

// CreateLoginTaskRequest 是创建 Codex 登录任务的 HTTP request。
type CreateLoginTaskRequest struct {
	ExpectedEmail string `json:"expectedEmail"`
	Mode          string `json:"mode"`
}

// ToServiceInput 校验并转换创建登录任务输入。
func (request CreateLoginTaskRequest) ToServiceInput(providerID string) (service.CreateLoginTaskInput, error) {
	mode := loginrunner.Mode(strings.TrimSpace(request.Mode))
	if mode == "" {
		mode = loginrunner.ModeBrowser
	}
	if mode != loginrunner.ModeBrowser && mode != loginrunner.ModeDeviceCode {
		return service.CreateLoginTaskInput{}, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "mode 无效")
	}
	expectedEmail := strings.TrimSpace(request.ExpectedEmail)
	if expectedEmail != "" && (len(expectedEmail) > 254 || !strings.Contains(expectedEmail, "@")) {
		return service.CreateLoginTaskInput{}, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "expectedEmail 无效")
	}
	return service.CreateLoginTaskInput{
		ProviderID:    providerID,
		Mode:          mode,
		ExpectedEmail: expectedEmail,
	}, nil
}

// LoginTaskResponse 是登录任务 HTTP response。
type LoginTaskResponse struct {
	TaskID        string                  `json:"taskId"`
	ProviderID    string                  `json:"providerId"`
	Status        service.LoginTaskStatus `json:"status"`
	Mode          loginrunner.Mode        `json:"mode"`
	ExpectedEmail *string                 `json:"expectedEmail"`
	LoginURL      *string                 `json:"loginUrl"`
	UserCode      *string                 `json:"userCode"`
	Account       *AccountResponse        `json:"account"`
	ErrorCode     *entity.ErrorCode       `json:"errorCode"`
	ErrorMessage  *string                 `json:"errorMessage"`
	CreatedAt     int64                   `json:"createdAt"`
	UpdatedAt     int64                   `json:"updatedAt"`
	ExpiresAt     int64                   `json:"expiresAt"`
}

// LoginTaskHTTPResponse 将 service 登录任务转换为 HTTP response。
func LoginTaskHTTPResponse(task service.LoginTask) LoginTaskResponse {
	response := LoginTaskResponse{
		TaskID:        task.TaskID,
		ProviderID:    task.ProviderID,
		Status:        task.Status,
		Mode:          task.Mode,
		ExpectedEmail: task.ExpectedEmail,
		LoginURL:      task.LoginURL,
		UserCode:      task.UserCode,
		ErrorCode:     task.ErrorCode,
		ErrorMessage:  task.ErrorMessage,
		CreatedAt:     task.CreatedAt,
		UpdatedAt:     task.UpdatedAt,
		ExpiresAt:     task.ExpiresAt,
	}
	if task.Account != nil {
		account := AccountEntityResponse(*task.Account, nil)
		response.Account = &account
	}
	return response
}

// LoginTaskID 从 route path 中解析 taskId。
func LoginTaskID(r *http.Request) (string, error) {
	taskID := chi.URLParam(r, "taskId")
	if !idPattern.MatchString(taskID) {
		return "", entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "taskId 无效")
	}
	return taskID, nil
}
