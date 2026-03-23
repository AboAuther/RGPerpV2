package funding

import (
	"context"
	"testing"
	"time"
)

type fundingRateClientStub struct {
	name  string
	rates map[string]SourceRateQuote
	err   error
}

func (s fundingRateClientStub) Name() string { return s.name }

func (s fundingRateClientStub) FetchRates(context.Context, []SourceRateRequest) (map[string]SourceRateQuote, error) {
	return s.rates, s.err
}

func TestRateCollectorSyncOncePersistsSnapshots(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	repo := &fundingPlannerStubRepo{
		symbols: []Symbol{{
			ID:         7,
			Symbol:     "BTC-PERP",
			AssetClass: "CRYPTO",
			Status:     "TRADING",
			Mappings: []SymbolMapping{
				{SourceName: "binance", SourceSymbol: "BTCUSDC", Status: "ACTIVE"},
				{SourceName: "hyperliquid", SourceSymbol: "BTC", Status: "ACTIVE"},
			},
		}},
	}
	collector, err := NewRateCollector(
		fundingStubClock{now: now},
		repo,
		[]RateSourceClient{
			fundingRateClientStub{name: "binance", rates: map[string]SourceRateQuote{
				"BTCUSDC": {SourceName: "binance", SourceSymbol: "BTCUSDC", Rate: "0.0008", IntervalSeconds: 28800, SourceTS: now, ReceivedTS: now},
			}},
			fundingRateClientStub{name: "hyperliquid", rates: map[string]SourceRateQuote{
				"BTC": {SourceName: "hyperliquid", SourceSymbol: "BTC", Rate: "0.0001", IntervalSeconds: 3600, SourceTS: now, ReceivedTS: now},
			}},
		},
	)
	if err != nil {
		t.Fatalf("new collector: %v", err)
	}
	if err := collector.SyncOnce(context.Background(), now); err != nil {
		t.Fatalf("sync once: %v", err)
	}
	if len(repo.appendedRateSnapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(repo.appendedRateSnapshots))
	}
}

func TestSnapshotRateProviderGetRates(t *testing.T) {
	provider := NewSnapshotRateProvider(&fundingPlannerStubRepo{
		latestRateSnapshots: []RateSnapshot{
			{SourceName: "binance", Rate: "0.0008", IntervalSeconds: 28800},
			{SourceName: "hyperliquid", Rate: "0.0001", IntervalSeconds: 3600},
		},
	})
	rates, err := provider.GetRates(context.Background(), Symbol{ID: 7}, time.Time{}, time.Now().UTC())
	if err != nil {
		t.Fatalf("get rates: %v", err)
	}
	if len(rates) != 2 {
		t.Fatalf("expected 2 rates, got %d", len(rates))
	}
	if rates[0].SourceName != "binance" || rates[1].SourceName != "hyperliquid" {
		t.Fatalf("unexpected rates: %+v", rates)
	}
}
