package dao

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	modernsqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// AccountDAO 封装 accounts 表访问。
type AccountDAO struct {
	db *gorm.DB
}

// NewAccountDAO 创建账号 DAO。
func NewAccountDAO(db *gorm.DB) AccountDAO {
	return AccountDAO{db: db}
}

// Create 创建账号记录。
func (dao AccountDAO) Create(ctx context.Context, account entity.Account) error {
	record := accountToModel(account)
	if err := dao.db.WithContext(ctx).Create(&record).Error; err != nil {
		return mapDatabaseError(err)
	}
	return nil
}

// Upsert 以复合账号键创建或更新账号元数据。
func (dao AccountDAO) Upsert(ctx context.Context, account entity.Account) error {
	record := accountToModel(account)
	err := dao.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "provider_id"},
			{Name: "account_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"storage_id",
			"label",
			"email",
			"plan_type",
			"is_active",
			"updated_at",
			"last_used_at",
		}),
	}).Create(&record).Error
	if err != nil {
		return mapDatabaseError(err)
	}
	return nil
}

// Get 根据 provider/account 读取账号。
func (dao AccountDAO) Get(ctx context.Context, providerID string, accountID string) (entity.Account, error) {
	var record model.Account
	err := dao.db.WithContext(ctx).
		Where("provider_id = ? AND account_id = ?", providerID, accountID).
		First(&record).Error
	if err != nil {
		return entity.Account{}, mapDatabaseError(err)
	}
	return accountFromModel(record), nil
}

// ListByProvider 按 provider 稳定列出账号。
func (dao AccountDAO) ListByProvider(ctx context.Context, providerID string, limit int) ([]entity.Account, error) {
	if limit <= 0 {
		limit = 100
	}

	var records []model.Account
	err := dao.db.WithContext(ctx).
		Where("provider_id = ?", providerID).
		Order("account_id ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, mapDatabaseError(err)
	}

	accounts := make([]entity.Account, 0, len(records))
	for _, record := range records {
		accounts = append(accounts, accountFromModel(record))
	}
	return accounts, nil
}

// SetActive 将同一 provider 下的活动账号切换到目标账号。
func (dao AccountDAO) SetActive(ctx context.Context, providerID string, accountID string, now int64) error {
	db := dao.db.WithContext(ctx)
	if err := db.Model(&model.Account{}).
		Where("provider_id = ? AND is_active = ?", providerID, true).
		Updates(map[string]any{
			"is_active":  false,
			"updated_at": now,
		}).Error; err != nil {
		return mapDatabaseError(err)
	}

	result := db.Model(&model.Account{}).
		Where("provider_id = ? AND account_id = ?", providerID, accountID).
		Updates(map[string]any{
			"is_active":    true,
			"updated_at":   now,
			"last_used_at": now,
		})
	if result.Error != nil {
		return mapDatabaseError(result.Error)
	}
	if result.RowsAffected == 0 {
		return entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return nil
}

// Delete 删除账号记录。usage_snapshots 通过外键级联删除。
func (dao AccountDAO) Delete(ctx context.Context, providerID string, accountID string) error {
	result := dao.db.WithContext(ctx).
		Where("provider_id = ? AND account_id = ?", providerID, accountID).
		Delete(&model.Account{})
	if result.Error != nil {
		return mapDatabaseError(result.Error)
	}
	if result.RowsAffected == 0 {
		return entity.NewAppError(entity.ErrorCodeNotFound)
	}
	return nil
}

func accountToModel(account entity.Account) model.Account {
	return model.Account{
		ProviderID: account.ProviderID,
		AccountID:  account.AccountID,
		StorageID:  account.StorageID,
		Label:      account.Label,
		Email:      account.Email,
		PlanType:   account.PlanType,
		IsActive:   account.IsActive,
		CreatedAt:  account.CreatedAt,
		UpdatedAt:  account.UpdatedAt,
		LastUsedAt: account.LastUsedAt,
	}
}

func accountFromModel(record model.Account) entity.Account {
	return entity.Account{
		ProviderID: record.ProviderID,
		AccountID:  record.AccountID,
		StorageID:  record.StorageID,
		Label:      record.Label,
		Email:      record.Email,
		PlanType:   record.PlanType,
		IsActive:   record.IsActive,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
		LastUsedAt: record.LastUsedAt,
	}
}

func mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	if appErr, ok := entity.AsAppError(err); ok {
		return appErr
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entity.NewAppError(entity.ErrorCodeNotFound)
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return entity.WrapAppError(entity.ErrorCodeConflict, err)
	}
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		return entity.WrapAppError(entity.ErrorCodeConflict, err)
	}
	if isSQLiteBusy(err) {
		return entity.WrapAppError(entity.ErrorCodeStorageBusy, err)
	}
	return entity.WrapAppError(entity.ErrorCodeInternal, err)
}

func isSQLiteBusy(err error) bool {
	var sqliteErr *modernsqlite.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_BUSY {
		return true
	}
	return strings.Contains(strings.ToLower(fmt.Sprint(err)), "database is locked")
}
