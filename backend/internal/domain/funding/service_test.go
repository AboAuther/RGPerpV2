package funding

import (
	"testing"
	"time"
)

func TestNormalizeRateConvertsEightHourRateToHourly(t *testing.T) {
	service, err := NewService(ServiceConfig{
		SettlementIntervalSec: 3600,
		CapRatePerHour:        "0.0075",
		MinValidSourceCount:   1,
		DefaultCryptoModel:    ModelExternalAvg,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	normalized, err := service.NormalizeRate("0.0008", 8*3600)
	if err != nil {
		t.Fatalf("normalize rate: %v", err)
	}
	if normalized != "0.0001" {
		t.Fatalf("unexpected normalized rate: %s", normalized)
	}
}

func TestBuildBatchProducesSignedItems(t *testing.T) {
	service, err := NewService(ServiceConfig{
		SettlementIntervalSec: 3600,
		CapRatePerHour:        "0.0075",
		MinValidSourceCount:   1,
		DefaultCryptoModel:    ModelExternalAvg,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	plan, err := service.BuildBatch(BuildBatchInput{
		FundingBatchID:  "fb_1",
		SymbolID:        1,
		Symbol:          "BTC-PERP",
		TimeWindowStart: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		TimeWindowEnd:   time.Date(2026, 3, 22, 1, 0, 0, 0, time.UTC),
		SettlementPrice: "100",
		Sources: []SourceRate{
			{SourceName: "binance", Rate: "0.0001", IntervalSeconds: 3600},
			{SourceName: "hyperliquid", Rate: "0.0001", IntervalSeconds: 3600},
		},
		Positions: []PositionSnapshot{
			{PositionID: "pos_long", UserID: 7, SymbolID: 1, Symbol: "BTC-PERP", Side: PositionSideLong, Qty: "2", ContractMultiplier: "1"},
			{PositionID: "pos_short", UserID: 8, SymbolID: 1, Symbol: "BTC-PERP", Side: PositionSideShort, Qty: "2", ContractMultiplier: "1"},
		},
		CreatedAt: time.Date(2026, 3, 22, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("build batch: %v", err)
	}
	if plan.Batch.Status != BatchStatusReady {
		t.Fatalf("unexpected batch status: %s", plan.Batch.Status)
	}
	if plan.Batch.NormalizedRate != "0.0001" {
		t.Fatalf("unexpected batch rate: %s", plan.Batch.NormalizedRate)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(plan.Items))
	}
	if plan.Items[0].FundingFee != "-0.02" {
		t.Fatalf("unexpected long funding fee: %s", plan.Items[0].FundingFee)
	}
	if plan.Items[1].FundingFee != "0.02" {
		t.Fatalf("unexpected short funding fee: %s", plan.Items[1].FundingFee)
	}
}

func TestAggregateRateClampsBySettlementCap(t *testing.T) {
	service, err := NewService(ServiceConfig{
		SettlementIntervalSec: 3600,
		CapRatePerHour:        "0.0075",
		MinValidSourceCount:   1,
		DefaultCryptoModel:    ModelExternalAvg,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	rate, _, err := service.AggregateRate([]SourceRate{
		{SourceName: "binance", Rate: "0.02", IntervalSeconds: 3600},
	})
	if err != nil {
		t.Fatalf("aggregate rate: %v", err)
	}
	if rate != "0.0075" {
		t.Fatalf("unexpected clamped rate: %s", rate)
	}
}
