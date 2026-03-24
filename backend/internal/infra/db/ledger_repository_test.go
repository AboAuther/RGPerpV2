package db

import (
	"context"
	"testing"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
)

func TestLedgerRepository_CreatePosting(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLedgerRepository(db)

	err := repo.CreatePosting(context.Background(), ledgerdomain.PostingRequest{
		LedgerTx: ledgerdomain.LedgerTx{
			ID:             "ldg_1",
			EventID:        "evt_1",
			BizType:        "TRANSFER",
			BizRefID:       "trf_1",
			Asset:          "USDC",
			IdempotencyKey: "idem_1",
			OperatorType:   "user",
			OperatorID:     "1",
			TraceID:        "trace_1",
			Status:         "COMMITTED",
			CreatedAt:      time.Now().UTC(),
		},
		Entries: []ledgerdomain.LedgerEntry{
			{AccountID: 1, Asset: "USDC", Amount: "100", EntryType: "IN"},
			{AccountID: 2, Asset: "USDC", Amount: "-100", EntryType: "OUT"},
		},
	})
	if err != nil {
		t.Fatalf("create posting: %v", err)
	}

	var snapshots []AccountBalanceSnapshotModel
	if err := db.Order("account_id asc").Find(&snapshots).Error; err != nil {
		t.Fatalf("query snapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("expected two snapshots")
	}
}

func TestOutboxAndConsumptionRepositories(t *testing.T) {
	db := setupTestDB(t)
	repo := NewLedgerRepository(db)

	err := repo.CreatePosting(context.Background(), ledgerdomain.PostingRequest{
		LedgerTx: ledgerdomain.LedgerTx{
			ID:             "ldg_2",
			EventID:        "evt_2",
			BizType:        "TRANSFER",
			BizRefID:       "trf_2",
			Asset:          "USDC",
			IdempotencyKey: "idem_2",
			OperatorType:   "user",
			OperatorID:     "1",
			TraceID:        "trace_2",
			Status:         "COMMITTED",
			CreatedAt:      time.Now().UTC(),
		},
		Entries: []ledgerdomain.LedgerEntry{
			{AccountID: 1, Asset: "USDC", Amount: "10", EntryType: "IN"},
			{AccountID: 2, Asset: "USDC", Amount: "-10", EntryType: "OUT"},
		},
	})
	if err != nil {
		t.Fatalf("create posting: %v", err)
	}

	outboxRepo := NewOutboxRepository(db)
	events, err := outboxRepo.ListPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected pending outbox event")
	}

	consumptions := NewMessageConsumptionRepository(db)
	ok, err := consumptions.TryBegin(context.Background(), "wallet-consumer", "evt_2", time.Now().UTC())
	if err != nil || !ok {
		t.Fatalf("expected first consume to succeed, ok=%v err=%v", ok, err)
	}
	ok, err = consumptions.TryBegin(context.Background(), "wallet-consumer", "evt_2", time.Now().UTC())
	if err != nil {
		t.Fatalf("duplicate consume: %v", err)
	}
	if ok {
		t.Fatal("expected duplicate consume to be deduped")
	}

	filtered, err := outboxRepo.ListPendingByEventTypeForConsumer(context.Background(), "ledger.committed", "wallet-consumer", 10)
	if err != nil {
		t.Fatalf("list pending for consumer: %v", err)
	}
	if len(filtered) != 0 {
		t.Fatalf("expected consumed event to be filtered out, got %d", len(filtered))
	}
}
