package db

import (
	"context"
	"errors"
	"testing"
	"time"

	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
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

	depositRepo := NewDepositRepository(db, map[int64]int{8453: 20})
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
	total, err := withdrawRepo.SumRequestedAmountByUserSince(ctx, 7, "USDC", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("sum withdraws: %v", err)
	}
	if total != "100" {
		t.Fatalf("expected total 100, got %s", total)
	}

	if _, err := accountResolver.UserWalletAccountID(ctx, 7, "USDC"); err != nil {
		t.Fatalf("resolve user wallet account: %v", err)
	}
}

func TestDepositRepository_CreateRejectsDuplicateChainTxLog(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDepositRepository(db, map[int64]int{8453: 20})
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

func TestWithdrawRepository_ReserveNextNonce(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWithdrawRepository(db)
	txManager := NewTxManager(db)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Create(ctx, walletdomain.WithdrawRequest{
		WithdrawID:     "wd_a",
		UserID:         7,
		ChainID:        8453,
		Asset:          "USDC",
		Amount:         "10",
		FeeAmount:      "1",
		ToAddress:      "0x0000000000000000000000000000000000000001",
		Status:         walletdomain.StatusApproved,
		HoldLedgerTxID: "ldg_a",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("create withdraw a: %v", err)
	}
	if err := repo.Create(ctx, walletdomain.WithdrawRequest{
		WithdrawID:     "wd_b",
		UserID:         7,
		ChainID:        8453,
		Asset:          "USDC",
		Amount:         "20",
		FeeAmount:      "1",
		ToAddress:      "0x0000000000000000000000000000000000000002",
		Status:         walletdomain.StatusApproved,
		HoldLedgerTxID: "ldg_b",
		CreatedAt:      now.Add(time.Second),
		UpdatedAt:      now.Add(time.Second),
	}); err != nil {
		t.Fatalf("create withdraw b: %v", err)
	}

	var reserved walletdomain.WithdrawRequest
	if err := txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		reserved, err = repo.ReserveNextNonce(txCtx, 8453, "0x00000000000000000000000000000000000000aa", 5)
		return err
	}); err != nil {
		t.Fatalf("reserve nonce: %v", err)
	}
	if reserved.BroadcastNonce == nil || *reserved.BroadcastNonce != 5 {
		t.Fatalf("expected reserved nonce 5, got %+v", reserved.BroadcastNonce)
	}

	blockedErr := txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := repo.ReserveNextNonce(txCtx, 8453, "0x00000000000000000000000000000000000000aa", 5)
		return err
	})
	if !errors.Is(blockedErr, errorsx.ErrConflict) {
		t.Fatalf("expected conflict while prior reserved signing is unresolved, got %v", blockedErr)
	}

	if err := txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repo.MarkBroadcasted(txCtx, "wd_a", "0xhash", reserved.BroadcastNonce)
	}); err != nil {
		t.Fatalf("mark broadcasted: %v", err)
	}
	if err := repo.UpdateStatus(ctx, "wd_a", []string{walletdomain.StatusSigning}, walletdomain.StatusBroadcasted); err != nil {
		t.Fatalf("update status broadcasted: %v", err)
	}

	if err := txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		reserved, err = repo.ReserveNextNonce(txCtx, 8453, "0x00000000000000000000000000000000000000aa", 6)
		return err
	}); err != nil {
		t.Fatalf("reserve second nonce: %v", err)
	}
	if reserved.BroadcastNonce == nil || *reserved.BroadcastNonce != 6 {
		t.Fatalf("expected reserved nonce 6, got %+v", reserved.BroadcastNonce)
	}
}

func TestBootstrapRepository_EnsureUserBootstrap(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBootstrapRepository(db)

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
}

func authUser(id uint64) authdomain.User {
	return authdomain.User{ID: id, EVMAddress: "0x0000000000000000000000000000000000000001", Status: "ACTIVE"}
}
