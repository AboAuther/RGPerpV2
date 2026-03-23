package db

import (
	"context"
	"time"

	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
)

type ReviewReadRepository struct {
	now func() time.Time
}

func NewReviewReadRepository() *ReviewReadRepository {
	return &ReviewReadRepository{
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (r *ReviewReadRepository) ListSymbols(_ context.Context) ([]readmodel.SymbolItem, error) {
	return []readmodel.SymbolItem{
		{Symbol: "BTC-USDC", AssetClass: "CRYPTO", TickSize: "0.1", StepSize: "0.001", MinNotional: "10", Status: "TRADING"},
		{Symbol: "ETH-USDC", AssetClass: "CRYPTO", TickSize: "0.01", StepSize: "0.001", MinNotional: "10", Status: "REDUCE_ONLY"},
		{Symbol: "XAUUSD-USDC", AssetClass: "COMMODITY", TickSize: "0.1", StepSize: "0.01", MinNotional: "20", Status: "HALTED"},
	}, nil
}

func (r *ReviewReadRepository) ListTickers(_ context.Context) ([]readmodel.TickerItem, error) {
	now := r.now()
	return []readmodel.TickerItem{
		{Symbol: "BTC-USDC", IndexPrice: "84210.35", MarkPrice: "84228.64", BestBid: "84222.1", BestAsk: "84233.4", TS: now.Add(-1 * time.Second).Format(time.RFC3339)},
		{Symbol: "ETH-USDC", IndexPrice: "2038.44", MarkPrice: "2039.01", BestBid: "2038.80", BestAsk: "2039.21", TS: now.Add(-1 * time.Second).Format(time.RFC3339)},
		{Symbol: "XAUUSD-USDC", IndexPrice: "2198.60", MarkPrice: "2198.60", BestBid: "2198.30", BestAsk: "2198.90", TS: now.Add(-4 * time.Second).Format(time.RFC3339)},
	}, nil
}

func (r *ReviewReadRepository) ListOrders(_ context.Context, _ uint64) ([]readmodel.OrderItem, error) {
	price := "84200.00"
	trigger := "1985.00"
	now := r.now()
	return []readmodel.OrderItem{
		{
			OrderID: "ord_20260321001", ClientOrderID: "cli_20260321001", Symbol: "BTC-USDC", Side: "BUY",
			PositionEffect: "OPEN", Type: "LIMIT", Qty: "0.050", FilledQty: "0.020", AvgFillPrice: "84196.40",
			Price: &price, TriggerPrice: nil, ReduceOnly: false, Status: "PARTIALLY_FILLED", CreatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
		},
		{
			OrderID: "ord_20260321002", ClientOrderID: "cli_20260321002", Symbol: "ETH-USDC", Side: "SELL",
			PositionEffect: "REDUCE", Type: "STOP_MARKET", Qty: "1.200", FilledQty: "0.000", AvgFillPrice: "0",
			Price: nil, TriggerPrice: &trigger, ReduceOnly: true, Status: "TRIGGER_WAIT", CreatedAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
		},
	}, nil
}

func (r *ReviewReadRepository) ListFills(_ context.Context, _ uint64) ([]readmodel.FillItem, error) {
	now := r.now()
	return []readmodel.FillItem{
		{FillID: "fill_20260321001", OrderID: "ord_20260321001", Symbol: "BTC-USDC", Side: "BUY", Qty: "0.020", Price: "84196.40", FeeAmount: "1.68", CreatedAt: now.Add(-15 * time.Minute).Format(time.RFC3339)},
		{FillID: "fill_20260321002", OrderID: "ord_20260321003", Symbol: "ETH-USDC", Side: "SELL", Qty: "0.500", Price: "2042.10", FeeAmount: "0.82", CreatedAt: now.Add(-45 * time.Minute).Format(time.RFC3339)},
	}, nil
}

func (r *ReviewReadRepository) ListPositions(_ context.Context, _ uint64) ([]readmodel.PositionItem, error) {
	return []readmodel.PositionItem{
		{
			PositionID: "pos_20260321001", Symbol: "BTC-USDC", Side: "LONG", Qty: "0.080", AvgEntryPrice: "83980.20",
			MarkPrice: "84228.64", InitialMargin: "1450.00", MaintenanceMargin: "870.00", RealizedPnL: "0",
			UnrealizedPnL: "19.87", FundingAccrual: "-4.12", LiquidationPrice: "78144.55", Status: "OPEN",
		},
		{
			PositionID: "pos_20260321002", Symbol: "ETH-USDC", Side: "SHORT", Qty: "1.200", AvgEntryPrice: "2044.40",
			MarkPrice: "2039.01", InitialMargin: "880.00", MaintenanceMargin: "520.00", RealizedPnL: "14.21",
			UnrealizedPnL: "6.47", FundingAccrual: "1.87", LiquidationPrice: "2278.30", Status: "OPEN",
		},
	}, nil
}

func (r *ReviewReadRepository) ListFunding(_ context.Context, _ uint64) ([]readmodel.FundingItem, error) {
	now := r.now()
	return []readmodel.FundingItem{
		{FundingID: "fund_20260321001", Symbol: "BTC-USDC", Direction: "PAY", Rate: "0.000100", Amount: "-4.12", SettledAt: now.Add(-8 * time.Hour).Format(time.RFC3339), BatchID: "funding_batch_2026032108"},
		{FundingID: "fund_20260321002", Symbol: "ETH-USDC", Direction: "RECEIVE", Rate: "-0.000050", Amount: "1.87", SettledAt: now.Add(-16 * time.Hour).Format(time.RFC3339), BatchID: "funding_batch_2026032100"},
	}, nil
}
