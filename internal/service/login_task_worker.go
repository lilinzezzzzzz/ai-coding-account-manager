package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/loginrunner"
)

func (service *LoginTaskService) runTask(ctx context.Context, task LoginTask, taskDir string) {
	taskID := task.TaskID
	codexHome := filepath.Join(taskDir, "codex-home")
	defer func() { _ = os.RemoveAll(taskDir) }()

	service.updateTask(taskID, func(task *LoginTask) {
		task.Status = LoginTaskStatusStarting
	})
	runtime, err := service.resolver.Resolve(ctx)
	if err != nil {
		service.failTask(ctx, taskID, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "未找到可用 Codex runtime", err))
		return
	}

	service.updateTask(taskID, func(task *LoginTask) {
		task.Status = LoginTaskStatusWaitingForUser
	})
	_, err = service.runner.Run(ctx, loginrunner.Input{
		RuntimePath: runtime.Path,
		CodexHome:   codexHome,
		Mode:        task.Mode,
		OnProgress: func(progress loginrunner.Progress) {
			service.updateTask(taskID, func(task *LoginTask) {
				if progress.LoginURL != nil {
					task.LoginURL = progress.LoginURL
				}
				if progress.UserCode != nil {
					task.UserCode = progress.UserCode
				}
			})
		},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			service.markCancelled(ctx, taskID)
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			service.markExpired(ctx, taskID)
			return
		}
		service.failTask(ctx, taskID, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "Codex 登录失败", err))
		return
	}

	service.updateTask(taskID, func(task *LoginTask) {
		task.Status = LoginTaskStatusVerifying
	})
	account, err := service.importer.ReadAccountFromCodexDir(ctx, codexHome)
	if err != nil {
		service.failTask(ctx, taskID, err)
		return
	}
	if err := validateExpectedLoginEmail(task.ExpectedEmail, account); err != nil {
		service.failTask(ctx, taskID, err)
		return
	}
	if account == nil || strings.TrimSpace(account.AccountID) == "" {
		service.failTask(ctx, taskID, entity.NewAppErrorWithMessage(entity.ErrorCodeUnavailable, "无法识别登录账号"))
		return
	}
	if err := service.importer.ImportAccountAuthFromCodexDir(ctx, *account, codexHome); err != nil {
		service.failTask(ctx, taskID, err)
		return
	}
	persisted, err := service.persistAndSyncImportedAccount(ctx, *account)
	if err != nil {
		service.failTask(ctx, taskID, err)
		return
	}

	service.updateTask(taskID, func(task *LoginTask) {
		task.Status = LoginTaskStatusImported
		task.Account = &persisted
		task.ErrorCode = nil
		task.ErrorMessage = nil
	})
	slog.InfoContext(ctx,
		"login task imported account",
		"provider_id", persisted.ProviderID,
		"task_id", taskID,
		"account_id", persisted.AccountID,
	)
}

func (service *LoginTaskService) persistAndSyncImportedAccount(ctx context.Context, account entity.Account) (entity.Account, error) {
	if err := service.activation.acquire(ctx); err != nil {
		return entity.Account{}, err
	}
	defer service.activation.release()

	persisted, err := service.persistImportedAccount(ctx, account)
	if err != nil {
		return entity.Account{}, err
	}
	if !persisted.IsActive {
		return persisted, nil
	}
	if err := service.importer.ActivateAccount(ctx, persisted); err != nil {
		return entity.Account{}, entity.WrapAppErrorWithMessage(entity.ErrorCodeUnavailable, "同步当前 Codex 登录态失败", err)
	}
	return persisted, nil
}

func validateExpectedLoginEmail(expectedEmail *string, account *entity.Account) error {
	if expectedEmail == nil {
		return nil
	}
	if account == nil || account.Email == nil || !strings.EqualFold(strings.TrimSpace(*expectedEmail), strings.TrimSpace(*account.Email)) {
		return entity.NewAppErrorWithMessage(entity.ErrorCodeConflict, "登录账号和期望邮箱不匹配")
	}
	return nil
}

func (service *LoginTaskService) persistImportedAccount(ctx context.Context, account entity.Account) (entity.Account, error) {
	account = normalizeAccount(account.ProviderID, account, service.now().UTC().UnixMilli())
	existing, err := service.daos.Accounts.Get(ctx, account.ProviderID, account.AccountID)
	if err == nil {
		account.IsActive = existing.IsActive
		account.LastUsedAt = existing.LastUsedAt
		account.CreatedAt = existing.CreatedAt
	} else if !isAppErrorCode(err, entity.ErrorCodeNotFound) {
		return entity.Account{}, err
	}

	if err := service.uow.WithinTransaction(ctx, func(daos dao.DAOs) error {
		return daos.Accounts.Upsert(ctx, account)
	}); err != nil {
		return entity.Account{}, err
	}
	return service.daos.Accounts.Get(ctx, account.ProviderID, account.AccountID)
}

func (service *LoginTaskService) updateTask(taskID string, update func(*LoginTask)) {
	service.mu.Lock()
	defer service.mu.Unlock()
	state, ok := service.tasks[taskID]
	if !ok || isTerminalLoginTaskStatus(state.task.Status) {
		return
	}
	update(&state.task)
	state.task.UpdatedAt = service.now().UTC().UnixMilli()
}

func (service *LoginTaskService) failTask(ctx context.Context, taskID string, err error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	state, ok := service.tasks[taskID]
	if !ok || isTerminalLoginTaskStatus(state.task.Status) {
		return
	}
	code := entity.ErrorCodeInternal
	message := entity.ErrorCodeInternal.DefaultMessage()
	if appErr, ok := entity.AsAppError(err); ok {
		code = appErr.ErrorCode()
		message = appErr.DisplayMessage()
	}
	state.task.Status = LoginTaskStatusFailed
	state.task.ErrorCode = &code
	state.task.ErrorMessage = &message
	state.task.UpdatedAt = service.now().UTC().UnixMilli()
	fields := []any{
		"provider_id", state.task.ProviderID,
		"task_id", taskID,
		"error_code", code,
		"error", err,
	}
	if appErr, ok := entity.AsAppError(err); ok && appErr.Cause != nil {
		fields = append(fields, "cause", appErr.Cause, "cause_type", fmt.Sprintf("%T", appErr.Cause))
	}
	slog.WarnContext(ctx, "login task failed", fields...)
}

func (service *LoginTaskService) markCancelled(ctx context.Context, taskID string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if state, ok := service.tasks[taskID]; ok && !isTerminalLoginTaskStatus(state.task.Status) {
		state.task.Status = LoginTaskStatusCancelled
		state.task.UpdatedAt = service.now().UTC().UnixMilli()
		slog.InfoContext(ctx, "login task cancelled", "provider_id", state.task.ProviderID, "task_id", taskID)
	}
}

func (service *LoginTaskService) markExpired(ctx context.Context, taskID string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if state, ok := service.tasks[taskID]; ok && !isTerminalLoginTaskStatus(state.task.Status) {
		code := entity.ErrorCodeUnavailable
		message := "Codex 登录任务已超时"
		state.task.Status = LoginTaskStatusExpired
		state.task.ErrorCode = &code
		state.task.ErrorMessage = &message
		state.task.UpdatedAt = service.now().UTC().UnixMilli()
		slog.WarnContext(ctx, "login task expired", "provider_id", state.task.ProviderID, "task_id", taskID, "error_code", code)
	}
}
