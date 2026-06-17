package dao

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// DAOs 汇总同一个事务上下文内可用的数据访问对象。
type DAOs struct {
	Accounts       AccountDAO
	UsageSnapshots UsageSnapshotDAO
}

// UnitOfWork 管理 DAO 事务边界。
type UnitOfWork struct {
	db *gorm.DB
}

// NewUnitOfWork 创建 GORM-backed unit-of-work。
func NewUnitOfWork(db *gorm.DB) UnitOfWork {
	return UnitOfWork{db: db}
}

// NewDAOs 创建非事务 DAO 集合。
func NewDAOs(db *gorm.DB) DAOs {
	return DAOs{
		Accounts:       NewAccountDAO(db),
		UsageSnapshots: NewUsageSnapshotDAO(db),
	}
}

// WithinTransaction 在同一个数据库事务中执行回调。
func (uow UnitOfWork) WithinTransaction(ctx context.Context, fn func(DAOs) error) error {
	if fn == nil {
		return fmt.Errorf("transaction callback is required")
	}

	err := uow.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(NewDAOs(tx))
	})
	if err != nil {
		return mapDatabaseError(err)
	}
	return nil
}
