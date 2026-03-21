package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
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

func TestDepositRepository_CreateRejectsDuplicateChainTxLog(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDepositRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()

	deposit := walletdomain.DepositChainTx{
		DepositID:    "dep_1",
		UserID:       7,
		ChainID:      8453,
		TxHash:       "0xdup",
		LogIndex:     1,
		FromAddress:  "0x1",
		ToAddress:    "0x2",
		TokenAddress: "0x3",
		Amount:       "100",
		Asset:        "USDC",
		BlockNumber:  10,
		Status:       walletdomain.StatusDetected,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := repo.Create(ctx, deposit); err != nil {
		t.Fatalf("create first deposit: %v", err)
	}
	deposit.DepositID = "dep_2"
	if err := repo.Create(ctx, deposit); err == nil {
		t.Fatalf("expected duplicate chain tx/log insert to fail")
	}

	var count int64
	if err := db.Model(&DepositChainTxModel{}).Where("chain_id = ? AND tx_hash = ? AND log_index = ?", 8453, "0xdup", 1).Count(&count).Error; err != nil {
		t.Fatalf("count duplicate deposits: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one deposit row, got %d", count)
	}
}

func TestBootstrapRepository_EnsureUserBootstrap(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBootstrapRepository(db, []ChainSpec{
		{ChainID: 1, Asset: "USDC"},
		{ChainID: 8453, Asset: "USDC"},
	}, &stubAllocator{})

	if err := repo.EnsureSystemBootstrap(context.Background()); err != nil {
		t.Fatalf("ensure system bootstrap: %v", err)
	}
	if err := repo.EnsureUserBootstrap(context.Background(), authUser(7)); err != nil {
		t.Fatalf("ensure user bootstrap: %v", err)
	}

	var count int64
	if err := db.Model(&AccountModel{}).Where("user_id = ?", 7).Count(&count).Error; err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 user accounts, got %d", count)
	}
	if err := db.Model(&DepositAddressModel{}).Where("user_id = ?", 7).Count(&count).Error; err != nil {
		t.Fatalf("count deposit addresses: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 deposit addresses, got %d", count)
	}
}

type stubAllocator struct{}

func (stubAllocator) Allocate(user authdomain.User, chainID int64, asset string) (string, error) {
	return fmt.Sprintf("0x%040x", chainID+int64(user.ID)), nil
}

func authUser(id uint64) authdomain.User {
	return authdomain.User{ID: id, EVMAddress: "0x0000000000000000000000000000000000000001", Status: "ACTIVE"}
}
