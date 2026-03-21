package db

import (
	"context"
	"time"

	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
	"gorm.io/gorm"
)

type BootstrapRepository struct {
	db *gorm.DB
}

func NewBootstrapRepository(db *gorm.DB) *BootstrapRepository {
	return &BootstrapRepository{db: db}
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
	return nil
}
