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

// RateLimitResetOutcome 表示 rate limit reset credit 的消费结果。
type RateLimitResetOutcome string

const (
	// RateLimitResetOutcomeReset 表示已消费 credit 并完成重置。
	RateLimitResetOutcomeReset RateLimitResetOutcome = "reset"
	// RateLimitResetOutcomeNothingToReset 表示当前没有符合条件的额度窗口。
	RateLimitResetOutcomeNothingToReset RateLimitResetOutcome = "nothingToReset"
	// RateLimitResetOutcomeNoCredit 表示账号没有可用的 reset credit。
	RateLimitResetOutcomeNoCredit RateLimitResetOutcome = "noCredit"
	// RateLimitResetOutcomeAlreadyRedeemed 表示该幂等键已完成过重置。
	RateLimitResetOutcomeAlreadyRedeemed RateLimitResetOutcome = "alreadyRedeemed"
)

// RateLimitResetResult 保存重置结果和重置后的账号状态。
type RateLimitResetResult struct {
	Outcome RateLimitResetOutcome
	Account *entity.Account
	Usage   *entity.UsageSnapshot
}

// Unsupported 返回能力不支持的稳定错误。
func Unsupported() error {
	return entity.NewAppError(entity.ErrorCodeUnsupported)
}
