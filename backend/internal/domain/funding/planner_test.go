package funding

import (
	"context"
	"testing"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type fundingStubClock struct {
	now time.Time
}

func (s fundingStubClock) Now() time.Time { return s.now }

type fundingStubIDGen struct {
	value string
}

func (s fundingStubIDGen) NewID(string) string { return s.value }

type fundingStubTxManager struct{}

func (fundingStubTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

type fundingStubRates struct {
	rates []SourceRate
	err   error
}

func (s fundingStubRates) GetRates(context.Context, Symbol, time.Time, time.Time) ([]SourceRate, error) {
	return s.rates, s.err
}

type fundingPlannerStubRepo struct {
	symbols               []Symbol
	batch                 Batch
	batchErr              error
	readyBatches          []Batch
	settlement            SettlementPrice
	settlementErr         error
	positions             []PositionSnapshot
	positionErr           error
	positionByID          map[string]PositionSnapshot
	latestRateSnapshots   []RateSnapshot
	appendedRateSnapshots []RateSnapshot
	createdBatch          Batch
	createdItems          []BatchItem
	updatedBatch          Batch
	updatedItems          []BatchItem
	updatedFundingByPos   map[string]string
}

func (s *fundingPlannerStubRepo) ListSymbolsForFunding(context.Context) ([]Symbol, error) {
	return s.symbols, nil
}

func (s *fundingPlannerStubRepo) GetBatchByWindow(context.Context, uint64, time.Time, time.Time) (Batch, error) {
	return s.batch, s.batchErr
}

func (s *fundingPlannerStubRepo) ListReadyBatches(context.Context, int) ([]Batch, error) {
	return append([]Batch(nil), s.readyBatches...), nil
}

func (s *fundingPlannerStubRepo) GetBatchByIDForUpdate(context.Context, string) (Batch, error) {
	if s.batchErr != nil {
		return Batch{}, s.batchErr
	}
	return s.batch, nil
}

func (s *fundingPlannerStubRepo) GetSettlementPriceAtOrBefore(context.Context, uint64, time.Time) (SettlementPrice, error) {
	return s.settlement, s.settlementErr
}

func (s *fundingPlannerStubRepo) ListLatestRateSnapshots(context.Context, uint64, time.Time) ([]RateSnapshot, error) {
	return s.latestRateSnapshots, nil
}

func (s *fundingPlannerStubRepo) AppendRateSnapshots(_ context.Context, snapshots []RateSnapshot) error {
	s.appendedRateSnapshots = append([]RateSnapshot(nil), snapshots...)
	return nil
}

func (s *fundingPlannerStubRepo) CreateBatch(_ context.Context, batch Batch) error {
	s.createdBatch = batch
	s.batch = batch
	return nil
}

func (s *fundingPlannerStubRepo) UpdateBatch(_ context.Context, batch Batch) error {
	s.updatedBatch = batch
	s.batch = batch
	return nil
}

func (s *fundingPlannerStubRepo) CreateBatchItems(_ context.Context, items []BatchItem) error {
	s.createdItems = append([]BatchItem(nil), items...)
	return nil
}

func (s *fundingPlannerStubRepo) ListBatchItemsForUpdate(context.Context, string) ([]BatchItem, error) {
	return append([]BatchItem(nil), s.createdItems...), nil
}

func (s *fundingPlannerStubRepo) UpdateBatchItem(_ context.Context, item BatchItem) error {
	s.updatedItems = append(s.updatedItems, item)
	return nil
}

func (s *fundingPlannerStubRepo) ListOpenPositionsForUpdate(context.Context, uint64) ([]PositionSnapshot, error) {
	return s.positions, s.positionErr
}

func (s *fundingPlannerStubRepo) GetPositionByIDForUpdate(_ context.Context, positionID string) (PositionSnapshot, error) {
	if position, ok := s.positionByID[positionID]; ok {
		return position, nil
	}
	return PositionSnapshot{}, errorsx.ErrNotFound
}

func (s *fundingPlannerStubRepo) UpdatePositionFundingAccrual(_ context.Context, positionID string, fundingAccrual string, _ time.Time) error {
	if s.updatedFundingByPos == nil {
		s.updatedFundingByPos = make(map[string]string)
	}
	s.updatedFundingByPos[positionID] = fundingAccrual
	return nil
}

func TestPlannerPlanBatchForSymbolCreatesBatch(t *testing.T) {
	service, err := NewService(ServiceConfig{
		SettlementIntervalSec: 3600,
		CapRatePerHour:        "0.0075",
		MinValidSourceCount:   1,
		DefaultCryptoModel:    ModelExternalAvg,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	repo := &fundingPlannerStubRepo{
		batchErr: errorsx.ErrNotFound,
		settlement: SettlementPrice{
			SymbolID:  7,
			Price:     "50000",
			CreatedAt: time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC),
		},
		positions: []PositionSnapshot{
			{PositionID: "pos_long", UserID: 101, SymbolID: 7, Symbol: "BTC-PERP", Side: PositionSideLong, Qty: "2", ContractMultiplier: "1"},
			{PositionID: "pos_short", UserID: 102, SymbolID: 7, Symbol: "BTC-PERP", Side: PositionSideShort, Qty: "1", ContractMultiplier: "1"},
		},
	}
	planner, err := NewPlanner(
		service,
		fundingStubClock{now: time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC)},
		fundingStubIDGen{value: "fb_1"},
		fundingStubTxManager{},
		repo,
		fundingStubRates{rates: []SourceRate{{SourceName: "binance", Rate: "0.0008", IntervalSeconds: 28800}}},
	)
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}

	plan, err := planner.PlanBatchForSymbol(context.Background(), Symbol{ID: 7, Symbol: "BTC-PERP", AssetClass: "CRYPTO", Status: "TRADING"}, planner.clock.Now())
	if err != nil {
		t.Fatalf("plan batch: %v", err)
	}
	if plan == nil {
		t.Fatalf("expected plan")
	}
	if plan.Batch.TimeWindowStart != time.Date(2026, 3, 22, 9, 0, 0, 0, time.UTC) {
		t.Fatalf("unexpected window start: %s", plan.Batch.TimeWindowStart)
	}
	if plan.Batch.TimeWindowEnd != time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC) {
		t.Fatalf("unexpected window end: %s", plan.Batch.TimeWindowEnd)
	}
	if plan.Batch.NormalizedRate != "0.0001" {
		t.Fatalf("unexpected rate: %s", plan.Batch.NormalizedRate)
	}
	if repo.createdBatch.ID != "fb_1" {
		t.Fatalf("unexpected created batch id: %s", repo.createdBatch.ID)
	}
	if len(repo.createdItems) != 2 {
		t.Fatalf("expected 2 items, got %d", len(repo.createdItems))
	}
	if repo.createdItems[0].FundingFee != "-10" {
		t.Fatalf("unexpected long funding fee: %s", repo.createdItems[0].FundingFee)
	}
	if repo.createdItems[1].FundingFee != "5" {
		t.Fatalf("unexpected short funding fee: %s", repo.createdItems[1].FundingFee)
	}
}

