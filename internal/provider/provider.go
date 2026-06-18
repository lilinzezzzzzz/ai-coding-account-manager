package provider

import (
	"context"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

// Provider 定义 provider-neutral 的账号 usage 和激活能力。
type Provider interface {
	Describe(context.Context) (Description, error)
	ImportCurrentAccount(context.Context) (*entity.Account, error)
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
	CanRefreshUsage                   bool
	CanActivateAccount                bool
	RequiresClientReloadAfterActivate bool
}

// Unsupported 返回能力不支持的稳定错误。
func Unsupported() error {
	return entity.NewAppError(entity.ErrorCodeUnsupported)
}
