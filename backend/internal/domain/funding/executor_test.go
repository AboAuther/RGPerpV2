package funding

import (
	"context"
	"testing"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
)

type fundingExecutorIDGen struct {
	values []string
	idx    int
}

func (g *fundingExecutorIDGen) NewID(string) string {
	if g.idx >= len(g.values) {
		return "id_exhausted"
	}
	value := g.values[g.idx]
	g.idx++
	return value
}

type fundingAccountsStub struct{}

func (fundingAccountsStub) ResolveFundingAccounts(context.Context, uint64, string) (FundingAccounts, error) {
	return FundingAccounts{
		UserPositionMarginAccountID: 101,
		FundingPoolAccountID:        202,
	}, nil
}

type fundingLedgerStub struct {
	requests []ledgerdomain.PostingRequest
}

func (s *fundingLedgerStub) Post(_ context.Context, req ledgerdomain.PostingRequest) error {
	s.requests = append(s.requests, req)
	return nil
}

func TestExecutorApplyBatchSettlesLedgerAndAccrual(t *testing.T) {
	now := time.Date(2026, 3, 23, 1, 0, 0, 0, time.UTC)
	repo := &fundingPlannerStubRepo{
		batch: Batch{
			ID:              "fb_1",
			SymbolID:        7,
			Symbol:          "BTC-PERP",
			TimeWindowStart: now.Add(-time.Hour),
			TimeWindowEnd:   now,
			NormalizedRate:  "0.0001",
			SettlementPrice: "50000",
			Status:          BatchStatusReady,
			CreatedAt:       now.Add(-time.Hour),
			UpdatedAt:       now.Add(-time.Hour),
		},
		createdItems: []BatchItem{
			{FundingBatchID: "fb_1", PositionID: "pos_long", UserID: 7, FundingFee: "-10", Status: ItemStatusPending, CreatedAt: now},
			{FundingBatchID: "fb_1", PositionID: "pos_short", UserID: 8, FundingFee: "5", Status: ItemStatusPending, CreatedAt: now},
		},
		positionByID: map[string]PositionSnapshot{
			"pos_long":  {PositionID: "pos_long", UserID: 7, ExistingFundingAccrual: "1"},
			"pos_short": {PositionID: "pos_short", UserID: 8, ExistingFundingAccrual: "-2"},
		},
	}
	ledger := &fundingLedgerStub{}
	executor, err := NewExecutor(
		ExecutorConfig{Asset: "USDC"},
		fundingStubClock{now: now},
		&fundingExecutorIDGen{values: []string{"ldg_1", "evt_1", "ldg_2", "evt_2"}},
		fundingStubTxManager{},
		repo,
		fundingAccountsStub{},
		ledger,
	)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}

	result, err := executor.ApplyBatch(context.Background(), "fb_1")
	if err != nil {
		t.Fatalf("apply batch: %v", err)
	}
	if result.Batch.Status != BatchStatusApplied {
		t.Fatalf("unexpected batch status: %s", result.Batch.Status)
	}
	if len(result.UserIDs) != 2 {
		t.Fatalf("expected 2 impacted users, got %d", len(result.UserIDs))
	}
	if len(ledger.requests) != 2 {
		t.Fatalf("expected 2 ledger postings, got %d", len(ledger.requests))
	}
	if repo.updatedFundingByPos["pos_long"] != "-9" {
		t.Fatalf("unexpected long funding accrual: %s", repo.updatedFundingByPos["pos_long"])
	}
	if repo.updatedFundingByPos["pos_short"] != "3" {
		t.Fatalf("unexpected short funding accrual: %s", repo.updatedFundingByPos["pos_short"])
	}
	if repo.updatedBatch.Status != BatchStatusApplied {
		t.Fatalf("unexpected updated batch status: %s", repo.updatedBatch.Status)
	}
	if len(repo.updatedItems) != 2 {
		t.Fatalf("expected 2 updated items, got %d", len(repo.updatedItems))
	}

	payPosting := ledger.requests[0]
	if payPosting.Entries[0].AccountID != 202 || payPosting.Entries[0].Amount != "10" {
		t.Fatalf("unexpected pay funding pool entry: %+v", payPosting.Entries[0])
	}
	if payPosting.Entries[1].AccountID != 101 || payPosting.Entries[1].Amount != "-10" {
		t.Fatalf("unexpected pay user margin entry: %+v", payPosting.Entries[1])
	}
	receivePosting := ledger.requests[1]
	if receivePosting.Entries[0].Amount != "-5" || receivePosting.Entries[1].Amount != "5" {
		t.Fatalf("unexpected receive posting: %+v", receivePosting.Entries)
	}
}

func TestExecutorApplyBatchIsIdempotentForAppliedBatch(t *testing.T) {
	now := time.Date(2026, 3, 23, 1, 0, 0, 0, time.UTC)
	repo := &fundingPlannerStubRepo{
		batch: Batch{
			ID:        "fb_done",
			Status:    BatchStatusApplied,
			UpdatedAt: now,
		},
	}
	ledger := &fundingLedgerStub{}
	executor, err := NewExecutor(
		ExecutorConfig{Asset: "USDC"},
		fundingStubClock{now: now},
		&fundingExecutorIDGen{},
		fundingStubTxManager{},
		repo,
		fundingAccountsStub{},
		ledger,
	)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}

	result, err := executor.ApplyBatch(context.Background(), "fb_done")
	if err != nil {
		t.Fatalf("apply batch: %v", err)
	}
	if result.Batch.Status != BatchStatusApplied {
		t.Fatalf("unexpected batch status: %s", result.Batch.Status)
	}
	if len(ledger.requests) != 0 {
		t.Fatalf("expected no ledger postings, got %d", len(ledger.requests))
	}
}
