package controller

import (
	"log/slog"
	"net/http"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httpcontract"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/httptransport"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/service"
)

// LoginTaskController 处理 Codex 登录任务 API。
type LoginTaskController struct {
	loginTasks *service.LoginTaskService
}

// NewLoginTaskController 创建登录任务 controller。
func NewLoginTaskController(loginTasks *service.LoginTaskService) LoginTaskController {
	return LoginTaskController{loginTasks: loginTasks}
}

// CreateLoginTask 创建 Codex 登录任务。
func (controller LoginTaskController) CreateLoginTask(w http.ResponseWriter, r *http.Request) error {
	providerID, err := httpcontract.ProviderID(r)
	if err != nil {
		return err
	}
	var request httpcontract.CreateLoginTaskRequest
	if err := httptransport.DecodeStrictJSON(r, &request); err != nil {
		return err
	}
	input, err := request.ToServiceInput(providerID)
	if err != nil {
		return err
	}
	task, err := controller.loginTasks.Create(r.Context(), input)
	if err != nil {
		return err
	}
	slog.InfoContext(r.Context(),
		"login task created",
		"provider_id", task.ProviderID,
		"task_id", task.TaskID,
		"mode", task.Mode,
		"expected_email_set", task.ExpectedEmail != nil,
	)
	httptransport.WriteOK(r.Context(), w, httpcontract.LoginTaskHTTPResponse(task))
	return nil
}

// GetLoginTask 返回 Codex 登录任务状态。
func (controller LoginTaskController) GetLoginTask(w http.ResponseWriter, r *http.Request) error {
	providerID, err := httpcontract.ProviderID(r)
	if err != nil {
		return err
	}
	taskID, err := httpcontract.LoginTaskID(r)
	if err != nil {
		return err
	}
	task, err := controller.loginTasks.Get(r.Context(), providerID, taskID)
	if err != nil {
		return err
	}
	httptransport.WriteOK(r.Context(), w, httpcontract.LoginTaskHTTPResponse(task))
	return nil
}

// CancelLoginTask 取消 Codex 登录任务。
func (controller LoginTaskController) CancelLoginTask(w http.ResponseWriter, r *http.Request) error {
	providerID, err := httpcontract.ProviderID(r)
	if err != nil {
		return err
	}
	taskID, err := httpcontract.LoginTaskID(r)
	if err != nil {
		return err
	}
	task, err := controller.loginTasks.Cancel(r.Context(), providerID, taskID)
	if err != nil {
		return err
	}
	slog.InfoContext(r.Context(),
		"login task cancel requested",
		"provider_id", task.ProviderID,
		"task_id", task.TaskID,
		"status", task.Status,
	)
	httptransport.WriteOK(r.Context(), w, httpcontract.LoginTaskHTTPResponse(task))
	return nil
}
