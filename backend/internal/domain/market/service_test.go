package market

import (
	"context"
	"testing"
	"time"
)

type catalogRepoStub struct {
	symbols []Symbol
}

func (s catalogRepoStub) UpsertSymbols(context.Context, []Symbol) error { return nil }
func (s catalogRepoStub) ListActiveSymbols(context.Context) ([]Symbol, error) {
	return s.symbols, nil
}

type snapshotRepoStub struct {
	raw    []SourcePriceSnapshot
	marks  []AggregatedPrice
	states []SymbolRuntimeState
}

func (s *snapshotRepoStub) AppendSourceSnapshots(_ context.Context, snapshots []SourcePriceSnapshot) error {
	s.raw = append(s.raw, snapshots...)
	return nil
}

func (s *snapshotRepoStub) ApplyAggregatedState(_ context.Context, snapshots []AggregatedPrice, states []SymbolRuntimeState) error {
	s.marks = append(s.marks, snapshots...)
	s.states = append(s.states, states...)
	return nil
}

type sourceClientStub struct {
	name   string
	quotes map[string]SourceQuote
}

func (s sourceClientStub) Name() string { return s.name }
func (s sourceClientStub) Fetch(_ context.Context, _ []SourceSymbolRequest) (map[string]SourceQuote, error) {
	return s.quotes, nil
}

func TestServiceSyncOnce_AggregatesWeightedIndexAndMark(t *testing.T) {
	now := time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC)
	snapshotRepo := &snapshotRepoStub{}
	service, err := NewService(
		AggregationConfig{
			MaxSourceAge:     10 * time.Second,
			MaxDeviationBps:  "50",
			MinHealthySource: 1,
			MarkClampBps:     "20",
			SourceHealth: map[string]SourceHealth{
				"binance":     {Enabled: true, Weight: "0.5"},
				"hyperliquid": {Enabled: true, Weight: "0.5"},
			},
		},
		catalogRepoStub{symbols: []Symbol{{
			ID:     1,
			Symbol: "BTC-PERP",
			Mappings: []SymbolMapping{
				{SourceName: "binance", SourceSymbol: "BTCUSDC", PriceScale: "1", QtyScale: "1", Status: "ACTIVE"},
				{SourceName: "hyperliquid", SourceSymbol: "BTC", PriceScale: "1", QtyScale: "1", Status: "ACTIVE"},
			},
		}}},
		snapshotRepo,
		[]SourceClient{
			sourceClientStub{name: "binance", quotes: map[string]SourceQuote{
				"BTCUSDC": {SourceName: "binance", SourceSymbol: "BTCUSDC", Bid: "100", Ask: "102", Last: "101", SourceTS: now, ReceivedTS: now},
			}},
			sourceClientStub{name: "hyperliquid", quotes: map[string]SourceQuote{
				"BTC": {SourceName: "hyperliquid", SourceSymbol: "BTC", Bid: "101", Ask: "101", Last: "101", SourceTS: now, ReceivedTS: now},
			}},
		},
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := service.SyncOnce(context.Background(), now); err != nil {
		t.Fatalf("sync once: %v", err)
	}
	if len(snapshotRepo.raw) != 2 {
		t.Fatalf("expected 2 raw snapshots, got %d", len(snapshotRepo.raw))
	}
	if len(snapshotRepo.marks) != 1 {
		t.Fatalf("expected 1 mark snapshot, got %d", len(snapshotRepo.marks))
	}
	if len(snapshotRepo.states) != 1 || snapshotRepo.states[0].DesiredStatus != "TRADING" {
		t.Fatalf("expected trading runtime state, got %+v", snapshotRepo.states)
	}
	got := snapshotRepo.marks[0]
	if got.IndexPrice != "101" {
		t.Fatalf("unexpected index price: %s", got.IndexPrice)
	}
	if got.MarkPrice != "101" {
		t.Fatalf("unexpected mark price: %s", got.MarkPrice)
	}
}

func TestServiceSyncOnce_RejectsWhenTwoSourcesSeverelyDiverge(t *testing.T) {
	now := time.Date(2026, 3, 22, 16, 0, 0, 0, time.UTC)
	snapshotRepo := &snapshotRepoStub{}
	service, err := NewService(
		AggregationConfig{
			MaxSourceAge:     10 * time.Second,
			MaxDeviationBps:  "50",
			MinHealthySource: 1,
			MarkClampBps:     "20",
			SourceHealth: map[string]SourceHealth{
				"binance":     {Enabled: true, Weight: "0.5"},
				"hyperliquid": {Enabled: true, Weight: "0.5"},
			},
		},
		catalogRepoStub{symbols: []Symbol{{
			ID:     1,
			Symbol: "ETH-PERP",
			Mappings: []SymbolMapping{
				{SourceName: "binance", SourceSymbol: "ETHUSDC", PriceScale: "1", QtyScale: "1", Status: "ACTIVE"},
				{SourceName: "hyperliquid", SourceSymbol: "ETH", PriceScale: "1", QtyScale: "1", Status: "ACTIVE"},
			},
		}}},
		snapshotRepo,
		[]SourceClient{
			sourceClientStub{name: "binance", quotes: map[string]SourceQuote{
				"ETHUSDC": {SourceName: "binance", SourceSymbol: "ETHUSDC", Bid: "200", Ask: "200", Last: "200", SourceTS: now, ReceivedTS: now},
			}},
			sourceClientStub{name: "hyperliquid", quotes: map[string]SourceQuote{
				"ETH": {SourceName: "hyperliquid", SourceSymbol: "ETH", Bid: "260", Ask: "260", Last: "260", SourceTS: now, ReceivedTS: now},
			}},
		},
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := service.SyncOnce(context.Background(), now); err == nil {
		t.Fatal("expected sync to fail when all sources diverge")
	}
	if len(snapshotRepo.raw) != 2 {
		t.Fatalf("expected raw source snapshots to persist even on aggregation failure, got %d", len(snapshotRepo.raw))
	}
	if len(snapshotRepo.marks) != 0 {
		t.Fatalf("expected no mark snapshot, got %d", len(snapshotRepo.marks))
	}
	if len(snapshotRepo.states) != 1 || snapshotRepo.states[0].DesiredStatus != "REDUCE_ONLY" {
		t.Fatalf("expected reduce-only runtime state, got %+v", snapshotRepo.states)
	}
}
