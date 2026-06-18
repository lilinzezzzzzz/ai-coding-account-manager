package model

// Account 是 accounts 表的 GORM 持久化模型。
type Account struct {
	ProviderID    string  `gorm:"column:provider_id;primaryKey"`
	AccountID     string  `gorm:"column:account_id;primaryKey"`
	StorageID     string  `gorm:"column:storage_id"`
	Label         string  `gorm:"column:label"`
	Email         *string `gorm:"column:email"`
	PlanType      *string `gorm:"column:plan_type"`
	PlanExpiresAt *int64  `gorm:"column:plan_expires_at"`
	IsActive      bool    `gorm:"column:is_active"`
	CreatedAt     int64   `gorm:"column:created_at"`
	UpdatedAt     int64   `gorm:"column:updated_at"`
	LastUsedAt    *int64  `gorm:"column:last_used_at"`
}

// TableName 返回 accounts 表名。
func (Account) TableName() string {
	return "accounts"
}

// UsageSnapshot 是 usage_snapshots 表的 GORM 持久化模型。
type UsageSnapshot struct {
	ProviderID   string   `gorm:"column:provider_id;primaryKey"`
	AccountID    string   `gorm:"column:account_id;primaryKey"`
	Status       string   `gorm:"column:status"`
	UsedPercent  *float64 `gorm:"column:used_percent"`
	ResetsAt     *int64   `gorm:"column:resets_at"`
	SnapshotJSON *string  `gorm:"column:snapshot_json"`
	ErrorCode    *string  `gorm:"column:error_code"`
	RefreshedAt  int64    `gorm:"column:refreshed_at"`
}

// TableName 返回 usage_snapshots 表名。
func (UsageSnapshot) TableName() string {
	return "usage_snapshots"
}

// SchemaMigration 是 schema_migrations 表的 GORM 持久化模型。
type SchemaMigration struct {
	Version   int   `gorm:"column:version;primaryKey"`
	AppliedAt int64 `gorm:"column:applied_at"`
}

// TableName 返回 schema_migrations 表名。
func (SchemaMigration) TableName() string {
	return "schema_migrations"
}
