package dao

import (
	"context"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UsageSnapshotDAO 封装 usage_snapshots 表访问。
type UsageSnapshotDAO struct {
	db *gorm.DB
}

// NewUsageSnapshotDAO 创建 usage snapshot DAO。
func NewUsageSnapshotDAO(db *gorm.DB) UsageSnapshotDAO {
	return UsageSnapshotDAO{db: db}
}

// Upsert 创建或更新账号最近 usage snapshot。
func (dao UsageSnapshotDAO) Upsert(ctx context.Context, snapshot entity.UsageSnapshot) error {
	record := usageSnapshotToModel(snapshot)
	err := dao.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "provider_id"},
			{Name: "account_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"status",
			"used_percent",
			"resets_at",
			"snapshot_json",
			"error_code",
			"refreshed_at",
		}),
	}).Create(&record).Error
	if err != nil {
		return mapDatabaseError(err)
	}
	return nil
}

// Get 根据 provider/account 读取最近 usage snapshot。
func (dao UsageSnapshotDAO) Get(ctx context.Context, providerID string, accountID string) (entity.UsageSnapshot, error) {
	var record model.UsageSnapshot
	err := dao.db.WithContext(ctx).
		Where("provider_id = ? AND account_id = ?", providerID, accountID).
		First(&record).Error
	if err != nil {
		return entity.UsageSnapshot{}, mapDatabaseError(err)
	}
	return usageSnapshotFromModel(record), nil
}

// ListAll 按 provider/account 稳定列出 usage snapshot。
func (dao UsageSnapshotDAO) ListAll(ctx context.Context, limit int) ([]entity.UsageSnapshot, error) {
	if limit <= 0 {
		limit = 500
	}

	var records []model.UsageSnapshot
	err := dao.db.WithContext(ctx).
		Order("provider_id ASC").
		Order("account_id ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	snapshots := make([]entity.UsageSnapshot, 0, len(records))
	for _, record := range records {
		snapshots = append(snapshots, usageSnapshotFromModel(record))
	}
	return snapshots, nil
}

// Delete 删除账号 usage snapshot。
func (dao UsageSnapshotDAO) Delete(ctx context.Context, providerID string, accountID string) error {
	result := dao.db.WithContext(ctx).
		Where("provider_id = ? AND account_id = ?", providerID, accountID).
		Delete(&model.UsageSnapshot{})
	if result.Error != nil {
		return mapDatabaseError(result.Error)
	}
	if result.RowsAffected == 0 {
		return entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return nil
}

func usageSnapshotToModel(snapshot entity.UsageSnapshot) model.UsageSnapshot {
	var errorCode *string
	if snapshot.ErrorCode != nil {
		value := string(*snapshot.ErrorCode)
		errorCode = &value
	}

	return model.UsageSnapshot{
		ProviderID:   snapshot.ProviderID,
		AccountID:    snapshot.AccountID,
		Status:       string(snapshot.Status),
		UsedPercent:  snapshot.UsedPercent,
		ResetsAt:     snapshot.ResetsAt,
		SnapshotJSON: snapshot.SnapshotJSON,
		ErrorCode:    errorCode,
		RefreshedAt:  snapshot.RefreshedAt,
	}
}

func usageSnapshotFromModel(record model.UsageSnapshot) entity.UsageSnapshot {
	var errorCode *entity.ErrorCode
	if record.ErrorCode != nil {
		value := entity.ErrorCode(*record.ErrorCode)
		errorCode = &value
	}

	return entity.UsageSnapshot{
		ProviderID:   record.ProviderID,
		AccountID:    record.AccountID,
		Status:       entity.UsageStatus(record.Status),
		UsedPercent:  record.UsedPercent,
		ResetsAt:     record.ResetsAt,
		SnapshotJSON: record.SnapshotJSON,
		ErrorCode:    errorCode,
		RefreshedAt:  record.RefreshedAt,
	}
}
