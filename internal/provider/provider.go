package provider

import (
	"context"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

// Provider 定义 provider-neutral 的账号、登录、usage 和激活能力。
type Provider interface {
	Describe(context.Context) (Description, error)
	DiscoverCurrentAccount(context.Context) (*entity.Account, error)
	StartLogin(context.Context) (*LoginTask, error)
	PollLogin(context.Context, string) (*LoginStatus, error)
	CancelLogin(context.Context, string) error
	RefreshAccount(context.Context, entity.Account) (*entity.UsageSnapshot, error)
	ActivateAccount(context.Context, entity.Account) error
	RemoveAccountData(context.Context, entity.Account) error
	Close(context.Context) error
}

// Description 描述 provider 的稳定标识、展示名和能力。
type Description struct {
	ID           string
	DisplayName  string
	Capabilities Capabilities
	Status       Status
	ErrorCode    *entity.ErrorCode
}

// Status 表示 provider 在管理器中的可用状态。
type Status string

const (
	// StatusAvailable 表示 provider 已可用。
	StatusAvailable Status = "available"
	// StatusUnavailable 表示 provider 初始化失败或当前不可用。
	StatusUnavailable Status = "unavailable"
)

// Capabilities 描述 provider 支持的通用操作。
type Capabilities struct {
	CanImportCurrentAccount           bool
	CanLogin                          bool
	CanRefreshUsage                   bool
	CanActivateAccount                bool
	RequiresClientReloadAfterActivate bool
}

// LoginTask 表示 provider 启动的登录任务。
type LoginTask struct {
	ID        string
	AuthURL   string
	ExpiresAt int64
}

// LoginState 表示登录任务状态。
type LoginState string

const (
	// LoginStatePending 表示等待用户完成登录。
	LoginStatePending LoginState = "pending"
	// LoginStateCompleted 表示登录已完成。
	LoginStateCompleted LoginState = "completed"
	// LoginStateCanceled 表示登录已取消。
	LoginStateCanceled LoginState = "canceled"
	// LoginStateFailed 表示登录失败。
	LoginStateFailed LoginState = "failed"
)

// LoginStatus 表示登录任务轮询结果。
type LoginStatus struct {
	ID        string
	State     LoginState
	Account   *entity.Account
	ErrorCode *entity.ErrorCode
}

// Unsupported 返回能力不支持的稳定错误。
func Unsupported() error {
	return entity.NewAppError(entity.ErrorCodeUnsupported)
}
