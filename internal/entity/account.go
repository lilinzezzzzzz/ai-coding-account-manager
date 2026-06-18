package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Account 表示 provider-neutral 的账号元数据。
type Account struct {
	ProviderID    string
	AccountID     string
	StorageID     string
	Label         string
	Email         *string
	PlanType      *string
	PlanExpiresAt *int64
	IsActive      bool
	CreatedAt     int64
	UpdatedAt     int64
	LastUsedAt    *int64
}

// UsageStatus 表示账号 usage snapshot 的稳定状态。
type UsageStatus string

const (
	// UsageStatusReady 表示 usage 数据可正常展示。
	UsageStatusReady UsageStatus = "ready"
	// UsageStatusRefreshing 表示账号正在刷新。
	UsageStatusRefreshing UsageStatus = "refreshing"
	// UsageStatusAuthExpired 表示账号登录态已失效。
	UsageStatusAuthExpired UsageStatus = "auth_expired"
	// UsageStatusRateLimitReached 表示额度已达到限制。
	UsageStatusRateLimitReached UsageStatus = "rate_limit_reached"
	// UsageStatusUnavailable 表示 provider 当前不可用。
	UsageStatusUnavailable UsageStatus = "unavailable"
	// UsageStatusUnsupported 表示 provider 不支持 usage。
	UsageStatusUnsupported UsageStatus = "unsupported"
)

// UsageSnapshot 表示数据库中保存的最近一次账号 usage 状态。
type UsageSnapshot struct {
	ProviderID   string
	AccountID    string
	Status       UsageStatus
	UsedPercent  *float64
	ResetsAt     *int64
	SnapshotJSON *string
	ErrorCode    *ErrorCode
	RefreshedAt  int64
}

// StorageIDForAccount 返回账号隔离凭据目录 ID。
func StorageIDForAccount(providerID string, accountID string) string {
	return accountID
}

// AccountIDFromEmail 根据 OpenAI 邮箱生成稳定账号 ID。
func AccountIDFromEmail(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return "chatgpt-" + hex.EncodeToString(sum[:])[:20]
}
