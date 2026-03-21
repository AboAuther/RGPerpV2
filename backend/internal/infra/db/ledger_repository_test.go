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