func TestPlannerPlanBatchForSymbolSkipsExistingBatch(t *testing.T) {
	service, err := NewService(ServiceConfig{
		SettlementIntervalSec: 3600,
		CapRatePerHour:        "0.0075",
		MinValidSourceCount:   1,
		DefaultCryptoModel:    ModelExternalAvg,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	repo := &fundingPlannerStubRepo{
		batch: Batch{ID: "fb_existing"},
	}
	planner, err := NewPlanner(
		service,
		fundingStubClock{now: time.Date(2026, 3, 22, 10, 5, 0, 0, time.UTC)},
		fundingStubIDGen{value: "fb_1"},
		fundingStubTxManager{},
		repo,
		fundingStubRates{},
	)
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}

	plan, err := planner.PlanBatchForSymbol(context.Background(), Symbol{ID: 7, Symbol: "BTC-PERP"}, planner.clock.Now())
	if err != nil {
		t.Fatalf("plan batch: %v", err)
	}
	if plan != nil {
		t.Fatalf("expected nil plan for existing batch")
	}
	if repo.createdBatch.ID != "" {
		t.Fatalf("expected no batch creation, got %s", repo.createdBatch.ID)
	}
}

func TestPlannerPlanDueBatchesReturnsCreatedPlans(t *testing.T) {
	service, err := NewService(ServiceConfig{
		SettlementIntervalSec: 3600,
		CapRatePerHour:        "0.0075",
		MinValidSourceCount:   1,
		DefaultCryptoModel:    ModelExternalAvg,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	repo := &fundingPlannerStubRepo{
		symbols:  []Symbol{{ID: 7, Symbol: "BTC-PERP", AssetClass: "CRYPTO", Status: "TRADING"}},
		batchErr: errorsx.ErrNotFound,
		settlement: SettlementPrice{
			SymbolID:  7,
			Price:     "50000",
			CreatedAt: time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC),
		},
	}
	planner, err := NewPlanner(
		service,
		fundingStubClock{now: time.Date(2026, 3, 22, 10, 1, 0, 0, time.UTC)},
		fundingStubIDGen{value: "fb_1"},
		fundingStubTxManager{},
		repo,
		fundingStubRates{rates: []SourceRate{{SourceName: "binance", Rate: "0.0008", IntervalSeconds: 28800}}},
	)
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}

	plans, err := planner.PlanDueBatches(context.Background())
	if err != nil {
		t.Fatalf("plan due batches: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
}
