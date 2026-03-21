package db

import (
	"context"
	"fmt"
	"time"

	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
	"gorm.io/gorm"
)

type DepositAddressAllocator interface {
	Allocate(user authdomain.User, chainID int64, asset string) (string, error)
}

type ChainSpec struct {
	ChainID       int64
	Asset         string
	Confirmations int
}

type BootstrapRepository struct {
	db        *gorm.DB
	chains    []ChainSpec
	allocator DepositAddressAllocator
}

func NewBootstrapRepository(db *gorm.DB, chains []ChainSpec, allocator DepositAddressAllocator) *BootstrapRepository {
	return &BootstrapRepository{
		db:        db,
		chains:    chains,
		allocator: allocator,
	}
}

func (r *BootstrapRepository) EnsureSystemBootstrap(ctx context.Context) error {
	now := time.Now().UTC()
	systemAccounts := []AccountModel{
		{AccountCode: "DEPOSIT_PENDING_CONFIRM", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "WITHDRAW_IN_TRANSIT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "WITHDRAW_FEE_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "CUSTODY_HOT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "TEST_FAUCET_POOL", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
	}

	for _, account := range systemAccounts {
		if err := DB(ctx, r.db).Where("user_id IS NULL AND account_code = ? AND asset = ?", account.AccountCode, account.Asset).
			FirstOrCreate(&account).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *BootstrapRepository) EnsureUserBootstrap(ctx context.Context, user authdomain.User) error {
	now := time.Now().UTC()
	for _, code := range []string{"USER_WALLET", "USER_ORDER_MARGIN", "USER_POSITION_MARGIN", "USER_WITHDRAW_HOLD"} {
		model := AccountModel{
			UserID:      &user.ID,
			AccountCode: code,
			AccountType: "USER",
			Asset:       "USDC",
			Status:      "ACTIVE",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := DB(ctx, r.db).
			Where("user_id = ? AND account_code = ? AND asset = ?", user.ID, code, "USDC").
			FirstOrCreate(&model).Error; err != nil {
			return err
		}
	}

	for _, chain := range r.chains {
		if r.allocator == nil {
			return fmt.Errorf("deposit address allocator is required")
		}
		address, err := r.allocator.Allocate(user, chain.ChainID, chain.Asset)
		if err != nil {
			return err
		}
		model := DepositAddressModel{
			UserID:    user.ID,
			ChainID:   chain.ChainID,
			Address:   address,
			Asset:     chain.Asset,
			Status:    "ACTIVE",
			CreatedAt: now,
		}
		if err := DB(ctx, r.db).
			Where("user_id = ? AND chain_id = ? AND asset = ?", user.ID, chain.ChainID, chain.Asset).
			FirstOrCreate(&model).Error; err != nil {
			return err
		}
	}
	return nil
}
