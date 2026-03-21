package db

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTxManager_WithinTransactionInjectsTx(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	manager := NewTxManager(db)
	called := false
	err = manager.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		called = true
		tx := DB(txCtx, db)
		if tx == nil {
			t.Fatal("expected transactional db")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("transaction should succeed: %v", err)
	}
	if !called {
		t.Fatal("expected callback execution")
	}
}
