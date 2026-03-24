package risk

import (
	"context"
	"errors"
	"testing"
	"time"

	hedgedomain "github.com/xiaobao/rgperp/backend/internal/domain/hedge"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type riskFakeClock struct{ now time.Time }

func (f riskFakeClock) Now() time.Time { return f.now }

type riskFakeIDGen struct {
	values []string
	idx    int
}

func (f *riskFakeIDGen) NewID(_ string) string {
	value := f.values[f.idx]
	f.idx++
	return value
}

type riskStubTxManager struct{}

func (riskStubTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

type riskStubRepo struct {
	accountState        AccountState
	latestSnapshot      Snapshot
	latestSnapshotErr   error
	createdSnapshots    []Snapshot
	liquidatingClaims   map[string]bool
	markLiquidatingErr  error
	hedgeState          HedgeState
	latestOpenHedge     hedgedomain.Intent
	latestOpenHedgeErr  error
	createdHedgeIntents []hedgedomain.Intent
}

func (s *riskStubRepo) GetAccountStateForUpdate(context.Context, uint64) (AccountState, error) {
	return s.accountState, nil
}

func (s *riskStubRepo) GetLatestRiskSnapshot(context.Context, uint64) (Snapshot, error) {
	if s.latestSnapshotErr != nil {
		return Snapshot{}, s.latestSnapshotErr
	}
	return s.latestSnapshot, nil
}

func (s *riskStubRepo) CreateRiskSnapshot(_ context.Context, snapshot Snapshot) (Snapshot, error) {
	snapshot.ID = uint64(len(s.createdSnapshots) + 1)
	s.createdSnapshots = append(s.createdSnapshots, snapshot)
	s.latestSnapshot = snapshot
	s.latestSnapshotErr = nil
	return snapshot, nil
}

func (s *riskStubRepo) MarkPositionLiquidating(_ context.Context, positionID string, _ time.Time) (bool, error) {
	if s.markLiquidatingErr != nil {
		return false, s.markLiquidatingErr
	}
	if s.liquidatingClaims == nil {
		s.liquidatingClaims = make(map[string]bool)
	}
	if s.liquidatingClaims[positionID] {
		return false, nil
	}
	s.liquidatingClaims[positionID] = true
	return true, nil
}

func (s *riskStubRepo) ListActiveSymbols(context.Context) ([]HedgeState, error) {
	return []HedgeState{s.hedgeState}, nil
}

func (s *riskStubRepo) GetHedgeStateForUpdate(context.Context, uint64) (HedgeState, error) {
	return s.hedgeState, nil
}

func (s *riskStubRepo) GetLatestOpenHedgeIntentForUpdate(context.Context, uint64) (hedgedomain.Intent, error) {
	if s.latestOpenHedgeErr != nil {
		return hedgedomain.Intent{}, s.latestOpenHedgeErr
	}
	return s.latestOpenHedge, nil
}

func (s *riskStubRepo) CreateHedgeIntent(_ context.Context, intent hedgedomain.Intent) error {
	s.createdHedgeIntents = append(s.createdHedgeIntents, intent)
	s.latestOpenHedge = intent
	s.latestOpenHedgeErr = nil
	return nil
}

type riskStubOutbox struct {
	events []DomainEvent
}

func (s *riskStubOutbox) Publish(_ context.Context, event DomainEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestRecalculateAccountRiskTriggersLiquidation(t *testing.T) {
	repo := &riskStubRepo{
		accountState: AccountState{
			UserID:                7,
			WalletBalance:         "0",
			OrderMarginBalance:    "0",
			PositionMarginBalance: "50",
			WithdrawHoldBalance:   "0",
			Positions: []PositionExposure{{
				PositionID:         "pos_1",
				SymbolID:           1,
				Symbol:             "BTC-PERP",
				Side:               "LONG",
				Qty:                "1",
				AvgEntryPrice:      "100",
				MarkPrice:          "500",
				Notional:           "500",
				InitialMargin:      "50",
				MaintenanceMargin:  "45",
				UnrealizedPnL:      "0",
				LiquidationFeeRate: "0.02",
				ContractMultiplier: "1",
			}},
		},
		latestSnapshotErr: errorsx.ErrNotFound,
	}
	outbox := &riskStubOutbox{}
	service, err := NewService(ServiceConfig{
		RiskBufferRatio:    "0",
		HedgeEnabled:       true,
		SoftThresholdRatio: "0.2",
		HardThresholdRatio: "0.4",
		TakerFeeRate:       "0.0006",
	}, riskFakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)}, &riskFakeIDGen{values: []string{"liq_1", "evt_1"}}, riskStubTxManager{}, repo, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	snapshot, trigger, err := service.RecalculateAccountRisk(context.Background(), 7, "mark_price")
	if err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if snapshot.RiskLevel != RiskLevelLiquidating {
		t.Fatalf("expected liquidating risk level, got %s", snapshot.RiskLevel)
	}
	if snapshot.MarginRatio != "0.9090909090909091" {
		t.Fatalf("unexpected margin ratio: %s", snapshot.MarginRatio)
	}
	if trigger == nil || trigger.LiquidationID != "liq_1" {
		t.Fatalf("expected liquidation trigger, got %+v", trigger)
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != "risk.liquidation.triggered" {
		t.Fatalf("expected only liquidation trigger event, got %+v", outbox.events)
	}
}

func TestRecalculateAccountRiskMarksNoNewRiskWhenAvailableBalanceTurnsNegative(t *testing.T) {
	repo := &riskStubRepo{
		accountState: AccountState{
			UserID:                9,
			WalletBalance:         "0",
			OrderMarginBalance:    "0",
			PositionMarginBalance: "20",
			WithdrawHoldBalance:   "0",
			Positions: []PositionExposure{{
				PositionID:         "pos_9",
				SymbolID:           1,
				Symbol:             "BTC-PERP",
				Side:               "LONG",
				Qty:                "1",
				AvgEntryPrice:      "100",
				MarkPrice:          "100",
				Notional:           "100",
				InitialMargin:      "20",
				MaintenanceMargin:  "10",
				UnrealizedPnL:      "0",
				LiquidationFeeRate: "0.02",
				ContractMultiplier: "1",
			}},
		},
		latestSnapshotErr: errorsx.ErrNotFound,
	}
	outbox := &riskStubOutbox{}
	service, err := NewService(ServiceConfig{
		RiskBufferRatio:    "0.2",
		HedgeEnabled:       true,
		SoftThresholdRatio: "0.2",
		HardThresholdRatio: "0.4",
		TakerFeeRate:       "0.0006",
	}, riskFakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)}, &riskFakeIDGen{values: []string{"evt_1"}}, riskStubTxManager{}, repo, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	snapshot, trigger, err := service.RecalculateAccountRisk(context.Background(), 9, "mark_price")
	if err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if snapshot.RiskLevel != RiskLevelNoNewRisk {
		t.Fatalf("expected NO_NEW_RISK, got %s", snapshot.RiskLevel)
	}
	if snapshot.AvailableBalance != "-20" {
		t.Fatalf("unexpected available balance: %s", snapshot.AvailableBalance)
	}
	if trigger != nil {
		t.Fatalf("expected no liquidation trigger, got %+v", trigger)
	}
	if len(outbox.events) != 0 {
		t.Fatalf("unexpected outbox events: %+v", outbox.events)
	}
}

func TestRecalculateAccountRiskPublishesSnapshotEventOnlyWhenRiskLevelChanges(t *testing.T) {
	repo := &riskStubRepo{
		accountState: AccountState{
			UserID:                12,
			WalletBalance:         "0",
			OrderMarginBalance:    "0",
			PositionMarginBalance: "20",
			WithdrawHoldBalance:   "0",
			Positions: []PositionExposure{{
				PositionID:         "pos_12",
				SymbolID:           1,
				Symbol:             "BTC-PERP",
				Side:               "LONG",
				Qty:                "1",
				AvgEntryPrice:      "100",
				MarkPrice:          "100",
				Notional:           "100",
				InitialMargin:      "20",
				MaintenanceMargin:  "10",
				UnrealizedPnL:      "0",
				LiquidationFeeRate: "0.02",
				ContractMultiplier: "1",
			}},
		},
		latestSnapshot: Snapshot{
			ID:                11,
			UserID:            12,
			Equity:            "30",
			AvailableBalance:  "10",
			MaintenanceMargin: "10",
			MarginRatio:       "2.5",
			RiskLevel:         RiskLevelSafe,
			TriggeredBy:       "mark_price",
		},
	}
	outbox := &riskStubOutbox{}
	service, err := NewService(ServiceConfig{
		RiskBufferRatio:    "0.2",
		HedgeEnabled:       true,
		SoftThresholdRatio: "0.2",
		HardThresholdRatio: "0.4",
		TakerFeeRate:       "0.0006",
	}, riskFakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)}, &riskFakeIDGen{values: []string{"evt_1"}}, riskStubTxManager{}, repo, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	snapshot, trigger, err := service.RecalculateAccountRisk(context.Background(), 12, "trade_fill")
	if err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if snapshot.RiskLevel != RiskLevelNoNewRisk {
		t.Fatalf("expected NO_NEW_RISK, got %s", snapshot.RiskLevel)
	}
	if trigger != nil {
		t.Fatalf("expected no liquidation trigger, got %+v", trigger)
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != "risk.snapshot.updated" {
		t.Fatalf("unexpected outbox events: %+v", outbox.events)
	}
	payload, ok := outbox.events[0].Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %+v", outbox.events[0].Payload)
	}
	if payload["risk_level"] != RiskLevelNoNewRisk {
		t.Fatalf("expected current risk level NO_NEW_RISK, got %+v", payload["risk_level"])
	}
	if payload["previous_risk_level"] != RiskLevelSafe {
		t.Fatalf("expected previous risk level SAFE, got %+v", payload["previous_risk_level"])
	}
}

func TestRecalculateAccountRiskTriggersIsolatedLiquidationWithoutAccountLiquidating(t *testing.T) {
	repo := &riskStubRepo{
		accountState: AccountState{
			UserID:                15,
			WalletBalance:         "100",
			OrderMarginBalance:    "0",
			PositionMarginBalance: "10",
			WithdrawHoldBalance:   "0",
			Positions: []PositionExposure{{
				PositionID:         "pos_iso_1",
				SymbolID:           1,
				Symbol:             "BTC-PERP",
				Side:               orderdomain.PositionSideLong,
				MarginMode:         orderdomain.MarginModeIsolated,
				Qty:                "1",
				AvgEntryPrice:      "100",
				MarkPrice:          "59",
				Notional:           "59",
				InitialMargin:      "10",
				MaintenanceMargin:  "5",
				UnrealizedPnL:      "-41",
				LiquidationPrice:   "60",
				LiquidationFeeRate: "0.02",
				ContractMultiplier: "1",
			}},
		},
		latestSnapshot: Snapshot{
			ID:                14,
			UserID:            15,
			Equity:            "70",
			AvailableBalance:  "60",
			MaintenanceMargin: "5",
			MarginRatio:       "14",
			RiskLevel:         RiskLevelSafe,
			TriggeredBy:       "mark_price",
		},
	}
	outbox := &riskStubOutbox{}
	service, err := NewService(ServiceConfig{
		RiskBufferRatio:    "0",
		HedgeEnabled:       true,
		SoftThresholdRatio: "0.2",
		HardThresholdRatio: "0.4",
		TakerFeeRate:       "0.0006",
	}, riskFakeClock{now: time.Date(2026, 3, 22, 1, 0, 0, 0, time.UTC)}, &riskFakeIDGen{values: []string{"liq_iso_1", "evt_iso_1"}}, riskStubTxManager{}, repo, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	snapshot, trigger, err := service.RecalculateAccountRisk(context.Background(), 15, "mark_price")
	if err != nil {
		t.Fatalf("recalculate: %v", err)
	}
	if snapshot.RiskLevel != RiskLevelSafe {
		t.Fatalf("expected account to remain SAFE, got %s", snapshot.RiskLevel)
	}
	if trigger == nil {
		t.Fatal("expected isolated liquidation trigger")
	}
	if trigger.Mode != liquidationModeIsolated || trigger.PositionID != "pos_iso_1" || trigger.Symbol != "BTC-PERP" {
		t.Fatalf("unexpected trigger: %+v", trigger)
	}
	if !repo.liquidatingClaims["pos_iso_1"] {
		t.Fatalf("expected position to be claimed for isolated liquidation")
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != "risk.liquidation.triggered" {
		t.Fatalf("unexpected outbox events: %+v", outbox.events)
	}
	payload, ok := outbox.events[0].Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %+v", outbox.events[0].Payload)
	}
	if payload["mode"] != liquidationModeIsolated || payload["position_id"] != "pos_iso_1" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestEvaluateHedgeIntentCreatesSellIntentFromDrift(t *testing.T) {
	repo := &riskStubRepo{
		hedgeState: HedgeState{
			SymbolID:         1,
			Symbol:           "BTC-PERP",
			InternalLongQty:  "10",
			InternalShortQty: "0",
			ManagedLongQty:   "0",
			ManagedShortQty:  "2",
		},
		latestOpenHedgeErr: errorsx.ErrNotFound,
	}
	outbox := &riskStubOutbox{}
	service, err := NewService(ServiceConfig{
		RiskBufferRatio:    "0",
		HedgeEnabled:       true,
		SoftThresholdRatio: "0.2",
		HardThresholdRatio: "0.4",
		TakerFeeRate:       "0.0006",
	}, riskFakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)}, &riskFakeIDGen{values: []string{"hint_1", "evt_1"}}, riskStubTxManager{}, repo, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	decision, err := service.EvaluateHedgeIntent(context.Background(), 1)
	if err != nil {
		t.Fatalf("evaluate hedge: %v", err)
	}
	if decision == nil {
		t.Fatal("expected hedge decision")
	}
	if decision.Intent.Side != hedgedomain.OrderSideSell || decision.Intent.TargetQty != "8" {
		t.Fatalf("unexpected intent: %+v", decision.Intent)
	}
	if decision.BreachLevel != "HARD" {
		t.Fatalf("expected HARD breach level, got %+v", decision)
	}
	if len(repo.createdHedgeIntents) != 1 {
		t.Fatalf("expected 1 hedge intent, got %d", len(repo.createdHedgeIntents))
	}
	if len(outbox.events) != 1 || outbox.events[0].EventType != "hedge.requested" {
		t.Fatalf("unexpected outbox events: %+v", outbox.events)
	}
}

func TestEvaluateHedgeIntentCreatesIntentOnSoftThreshold(t *testing.T) {
	repo := &riskStubRepo{
		hedgeState: HedgeState{
			SymbolID:         1,
			Symbol:           "BTC-PERP",
			InternalLongQty:  "10",
			InternalShortQty: "0",
			ManagedLongQty:   "0",
			ManagedShortQty:  "8",
		},
		latestOpenHedgeErr: errorsx.ErrNotFound,
	}
	outbox := &riskStubOutbox{}
	service, err := NewService(ServiceConfig{
		RiskBufferRatio:    "0",
		HedgeEnabled:       true,
		SoftThresholdRatio: "0.2",
		HardThresholdRatio: "0.4",
		TakerFeeRate:       "0.0006",
	}, riskFakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)}, &riskFakeIDGen{values: []string{"hint_1", "evt_1"}}, riskStubTxManager{}, repo, outbox)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	decision, err := service.EvaluateHedgeIntent(context.Background(), 1)
	if err != nil {
		t.Fatalf("evaluate hedge: %v", err)
	}
	if decision == nil {
		t.Fatal("expected hedge decision")
	}
	if decision.BreachLevel != "SOFT" {
		t.Fatalf("expected SOFT breach level, got %+v", decision)
	}
	if decision.Intent.TargetQty != "2" {
		t.Fatalf("unexpected intent: %+v", decision.Intent)
	}
}

