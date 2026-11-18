package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/dao"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/codexruntime"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/loginrunner"
)

const (
	defaultLoginTaskTTL = 5 * time.Minute
	loginTaskIDPrefix   = "login_"
	codexProviderID     = "codex"
)

// LoginTaskStatus 表示登录任务状态。
type LoginTaskStatus string

const (
	// LoginTaskStatusPending 表示任务已创建。
	LoginTaskStatusPending LoginTaskStatus = "pending"
	// LoginTaskStatusStarting 表示任务正在启动 Codex 登录进程。
	LoginTaskStatusStarting LoginTaskStatus = "starting"
	// LoginTaskStatusWaitingForUser 表示任务正在等待用户完成登录。
	LoginTaskStatusWaitingForUser LoginTaskStatus = "waiting_for_user"
	// LoginTaskStatusVerifying 表示任务正在读取 Codex 账号并导入凭据。
	LoginTaskStatusVerifying LoginTaskStatus = "verifying"
	// LoginTaskStatusImported 表示任务已导入账号。
	LoginTaskStatusImported LoginTaskStatus = "imported"
	// LoginTaskStatusFailed 表示任务失败。
	LoginTaskStatusFailed LoginTaskStatus = "failed"
	// LoginTaskStatusCancelled 表示任务已取消。
	LoginTaskStatusCancelled LoginTaskStatus = "cancelled"
	// LoginTaskStatusExpired 表示任务已过期。
	LoginTaskStatusExpired LoginTaskStatus = "expired"
)

// LoginTask 表示一次 Codex 登录任务的可展示状态。
type LoginTask struct {
	TaskID        string
	ProviderID    string
	Status        LoginTaskStatus
	Mode          loginrunner.Mode
	ExpectedEmail *string
	LoginURL      *string
	UserCode      *string
	Account       *entity.Account
	ErrorCode     *entity.ErrorCode
	ErrorMessage  *string
	CreatedAt     int64
	UpdatedAt     int64
	ExpiresAt     int64
}

// CreateLoginTaskInput 保存创建登录任务的输入。
type CreateLoginTaskInput struct {
	ProviderID    string
	Mode          loginrunner.Mode
	ExpectedEmail string
}

// CodexAccountImporter 描述从隔离 CODEX_HOME 读取、导入和激活 auth.json 的能力。
type CodexAccountImporter interface {
	ReadAccountFromCodexDir(context.Context, string) (*entity.Account, error)
	ImportAccountAuthFromCodexDir(context.Context, entity.Account, string) error
	ActivateAccount(context.Context, entity.Account) error
}

// LoginTaskService 编排 Codex 登录任务生命周期。
type LoginTaskService struct {
	uow        dao.UnitOfWork
	daos       dao.DAOs
	resolver   *codexruntime.Resolver
	runner     loginRunner
	importer   CodexAccountImporter
	rootDir    string
	now        func() time.Time
	taskTTL    time.Duration
	activation *AccountActivationCoordinator

	mu    sync.Mutex
	tasks map[string]*loginTaskState
}

type loginRunner interface {
	Run(context.Context, loginrunner.Input) (loginrunner.Result, error)
}

type loginTaskState struct {
	task   LoginTask
	cancel context.CancelFunc
	dir    string
}

// LoginTaskConfig 保存 LoginTaskService 依赖。
type LoginTaskConfig struct {
	UnitOfWork dao.UnitOfWork
	DAOs       dao.DAOs
	Resolver   *codexruntime.Resolver
	Runner     loginRunner
	Importer   CodexAccountImporter
	RootDir    string
	Now        func() time.Time
	TaskTTL    time.Duration
	Activation *AccountActivationCoordinator
}

