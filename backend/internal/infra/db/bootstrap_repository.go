package db

import (
	"context"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/config"
	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
	marketdomain "github.com/xiaobao/rgperp/backend/internal/domain/market"
	"gorm.io/gorm"
)

var removedDefaultSymbols = []string{
	"AAPL-USDC",
	"NVDA-USDC",
}

type BootstrapRepository struct {
	db *gorm.DB
}

func NewBootstrapRepository(db *gorm.DB) *BootstrapRepository {
	return &BootstrapRepository{db: db}
}

func (r *BootstrapRepository) EnsureSystemBootstrap(ctx context.Context) error {
	now := time.Now().UTC()
	systemAccounts := []AccountModel{
		{AccountCode: "SYSTEM_POOL", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "TRADING_FEE_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "DEPOSIT_PENDING_CONFIRM", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "WITHDRAW_IN_TRANSIT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "WITHDRAW_FEE_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "FUNDING_POOL", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "PENALTY_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "INSURANCE_FUND", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "HEDGE_COLLATERAL_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "HEDGE_PNL_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "ROUNDING_DIFF_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
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

func (r *BootstrapRepository) EnsureMarketBootstrap(ctx context.Context) error {
	catalog := NewMarketCatalogRepository(r.db)
	seeds := config.DefaultMarketSymbolSeeds()
	symbols := make([]marketdomain.Symbol, 0, len(seeds))
	for _, seed := range seeds {
		mappings := make([]marketdomain.SymbolMapping, 0, 4)
		if seed.BinanceSymbol != "" {
			mappings = append(mappings, marketdomain.SymbolMapping{
				SourceName:   "binance",
				SourceSymbol: seed.BinanceSymbol,
				PriceScale:   "1",
				QtyScale:     "1",
				Status:       "ACTIVE",
			})
		}
		if seed.HyperliquidSymbol != "" {
			mappings = append(mappings, marketdomain.SymbolMapping{
				SourceName:   "hyperliquid",
				SourceSymbol: seed.HyperliquidSymbol,
				PriceScale:   "1",
				QtyScale:     "1",
				Status:       "ACTIVE",
			})
		}
		if seed.CoinbaseSymbol != "" {
			mappings = append(mappings, marketdomain.SymbolMapping{
				SourceName:   "coinbase",
				SourceSymbol: seed.CoinbaseSymbol,
				PriceScale:   "1",
				QtyScale:     "1",
				Status:       "ACTIVE",
			})
		}
		if seed.TwelveDataSymbol != "" {
			mappings = append(mappings, marketdomain.SymbolMapping{
				SourceName:   "twelvedata",
				SourceSymbol: seed.TwelveDataSymbol,
				PriceScale:   "1",
				QtyScale:     "1",
				Status:       "ACTIVE",
			})
		}
		symbols = append(symbols, marketdomain.Symbol{
			Symbol:             seed.Symbol,
			AssetClass:         seed.AssetClass,
			BaseAsset:          seed.BaseAsset,
			QuoteAsset:         seed.QuoteAsset,
			ContractMultiplier: seed.ContractMultiplier,
			TickSize:           seed.TickSize,
			StepSize:           seed.StepSize,
			MinNotional:        seed.MinNotional,
			MaxLeverage:        seed.MaxLeverage,
			Status:             seed.Status,
			SessionPolicy:      seed.SessionPolicy,
			Mappings:           mappings,
		})
	}
	if err := catalog.UpsertSymbols(ctx, symbols); err != nil {
		return err
	}
	return r.decommissionRemovedDefaultSymbols(ctx)
}

func (r *BootstrapRepository) decommissionRemovedDefaultSymbols(ctx context.Context) error {
	if len(removedDefaultSymbols) == 0 {
		return nil
	}
	if err := DB(ctx, r.db).
		Model(&SymbolModel{}).
		Where("symbol IN ?", removedDefaultSymbols).
		Update("status", "DELISTED").Error; err != nil {
		return err
	}
	return DB(ctx, r.db).
		Model(&SymbolMappingModel{}).
		Where("symbol_id IN (?)",
			DB(ctx, r.db).Model(&SymbolModel{}).Select("id").Where("symbol IN ?", removedDefaultSymbols),
		).
		Update("status", "INACTIVE").Error
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
