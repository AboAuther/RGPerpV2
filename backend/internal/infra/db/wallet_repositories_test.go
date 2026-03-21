package db

import (
	"context"
	"testing"
	"time"

	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"gorm.io/gorm"
)

func seedAccounts(t *testing.T, db *gorm.DB) {
	t.Helper()
	userID := uint64(7)
	models := []AccountModel{
		{UserID: &userID, AccountCode: "USER_WALLET", AccountType: "USER", Asset: "USDC", Status: "ACTIVE", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{UserID: &userID, AccountCode: "USER_WITHDRAW_HOLD", AccountType: "USER", Asset: "USDC", Status: "ACTIVE", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{UserID: nil, AccountCode: "DEPOSIT_PENDING_CONFIRM", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{UserID: nil, AccountCode: "WITHDRAW_IN_TRANSIT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{UserID: nil, AccountCode: "WITHDRAW_FEE_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	if err := db.Create(&models).Error; err != nil {
		t.Fatalf("seed accounts: %v", err)
	}
}

func TestWalletRepositories(t *testing.T) {
	db := setupTestDB(t)
	seedAccounts(t, db)
	ctx := context.Background()

	depositRepo := NewDepositRepository(db)
	withdrawRepo := NewWithdrawRepository(db)
	accountResolver := NewAccountResolver(db)

	if err := db.Create(&DepositChainTxModel{
		DepositID: "dep_1", UserID: 7, ChainID: 8453, TxHash: "0x1", LogIndex: 1,
		FromAddress: "0x1", ToAddress: "0x2", TokenAddress: "0x3", Amount: "100", BlockNumber: 1, Confirmations: 20,
		Status: "CREDIT_READY", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("seed deposit: %v", err)
	}

	deposit, err := depositRepo.GetByID(ctx, "dep_1")
	if err != nil || deposit.DepositID != "dep_1" {
		t.Fatalf("get deposit: %v", err)
	}

	if err := withdrawRepo.Create(ctx, walletdomain.WithdrawRequest{
		WithdrawID:     "wd_1",
		UserID:         7,
		ChainID:        8453,
		Asset:          "USDC",
		Amount:         "100",
		FeeAmount:      "1",
		ToAddress:      "0x0000000000000000000000000000000000000001",
		Status:         "HOLD",
		HoldLedgerTxID: "ldg_1",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}); err != nil {
		t.Fatalf("create withdraw: %v", err)
	}

	wd, err := withdrawRepo.GetByID(ctx, "wd_1")
	if err != nil || wd.WithdrawID != "wd_1" {
		t.Fatalf("get withdraw: %v", err)
	}

	if _, err := accountResolver.UserWalletAccountID(ctx, 7, "USDC"); err != nil {
		t.Fatalf("resolve user wallet account: %v", err)
	}
}