// NewLoginTaskService 创建登录任务 service。
func NewLoginTaskService(cfg LoginTaskConfig) (*LoginTaskService, error) {
	if cfg.Resolver == nil {
		return nil, fmt.Errorf("codex runtime resolver is required")
	}
	if cfg.Runner == nil {
		return nil, fmt.Errorf("login runner is required")
	}
	if cfg.Importer == nil {
		return nil, fmt.Errorf("codex account importer is required")
	}
	if cfg.Activation == nil {
		return nil, fmt.Errorf("account activation coordinator is required")
	}
	if cfg.RootDir == "" {
		return nil, fmt.Errorf("login task root dir is required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	taskTTL := cfg.TaskTTL
	if taskTTL <= 0 {
		taskTTL = defaultLoginTaskTTL
	}
	if err := os.MkdirAll(cfg.RootDir, 0o700); err != nil {
		return nil, fmt.Errorf("create login task root dir: %w", err)
	}
	return &LoginTaskService{
		uow:        cfg.UnitOfWork,
		daos:       cfg.DAOs,
		resolver:   cfg.Resolver,
		runner:     cfg.Runner,
		importer:   cfg.Importer,
		rootDir:    cfg.RootDir,
		now:        now,
		taskTTL:    taskTTL,
		activation: cfg.Activation,
		tasks:      map[string]*loginTaskState{},
	}, nil
}

// Create 创建并异步启动 Codex 登录任务。
func (service *LoginTaskService) Create(ctx context.Context, input CreateLoginTaskInput) (LoginTask, error) {
	if input.ProviderID != codexProviderID {
		return LoginTask{}, entity.NewAppError(entity.ErrorCodeUnsupported)
	}
	mode := input.Mode
	if mode == "" {
		mode = loginrunner.ModeBrowser
	}
	if mode != loginrunner.ModeBrowser && mode != loginrunner.ModeDeviceCode {
		return LoginTask{}, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "登录方式不支持")
	}
	expectedEmail := strings.TrimSpace(input.ExpectedEmail)
	if expectedEmail != "" && (len(expectedEmail) > 254 || !strings.Contains(expectedEmail, "@")) {
		return LoginTask{}, entity.NewAppErrorWithMessage(entity.ErrorCodeValidationFailed, "expectedEmail 无效")
	}
	if err := ctx.Err(); err != nil {
		return LoginTask{}, err
	}

	taskID, err := newLoginTaskID()
	if err != nil {
		return LoginTask{}, err
	}
	taskDir := filepath.Join(service.rootDir, taskID)
	now := service.now().UTC()
	task := LoginTask{
		TaskID:     taskID,
		ProviderID: input.ProviderID,
		Status:     LoginTaskStatusPending,
		Mode:       mode,
		CreatedAt:  now.UnixMilli(),
		UpdatedAt:  now.UnixMilli(),
		ExpiresAt:  now.Add(service.taskTTL).UnixMilli(),
	}
	if expectedEmail != "" {
		task.ExpectedEmail = &expectedEmail
	}

	// 异步任务不跟随 HTTP 请求取消，但保留 Trace ID 等 context value。
	runCtx, cancel := context.WithDeadline(context.WithoutCancel(ctx), now.Add(service.taskTTL))
	state := &loginTaskState{task: task, cancel: cancel, dir: taskDir}

	service.mu.Lock()
	service.tasks[taskID] = state
	service.mu.Unlock()

	go func() {
		defer cancel()
		service.runTask(runCtx, task, taskDir)
	}()
	return task, nil
}

// Get 返回登录任务状态。
func (service *LoginTaskService) Get(ctx context.Context, providerID string, taskID string) (LoginTask, error) {
	if providerID != codexProviderID {
		return LoginTask{}, entity.NewAppError(entity.ErrorCodeUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return LoginTask{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	state, ok := service.tasks[taskID]
	if !ok {
		return LoginTask{}, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return state.task, nil
}

// Cancel 取消登录任务。
func (service *LoginTaskService) Cancel(ctx context.Context, providerID string, taskID string) (LoginTask, error) {
	if providerID != codexProviderID {
		return LoginTask{}, entity.NewAppError(entity.ErrorCodeUnsupported)
	}
	if err := ctx.Err(); err != nil {
		return LoginTask{}, err
	}
	service.mu.Lock()
	state, ok := service.tasks[taskID]
	if !ok {
		service.mu.Unlock()
		return LoginTask{}, entity.NewAppError(entity.ErrorCodeNotFound)
	}
	if isTerminalLoginTaskStatus(state.task.Status) {
		task := state.task
		service.mu.Unlock()
		return task, nil
	}
	state.cancel()
	state.task.Status = LoginTaskStatusCancelled
	state.task.UpdatedAt = service.now().UTC().UnixMilli()
	task := state.task
	taskDir := state.dir
	service.mu.Unlock()

	_ = os.RemoveAll(taskDir)
	return task, nil
}

func isTerminalLoginTaskStatus(status LoginTaskStatus) bool {
	switch status {
	case LoginTaskStatusImported, LoginTaskStatusFailed, LoginTaskStatusCancelled, LoginTaskStatusExpired:
		return true
	default:
		return false
	}
}

func newLoginTaskID() (string, error) {
	var randomBytes [12]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", entity.WrapAppError(entity.ErrorCodeInternal, err)
	}
	return loginTaskIDPrefix + hex.EncodeToString(randomBytes[:]), nil
}
