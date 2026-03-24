package adminops

import (
	"context"
	"testing"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	liquidationdomain "github.com/xiaobao/rgperp/backend/internal/domain/liquidation"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type fakeIDGen struct {
	values []string
	idx    int
}

func (g *fakeIDGen) NewID(prefix string) string {
	if g.idx < len(g.values) {
		value := g.values[g.idx]
		g.idx++
		return value
	}
	return prefix + "_generated"
}

type fakeTxManager struct{}

func (fakeTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

type fakeAccountResolver struct{}

func (fakeAccountResolver) SystemPoolAccountID(context.Context, string) (uint64, error) {
	return 11, nil
}
func (fakeAccountResolver) CustodyHotAccountID(context.Context, string) (uint64, error) {
	return 12, nil
}
func (fakeAccountResolver) InsuranceFundAccountID(context.Context, string) (uint64, error) {
	return 13, nil
}

type fakeLedgerPoster struct {
	postings []ledgerdomain.PostingRequest
}

func (f *fakeLedgerPoster) Post(_ context.Context, req ledgerdomain.PostingRequest) error {
	f.postings = append(f.postings, req)
	return nil
}

type fakeLiquidationRepo struct {
	liquidation liquidationdomain.Liquidation
	positionErr error
	openList    []liquidationdomain.Position
	updated     []liquidationdomain.Liquidation
}

func (f *fakeLiquidationRepo) GetLiquidationByIDForUpdate(_ context.Context, _ string) (liquidationdomain.Liquidation, error) {
	return f.liquidation, nil
}
func (f *fakeLiquidationRepo) UpdateLiquidation(_ context.Context, liquidation liquidationdomain.Liquidation) error {
	f.updated = append(f.updated, liquidation)
	f.liquidation = liquidation
	return nil
}
func (f *fakeLiquidationRepo) GetPositionForLiquidationByID(context.Context, uint64, string) (liquidationdomain.Position, error) {
	if f.positionErr != nil {
		return liquidationdomain.Position{}, f.positionErr
	}
	return liquidationdomain.Position{PositionID: "pos_1", Status: "LIQUIDATING"}, nil
}
func (f *fakeLiquidationRepo) ListOpenPositionsForUpdate(context.Context, uint64) ([]liquidationdomain.Position, error) {
	return f.openList, nil
}

type fakeLiquidationExecutor struct {
	input  liquidationdomain.ExecuteInput
	result liquidationdomain.Liquidation
}

func (f *fakeLiquidationExecutor) Execute(_ context.Context, input liquidationdomain.ExecuteInput) (liquidationdomain.Liquidation, error) {
	f.input = input
	if f.result.ID == "" {
		f.result = liquidationdomain.Liquidation{
			ID:          input.LiquidationID,
			Status:      liquidationdomain.StatusExecuted,
			AbortReason: nil,
		}
	}
	return f.result, nil
}

func TestTopUpInsuranceFundPostsBalancedLedgerTransfer(t *testing.T) {
	ledger := &fakeLedgerPoster{}
	svc, err := NewService(
		fakeClock{now: time.Date(2026, 3, 24, 8, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"iftop_1", "ldg_1", "evt_1"}},
		fakeTxManager{},
		fakeAccountResolver{},
		ledger,
		&fakeLiquidationRepo{},
		&fakeLiquidationExecutor{},
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.TopUpInsuranceFund(context.Background(), InsuranceFundTopUpInput{
		OperatorID:     "0xabc",
		TraceID:        "trace_1",
		IdempotencyKey: "idem_1",
		Reason:         "seed insurance",
		Asset:          "USDC",
		Amount:         "25",
		SourceAccount:  InsuranceTopUpSourceSystemPool,
	})
	if err != nil {
		t.Fatalf("top up insurance fund: %v", err)
	}
	if result.TopUpID != "iftop_1" || result.Status != "COMMITTED" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(ledger.postings) != 1 {
		t.Fatalf("expected one posting, got %d", len(ledger.postings))
	}
	posting := ledger.postings[0]
	if posting.LedgerTx.BizType != "insurance_fund.topup" || posting.LedgerTx.IdempotencyKey != "idem_1" {
		t.Fatalf("unexpected ledger tx: %+v", posting.LedgerTx)
	}
	if len(posting.Entries) != 2 || posting.Entries[0].AccountID != 11 || posting.Entries[0].Amount != "-25" || posting.Entries[1].AccountID != 13 || posting.Entries[1].Amount != "25" {
		t.Fatalf("unexpected ledger entries: %+v", posting.Entries)
	}
}

func TestRetryPendingLiquidationReplaysExistingRecord(t *testing.T) {
	repo := &fakeLiquidationRepo{
		liquidation: liquidationdomain.Liquidation{
			ID:                    "liq_1",
			UserID:                7,
			Mode:                  liquidationdomain.ModeIsolated,
			Status:                liquidationdomain.StatusPendingManual,
			TriggerRiskSnapshotID: 99,
			PrePositionsSnapshot:  []liquidationdomain.PositionSnapshot{{PositionID: "pos_iso_1"}},
		},
	}
	executor := &fakeLiquidationExecutor{}
	svc, err := NewService(fakeClock{now: time.Now().UTC()}, &fakeIDGen{}, fakeTxManager{}, fakeAccountResolver{}, &fakeLedgerPoster{}, repo, executor)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.RetryPendingLiquidation(context.Background(), LiquidationActionInput{
		LiquidationID: "liq_1",
		OperatorID:    "0xabc",
		TraceID:       "trace_retry",
	})
	if err != nil {
		t.Fatalf("retry liquidation: %v", err)
	}
	if result.Status != liquidationdomain.StatusExecuted {
		t.Fatalf("unexpected result: %+v", result)
	}
	if executor.input.PositionID != "pos_iso_1" || executor.input.TriggerRiskSnapshotID != 99 {
		t.Fatalf("unexpected execute input: %+v", executor.input)
	}
}

func TestClosePendingLiquidationMarksStaleIsolatedRecordAborted(t *testing.T) {
	repo := &fakeLiquidationRepo{
		liquidation: liquidationdomain.Liquidation{
			ID:                   "liq_2",
			UserID:               7,
			Mode:                 liquidationdomain.ModeIsolated,
			Status:               liquidationdomain.StatusPendingManual,
			PrePositionsSnapshot: []liquidationdomain.PositionSnapshot{{PositionID: "pos_iso_1"}},
		},
		positionErr: errorsx.ErrNotFound,
	}
	svc, err := NewService(fakeClock{now: time.Date(2026, 3, 24, 9, 0, 0, 0, time.UTC)}, &fakeIDGen{}, fakeTxManager{}, fakeAccountResolver{}, &fakeLedgerPoster{}, repo, &fakeLiquidationExecutor{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.ClosePendingLiquidation(context.Background(), LiquidationActionInput{
		LiquidationID: "liq_2",
		OperatorID:    "0xabc",
		TraceID:       "trace_close",
		Reason:        "position already closed",
	})
	if err != nil {
		t.Fatalf("close liquidation: %v", err)
	}
	if result.Status != liquidationdomain.StatusAborted {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(repo.updated) != 1 || repo.updated[0].AbortReason == nil || *repo.updated[0].AbortReason == "" {
		t.Fatalf("expected abort reason to be recorded, got %+v", repo.updated)
	}
}