func TestPreviewAccountRiskRejectsStaleMarkPrice(t *testing.T) {
	repo := &riskStubRepo{
		accountState: AccountState{
			UserID:                11,
			WalletBalance:         "100",
			OrderMarginBalance:    "0",
			PositionMarginBalance: "20",
			WithdrawHoldBalance:   "0",
			Positions: []PositionExposure{{
				PositionID:         "pos_stale",
				SymbolID:           1,
				Symbol:             "BTC-PERP",
				Side:               "LONG",
				Qty:                "1",
				AvgEntryPrice:      "100",
				MarkPrice:          "100",
				Notional:           "100",
				InitialMargin:      "20",
				MaintenanceMargin:  "10",
				UnrealizedPnL:      "0",
				LiquidationFeeRate: "0.02",
				ContractMultiplier: "1",
				MarkPriceUpdatedAt: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
			}},
		},
	}
	service, err := NewService(ServiceConfig{
		RiskBufferRatio:             "0",
		HedgeEnabled:                true,
		SoftThresholdRatio:          "0.2",
		HardThresholdRatio:          "0.4",
		MarkPriceStaleSec:           30,
		ForceReduceOnlyOnStalePrice: true,
		TakerFeeRate:                "0.0006",
	}, riskFakeClock{now: time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC)}, &riskFakeIDGen{values: []string{"evt_1"}}, riskStubTxManager{}, repo, &riskStubOutbox{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.PreviewAccountRisk(context.Background(), 11)
	if !errors.Is(err, errorsx.ErrConflict) {
		t.Fatalf("expected stale mark conflict, got %v", err)
	}
}

func TestComputeAccountMetricsSubtractsPendingTradingFee(t *testing.T) {
	metrics := ComputeAccountMetrics(AccountState{
		UserID:                12,
		WalletBalance:         "1000",
		OrderMarginBalance:    "25.66",
		PositionMarginBalance: "0",
		WithdrawHoldBalance:   "0",
		PendingOrders: []PendingOrderExposure{{
			OrderID:             "ord_1",
			SymbolID:            1,
			Symbol:              "BTC-PERP",
			Side:                "BUY",
			Type:                "STOP_MARKET",
			Qty:                 "1",
			TriggerPrice:        stringPtr("110"),
			FrozenInitialMargin: "25",
			FrozenFee:           "0.66",
			FrozenMargin:        "25.66",
			ContractMultiplier:  "1",
			MarkPrice:           "100",
		}},
	}, ServiceConfig{
		RiskBufferRatio:    "0",
		SoftThresholdRatio: "0.2",
		HardThresholdRatio: "0.4",
		TakerFeeRate:       "0.0006",
	})

	if metrics.PendingTradingFee != "0.66" {
		t.Fatalf("expected pending trading fee 0.66, got %s", metrics.PendingTradingFee)
	}
	if metrics.InitialMargin != "25" {
		t.Fatalf("expected pending initial margin 25, got %s", metrics.InitialMargin)
	}
	if metrics.Equity != "1025" {
		t.Fatalf("expected equity 1025 after fee deduction, got %s", metrics.Equity)
	}
}

func stringPtr(value string) *string {
	return &value
}
