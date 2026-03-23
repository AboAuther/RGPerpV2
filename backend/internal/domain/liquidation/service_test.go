package liquidation

import (
	"context"
	"testing"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type liquidationFakeClock struct{ now time.Time }

func (f liquidationFakeClock) Now() time.Time { return f.now }

type liquidationFakeIDGen struct {
	values []string
	idx    int
}

func (f *liquidationFakeIDGen) NewID(_ string) string {
	value := f.values[f.idx]
	f.idx++
	return value
}

type liquidationStubTxManager struct{}

func (liquidationStubTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

type liquidationStubRepo struct {
	liquidation    Liquidation
	liquidationErr error
	positions      []Position
	orders         []RiskIncreaseOrder
	balances       CoverageBalances
	createdItems   []Item
	createdOrders  []OrderRecord
	createdFills   []FillRecord
}

func (s *liquidationStubRepo) GetLiquidationByIDForUpdate(context.Context, string) (Liquidation, error) {
	if s.liquidationErr != nil {
		return Liquidation{}, s.liquidationErr
	}
	return s.liquidation, nil
}

func (s *liquidationStubRepo) CreateLiquidation(_ context.Context, liquidation Liquidation) error {
	s.liquidation = liquidation
	s.liquidationErr = nil
	return nil
}

func (s *liquidationStubRepo) UpdateLiquidation(_ context.Context, liquidation Liquidation) error {
	s.liquidation = liquidation
	return nil
}

func (s *liquidationStubRepo) CreateItem(_ context.Context, item Item) error {
	s.createdItems = append(s.createdItems, item)
	return nil
}

func (s *liquidationStubRepo) ListOpenPositionsForUpdate(context.Context, uint64) ([]Position, error) {
	return s.positions, nil
}

func (s *liquidationStubRepo) ListRiskIncreaseOrdersForUpdate(context.Context, uint64) ([]RiskIncreaseOrder, error) {
	return s.orders, nil
}

func (s *liquidationStubRepo) CancelOrders(context.Context, []string, time.Time) error { return nil }

func (s *liquidationStubRepo) CreateOrder(_ context.Context, order OrderRecord) error {
	s.createdOrders = append(s.createdOrders, order)
	return nil
}

func (s *liquidationStubRepo) CreateFill(_ context.Context, fill FillRecord) error {
	s.createdFills = append(s.createdFills, fill)
	return nil
}

func (s *liquidationStubRepo) UpsertPosition(_ context.Context, position Position) error {
	s.positions[0] = position
	return nil
}

func (s *liquidationStubRepo) GetCoverageBalancesForUpdate(context.Context, uint64, string) (CoverageBalances, error) {
	return s.balances, nil
}

type liquidationStubAccounts struct{}

func (liquidationStubAccounts) ResolveLiquidationAccounts(context.Context, uint64, string) (Accounts, error) {
	return Accounts{
		UserWalletAccountID:         11,
		UserOrderMarginAccountID:    12,
		UserPositionMarginAccountID: 13,
		SystemPoolAccountID:         14,
		PenaltyAccountID:            15,
		InsuranceFundAccountID:      16,
	}, nil
}

type liquidationStubLedger struct {
	postings []ledgerdomain.PostingRequest
}

func (s *liquidationStubLedger) Post(_ context.Context, req ledgerdomain.PostingRequest) error {
	s.postings = append(s.postings, req)
	return nil
}

type liquidationStubRisk struct {
	snapshots []riskdomain.Snapshot
	idx       int
}

func (s *liquidationStubRisk) PreviewAccountRisk(context.Context, uint64) (riskdomain.Snapshot, error) {
	if len(s.snapshots) == 0 {
		return riskdomain.Snapshot{RiskLevel: riskdomain.RiskLevelLiquidating}, nil
	}
	value := s.snapshots[s.idx]
	if s.idx < len(s.snapshots)-1 {
		s.idx++
	}
	return value, nil
}

type liquidationStubOutbox struct {
	events []DomainEvent
}

func (s *liquidationStubOutbox) Publish(_ context.Context, event DomainEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestExecuteLiquidationConsumesWalletBeforeInsurance(t *testing.T) {
	repo := &liquidationStubRepo{
		liquidationErr: errorsx.ErrNotFound,
		balances: CoverageBalances{
			UserWalletBalance:    "20",
			InsuranceFundBalance: "100",
		},
		positions: []Position{{
			PositionID:         "pos_1",
			UserID:             7,
			SymbolID:           1,
			Symbol:             "BTC-PERP",
			Side:               "LONG",
			Qty:                "1",
			AvgEntryPrice:      "100",
			MarkPrice:          "50",
			InitialMargin:      "10",
			MaintenanceMargin:  "5",
			RealizedPnL:        "0",
			UnrealizedPnL:      "-50",
			FundingAccrual:     "0",
			LiquidationPrice:   "60",
			BankruptcyPrice:    "0",
			ContractMultiplier: "1",
			Status:             "OPEN",
			CreatedAt:          time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		}},
	}
	ledger := &liquidationStubLedger{}
	outbox := &liquidationStubOutbox{}
	risk := &liquidationStubRisk{snapshots: []riskdomain.Snapshot{
		{
			Equity:            "-40",
			AvailableBalance:  "-45",
			MaintenanceMargin: "5",
			MarginRatio:       "-8",
			RiskLevel:         riskdomain.RiskLevelLiquidating,
		},
		{
			Equity:            "-40",
			AvailableBalance:  "-45",
			MaintenanceMargin: "5",
			MarginRatio:       "-8",
			RiskLevel:         riskdomain.RiskLevelLiquidating,
		},
		{
			Equity:            "0",
			AvailableBalance:  "0",
			MaintenanceMargin: "0",
			MarginRatio:       "999",
			RiskLevel:         riskdomain.RiskLevelSafe,
		},
	}}
	service, err := NewService(ServiceConfig{
		Asset:            "USDC",
		PenaltyRate:      "0.01",
		ExtraSlippageBps: 0,
	}, liquidationFakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)}, &liquidationFakeIDGen{
		values: []string{"ldg_1", "evt_1", "ord_1", "fill_1", "evt_2"},
	}, liquidationStubTxManager{}, repo, liquidationStubAccounts{}, ledger, risk, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	liquidation, err := service.Execute(context.Background(), ExecuteInput{
		LiquidationID:         "liq_1",
		UserID:                7,
		TriggerRiskSnapshotID: 99,
		TraceID:               "trace_1",
	})
	if err != nil {
		t.Fatalf("execute liquidation: %v", err)
	}
	if liquidation.Status != StatusExecuted {
		t.Fatalf("expected executed liquidation, got %s", liquidation.Status)
	}
	if liquidation.PenaltyAmount != "0.5" {
		t.Fatalf("unexpected penalty amount: %s", liquidation.PenaltyAmount)
	}
	if liquidation.InsuranceFundUsed != "20.5" || liquidation.BankruptAmount != "0" {
		t.Fatalf("unexpected liquidation totals: %+v", liquidation)
	}
	if len(ledger.postings) != 1 {
		t.Fatalf("expected one ledger posting, got %d", len(ledger.postings))
	}
	if len(ledger.postings[0].Entries) != 5 {
		t.Fatalf("expected five ledger entries including wallet cover and insurance, got %d", len(ledger.postings[0].Entries))
	}
	if ledger.postings[0].Entries[3].AccountID != 11 || ledger.postings[0].Entries[3].Amount != "-20" {
		t.Fatalf("expected wallet cover entry, got %+v", ledger.postings[0].Entries[3])
	}
	if ledger.postings[0].Entries[4].AccountID != 16 || ledger.postings[0].Entries[4].Amount != "-20.5" {
		t.Fatalf("expected insurance entry, got %+v", ledger.postings[0].Entries[4])
	}
	if repo.positions[0].Status != "CLOSED" || repo.positions[0].Qty != "0" {
		t.Fatalf("unexpected closed position: %+v", repo.positions[0])
	}
	if repo.positions[0].Notional != "0" {
		t.Fatalf("expected closed position notional to be zero, got %+v", repo.positions[0])
	}
	if repo.positions[0].RealizedPnL != "-50" {
		t.Fatalf("expected liquidation realized pnl to be accumulated, got %+v", repo.positions[0])
	}
	if len(repo.createdItems) != 1 || len(repo.createdOrders) != 1 || len(repo.createdFills) != 1 {
		t.Fatalf("expected liquidation artifacts to be written")
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != "risk.liquidation.executed" {
		t.Fatalf("unexpected outbox events: %+v", outbox.events)
	}
	if liquidation.PreAccountSnapshot == nil || liquidation.PostAccountSnapshot == nil || liquidation.SettlementSnapshot == nil {
		t.Fatalf("expected liquidation audit snapshots, got %+v", liquidation)
	}
	if len(liquidation.PrePositionsSnapshot) != 1 || len(liquidation.PostPositionsSnapshot) != 1 || len(liquidation.PriceSnapshot) != 1 {
		t.Fatalf("expected position and price snapshots, got %+v", liquidation)
	}
	if liquidation.PostPositionsSnapshot[0].Status != "CLOSED" || liquidation.PostPositionsSnapshot[0].ExecutionPrice != "50" {
		t.Fatalf("expected closed post position snapshot, got %+v", liquidation.PostPositionsSnapshot[0])
	}
}

func TestExecuteLiquidationMarksManualRequiredWhenCoverageIsInsufficient(t *testing.T) {
	repo := &liquidationStubRepo{
		liquidationErr: errorsx.ErrNotFound,
		balances: CoverageBalances{
			UserWalletBalance:    "5",
			InsuranceFundBalance: "10",
		},
		positions: []Position{{
			PositionID:         "pos_1",
			UserID:             7,
			SymbolID:           1,
			Symbol:             "BTC-PERP",
			Side:               "LONG",
			Qty:                "1",
			AvgEntryPrice:      "100",
			MarkPrice:          "50",
			InitialMargin:      "10",
			MaintenanceMargin:  "5",
			RealizedPnL:        "0",
			UnrealizedPnL:      "-50",
			FundingAccrual:     "0",
			LiquidationPrice:   "60",
			BankruptcyPrice:    "0",
			ContractMultiplier: "1",
			Status:             "OPEN",
			CreatedAt:          time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		}},
	}
	ledger := &liquidationStubLedger{}
	outbox := &liquidationStubOutbox{}
	risk := &liquidationStubRisk{snapshots: []riskdomain.Snapshot{
		{
			Equity:            "-40",
			AvailableBalance:  "-45",
			MaintenanceMargin: "5",
			MarginRatio:       "-8",
			RiskLevel:         riskdomain.RiskLevelLiquidating,
		},
		{
			Equity:            "-40",
			AvailableBalance:  "-45",
			MaintenanceMargin: "5",
			MarginRatio:       "-8",
			RiskLevel:         riskdomain.RiskLevelLiquidating,
		},
	}}
	service, err := NewService(ServiceConfig{
		Asset:            "USDC",
		PenaltyRate:      "0.01",
		ExtraSlippageBps: 0,
	}, liquidationFakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)}, &liquidationFakeIDGen{
		values: []string{"evt_1"},
	}, liquidationStubTxManager{}, repo, liquidationStubAccounts{}, ledger, risk, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	liquidation, err := service.Execute(context.Background(), ExecuteInput{
		LiquidationID:         "liq_1",
		UserID:                7,
		TriggerRiskSnapshotID: 99,
		TraceID:               "trace_1",
	})
	if err != nil {
		t.Fatalf("execute liquidation: %v", err)
	}
	if liquidation.Status != StatusPendingManual {
		t.Fatalf("expected manual-required liquidation, got %+v", liquidation)
	}
	if liquidation.BankruptAmount != "25.5" {
		t.Fatalf("unexpected bankrupt amount: %+v", liquidation)
	}
	if len(ledger.postings) != 0 {
		t.Fatalf("expected no settlement posting when manual intervention is required")
	}
	if len(repo.createdItems) != 0 || len(repo.createdOrders) != 0 || len(repo.createdFills) != 0 {
		t.Fatalf("expected no liquidation artifacts before manual handling")
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != "risk.liquidation.manual_required" {
		t.Fatalf("unexpected outbox events: %+v", outbox.events)
	}
	if liquidation.SettlementSnapshot == nil || liquidation.PostAccountSnapshot == nil {
		t.Fatalf("expected manual liquidation snapshots, got %+v", liquidation)
	}
	if liquidation.SettlementSnapshot.BankruptAmount != "25.5" {
		t.Fatalf("unexpected settlement snapshot: %+v", liquidation.SettlementSnapshot)
	}
}

func TestExecuteLiquidationIsIdempotentForExecutedLiquidation(t *testing.T) {
	repo := &liquidationStubRepo{
		liquidation: Liquidation{
			ID:                    "liq_1",
			UserID:                7,
			Status:                StatusExecuted,
			TriggerRiskSnapshotID: 99,
			PenaltyAmount:         "1",
			InsuranceFundUsed:     "2",
			BankruptAmount:        "2",
		},
	}
	ledger := &liquidationStubLedger{}
	outbox := &liquidationStubOutbox{}
	service, err := NewService(ServiceConfig{
		Asset:            "USDC",
		PenaltyRate:      "0.01",
		ExtraSlippageBps: 0,
	}, liquidationFakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)}, &liquidationFakeIDGen{}, liquidationStubTxManager{}, repo, liquidationStubAccounts{}, ledger, &liquidationStubRisk{}, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	liquidation, err := service.Execute(context.Background(), ExecuteInput{
		LiquidationID:         "liq_1",
		UserID:                7,
		TriggerRiskSnapshotID: 99,
		TraceID:               "trace_1",
	})
	if err != nil {
		t.Fatalf("execute liquidation: %v", err)
	}
	if liquidation.Status != StatusExecuted {
		t.Fatalf("expected executed liquidation, got %+v", liquidation)
	}
	if len(ledger.postings) != 0 || len(repo.createdItems) != 0 || len(repo.createdOrders) != 0 || len(repo.createdFills) != 0 || len(outbox.events) != 0 {
		t.Fatalf("expected idempotent path to avoid side effects")
	}
}

func TestExecuteLiquidationAbortsAfterOrderReleaseWhenRiskRecovers(t *testing.T) {
	repo := &liquidationStubRepo{
		liquidationErr: errorsx.ErrNotFound,
		balances: CoverageBalances{
			UserWalletBalance:    "2",
			InsuranceFundBalance: "100",
		},
		positions: []Position{{
			PositionID:         "pos_1",
			UserID:             7,
			SymbolID:           1,
			Symbol:             "BTC-PERP",
			Side:               "LONG",
			Qty:                "1",
			AvgEntryPrice:      "100",
			MarkPrice:          "50",
			InitialMargin:      "10",
			MaintenanceMargin:  "5",
			UnrealizedPnL:      "-50",
			FundingAccrual:     "0",
			LiquidationPrice:   "60",
			ContractMultiplier: "1",
			Status:             "OPEN",
			CreatedAt:          time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		}},
		orders: []RiskIncreaseOrder{{
			OrderID:       "ord_risk_1",
			ClientOrderID: "coid_1",
			UserID:        7,
			SymbolID:      1,
			Symbol:        "BTC-PERP",
			Side:          "BUY",
			Qty:           "1",
			FrozenMargin:  "5",
			Status:        "RESTING",
		}},
	}
	ledger := &liquidationStubLedger{}
	outbox := &liquidationStubOutbox{}
	risk := &liquidationStubRisk{snapshots: []riskdomain.Snapshot{
		{
			Equity:            "-3",
			AvailableBalance:  "-8",
			MaintenanceMargin: "5",
			MarginRatio:       "-0.6",
			RiskLevel:         riskdomain.RiskLevelLiquidating,
		},
		{
			Equity:            "2",
			AvailableBalance:  "1",
			MaintenanceMargin: "5",
			MarginRatio:       "0.4",
			RiskLevel:         riskdomain.RiskLevelSafe,
		},
	}}
	service, err := NewService(ServiceConfig{
		Asset:            "USDC",
		PenaltyRate:      "0.01",
		ExtraSlippageBps: 50,
	}, liquidationFakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)}, &liquidationFakeIDGen{
		values: []string{"ldg_1", "evt_1", "evt_2"},
	}, liquidationStubTxManager{}, repo, liquidationStubAccounts{}, ledger, risk, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	liquidation, err := service.Execute(context.Background(), ExecuteInput{
		LiquidationID:         "liq_2",
		UserID:                7,
		TriggerRiskSnapshotID: 100,
		TraceID:               "trace_2",
	})
	if err != nil {
		t.Fatalf("execute liquidation: %v", err)
	}
	if liquidation.Status != StatusAborted {
		t.Fatalf("expected aborted liquidation, got %+v", liquidation)
	}
	if liquidation.AbortReason == nil || *liquidation.AbortReason != abortReasonAfterRelease {
		t.Fatalf("expected abort reason after release, got %+v", liquidation.AbortReason)
	}
	if len(ledger.postings) != 1 {
		t.Fatalf("expected only release posting, got %d", len(ledger.postings))
	}
	if liquidation.PreAccountSnapshot == nil || liquidation.PostAccountSnapshot == nil || len(liquidation.ReleasedOrders) != 1 {
		t.Fatalf("expected abort audit snapshots, got %+v", liquidation)
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != "risk.liquidation.aborted" {
		t.Fatalf("unexpected outbox events: %+v", outbox.events)
	}
}
