package db

import (
	"context"
	"errors"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// TxManager provides a gorm-backed transaction manager.
type TxManager struct {
	db *gorm.DB
}

const maxTransactionRetries = 3

func NewTxManager(db *gorm.DB) *TxManager {
	return &TxManager{db: db}
}

func (m *TxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}
	var err error
	for attempt := 0; attempt < maxTransactionRetries; attempt++ {
		err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return fn(withTx(ctx, tx))
		})
		if err == nil || !isRetryableTransactionError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
	return err
}

func DB(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := txFromContext(ctx); ok {
		return tx
	}
	return fallback.WithContext(ctx)
}

func isRetryableTransactionError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1213 || mysqlErr.Number == 1205
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "deadlock found when trying to get lock") ||
		strings.Contains(text, "lock wait timeout exceeded") ||
		strings.Contains(text, "serialization failure")
}
