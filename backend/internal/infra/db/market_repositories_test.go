package db

import (
	"context"
	"testing"
	"time"

	marketdomain "github.com/xiaobao/rgperp/backend/internal/domain/market"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
)

func TestMarketSnapshotRepository_ApplyAggregatedStateRefreshesPositionsAndRuntimeStatus(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMarketSnapshotRepository(db, nil)
	now := time.Now().UTC()

	if err := db.Create(&[]SymbolModel{
		{
			ID:                 1,
			Symbol:             "BTC-PERP",
			AssetClass:         "CRYPTO",
			BaseAsset:          "BTC",
			QuoteAsset:         "USDC",
			ContractMultiplier: "1",
			TickSize:           "0.1",
			StepSize:           "0.001",
			MinNotional:        "10",
			Status:             "TRADING",
			SessionPolicy:      "24_7",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 2,
			Symbol:             "ETH-PERP",
			AssetClass:         "CRYPTO",
			BaseAsset:          "ETH",
			QuoteAsset:         "USDC",
			ContractMultiplier: "1",
			TickSize:           "0.1",
			StepSize:           "0.001",
			MinNotional:        "10",
			Status:             "PAUSED",
			SessionPolicy:      "24_7",
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}).Error; err != nil {
		t.Fatalf("seed symbols: %v", err)
	}
	if err := db.Create(&[]RiskTierModel{
		{SymbolID: 1, TierLevel: 1, MaxNotional: "1000000", MaxLeverage: "20", IMR: "0.1", MMR: "0.05", LiquidationFeeRate: "0.005", CreatedAt: now},
		{SymbolID: 2, TierLevel: 1, MaxNotional: "1000000", MaxLeverage: "20", IMR: "0.1", MMR: "0.05", LiquidationFeeRate: "0.005", CreatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed risk tiers: %v", err)
	}
	if err := db.Create(&PositionModel{
		PositionID:        "pos_1",
		UserID:            7,
		SymbolID:          1,
		Side:              orderdomain.PositionSideLong,
		Qty:               "2",
		AvgEntryPrice:     "100",
		MarkPrice:         "100",
		Notional:          "200",
		InitialMargin:     "20",
		MaintenanceMargin: "10",
		RealizedPnL:       "0",
		UnrealizedPnL:     "0",
		FundingAccrual:    "0",
		LiquidationPrice:  "0",
		BankruptcyPrice:   "0",
		Status:            orderdomain.PositionStatusOpen,
		CreatedAt:         now,
		UpdatedAt:         now,
	}).Error; err != nil {
		t.Fatalf("seed position: %v", err)
	}

	err := repo.ApplyAggregatedState(context.Background(),
		[]marketdomain.AggregatedPrice{{
			SymbolID:    1,
			IndexPrice:  "110",
			MarkPrice:   "110",
			BestBid:     "109",
			BestAsk:     "111",
			CalcVersion: now.UnixMilli(),
			CreatedAt:   now,
		}},
		[]marketdomain.SymbolRuntimeState{
			{SymbolID: 1, DesiredStatus: "REDUCE_ONLY"},
			{SymbolID: 2, DesiredStatus: "TRADING"},
		},
	)
	if err != nil {
		t.Fatalf("apply aggregated state: %v", err)
	}

	var position PositionModel
	if err := db.Where("position_id = ?", "pos_1").Take(&position).Error; err != nil {
		t.Fatalf("load position: %v", err)
	}
	if position.MarkPrice != "110" || position.Notional != "220" || position.MaintenanceMargin != "11" || position.UnrealizedPnL != "20" {
		t.Fatalf("unexpected refreshed position: %+v", position)
	}

	var symbolOne SymbolModel
	if err := db.Where("id = ?", 1).Take(&symbolOne).Error; err != nil {
		t.Fatalf("load symbol 1: %v", err)
	}
	if symbolOne.Status != "REDUCE_ONLY" {
		t.Fatalf("expected symbol 1 to degrade to reduce-only, got %s", symbolOne.Status)
	}

	var symbolTwo SymbolModel
	if err := db.Where("id = ?", 2).Take(&symbolTwo).Error; err != nil {
		t.Fatalf("load symbol 2: %v", err)
	}
	if symbolTwo.Status != "PAUSED" {
		t.Fatalf("expected paused symbol to remain paused, got %s", symbolTwo.Status)
	}
}
