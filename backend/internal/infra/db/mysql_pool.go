package db

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

func ConfigureMySQLConnectionPool(gormDB *gorm.DB, maxOpen int, maxIdle int, connMaxLifetime time.Duration) error {
	if gormDB == nil {
		return fmt.Errorf("gorm db is nil")
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return err
	}
	if maxOpen > 0 {
		sqlDB.SetMaxOpenConns(maxOpen)
	}
	if maxIdle > 0 {
		sqlDB.SetMaxIdleConns(maxIdle)
	}
	if connMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(connMaxLifetime)
	}
	return nil
}
