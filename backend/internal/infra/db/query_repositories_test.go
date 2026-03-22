package db

import (
	"context"
	"testing"
	"time"

	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
)

type fakeDepositAddressAllocator struct {
	address string
	valid   bool
	err     error
}

func (f fakeDepositAddressAllocator) Allocate(_ context.Context, _ uint64, _ int64, _ string) (string, error) {
	return f.address, f.err
}

func (f fakeDepositAddressAllocator) Validate(_ context.Context, _ uint64, _ int64, _ string, _ string) (string, bool, error) {
	return f.address, f.valid, f.err
}

func TestExplorerQueryRepository_ListEventsScopesNonAdminUsers(t *testing.T) {
	db := setupTestDB(t)
	repo := NewExplorerQueryRepository(db)
	now := time.Now().UTC()

	ledgerTxs := []LedgerTxModel{
		{LedgerTxID: "ldg_dep_user", EventID: "evt_dep_user", BizType: "DEPOSIT", BizRefID: "dep_user", Asset: "USDC", IdempotencyKey: "idem_dep_user", OperatorType: "system", OperatorID: "indexer", TraceID: "trace_1", Status: "COMMITTED", CreatedAt: now},
		{LedgerTxID: "ldg_dep_other", EventID: "evt_dep_other", BizType: "DEPOSIT", BizRefID: "dep_other", Asset: "USDC", IdempotencyKey: "idem_dep_other", OperatorType: "system", OperatorID: "indexer", TraceID: "trace_2", Status: "COMMITTED", CreatedAt: now},
		{LedgerTxID: "ldg_wd_user", EventID: "evt_wd_user", BizType: "WITHDRAW_REFUND", BizRefID: "wd_user", Asset: "USDC", IdempotencyKey: "idem_wd_user", OperatorType: "system", OperatorID: "wallet", TraceID: "trace_3", Status: "COMMITTED", CreatedAt: now},
		{LedgerTxID: "ldg_wd_other", EventID: "evt_wd_other", BizType: "WITHDRAW_REFUND", BizRefID: "wd_other", Asset: "USDC", IdempotencyKey: "idem_wd_other", OperatorType: "system", OperatorID: "wallet", TraceID: "trace_4", Status: "COMMITTED", CreatedAt: now},
		{LedgerTxID: "ldg_trf_user", EventID: "evt_trf_user", BizType: "TRANSFER", BizRefID: "trf_user", Asset: "USDC", IdempotencyKey: "idem_trf_user", OperatorType: "user", OperatorID: "7", TraceID: "trace_5", Status: "COMMITTED", CreatedAt: now},
		{LedgerTxID: "ldg_trf_other", EventID: "evt_trf_other", BizType: "TRANSFER", BizRefID: "trf_other", Asset: "USDC", IdempotencyKey: "idem_trf_other", OperatorType: "user", OperatorID: "8", TraceID: "trace_6", Status: "COMMITTED", CreatedAt: now},
	}
	if err := db.Create(&ledgerTxs).Error; err != nil {
		t.Fatalf("seed ledger txs: %v", err)
	}

	if err := db.Create(&[]DepositChainTxModel{
		{DepositID: "dep_user", UserID: 7, ChainID: 8453, TxHash: "0xdep1", LogIndex: 1, FromAddress: "0x1", ToAddress: "0x2", TokenAddress: "0x3", Amount: "100", BlockNumber: 1, Confirmations: 20, Status: "CREDITED", CreatedAt: now, UpdatedAt: now},
		{DepositID: "dep_other", UserID: 8, ChainID: 8453, TxHash: "0xdep2", LogIndex: 1, FromAddress: "0x1", ToAddress: "0x2", TokenAddress: "0x3", Amount: "100", BlockNumber: 1, Confirmations: 20, Status: "CREDITED", CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed deposits: %v", err)
	}

	if err := db.Create(&[]WithdrawRequestModel{
		{WithdrawID: "wd_user", UserID: 7, ChainID: 8453, Asset: "USDC", Amount: "100", FeeAmount: "1", ToAddress: "0x1", Status: "REFUNDED", HoldLedgerTxID: "hold_1", CreatedAt: now, UpdatedAt: now},
		{WithdrawID: "wd_other", UserID: 8, ChainID: 8453, Asset: "USDC", Amount: "100", FeeAmount: "1", ToAddress: "0x2", Status: "REFUNDED", HoldLedgerTxID: "hold_2", CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed withdrawals: %v", err)
	}

	outbox := []OutboxEventModel{
		{EventID: "ob_dep_user", AggregateType: "ledger_tx", AggregateID: "ldg_dep_user", EventType: "deposit.user", PayloadJSON: "{\"tx_hash\":\"0xchain_dep_user\",\"router_address\":\"0xrouter_user\"}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_dep_other", AggregateType: "ledger_tx", AggregateID: "ldg_dep_other", EventType: "deposit.other", PayloadJSON: "{}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_wd_user", AggregateType: "ledger_tx", AggregateID: "ldg_wd_user", EventType: "withdraw.user", PayloadJSON: "{\"tx_hash\":\"0xchain_wd_user\",\"to_address\":\"0xwithdraw_user\"}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_wd_other", AggregateType: "ledger_tx", AggregateID: "ldg_wd_other", EventType: "withdraw.other", PayloadJSON: "{}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_trf_user", AggregateType: "ledger_tx", AggregateID: "ldg_trf_user", EventType: "transfer.user", PayloadJSON: "{}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_trf_other", AggregateType: "ledger_tx", AggregateID: "ldg_trf_other", EventType: "transfer.other", PayloadJSON: "{}", Status: "PENDING", CreatedAt: now},
	}
	if err := db.Create(&outbox).Error; err != nil {
		t.Fatalf("seed outbox: %v", err)
	}

	items, err := repo.ListEvents(context.Background(), 7, false, 100)
	if err != nil {
		t.Fatalf("list scoped explorer events: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 user-scoped events, got %d", len(items))
	}
	got := map[string]bool{}
	for _, item := range items {
		got[item.EventID] = true
	}
	if !got["ob_dep_user"] || !got["ob_wd_user"] || !got["ob_trf_user"] {
		t.Fatalf("expected only user-owned events, got %#v", got)
	}
	if got["ob_dep_other"] || got["ob_wd_other"] || got["ob_trf_other"] {
		t.Fatalf("unexpected leakage of other users' events, got %#v", got)
	}

	adminItems, err := repo.ListEvents(context.Background(), 7, true, 100)
	if err != nil {
		t.Fatalf("list admin explorer events: %v", err)
	}
	if len(adminItems) != 6 {
		t.Fatalf("expected 6 admin-visible events, got %d", len(adminItems))
	}

	var depositEvent, withdrawEvent *readmodel.ExplorerEvent
	for idx := range items {
		switch items[idx].EventID {
		case "ob_dep_user":
			depositEvent = &items[idx]
		case "ob_wd_user":
			withdrawEvent = &items[idx]
		}
	}
	if depositEvent == nil || depositEvent.ChainTxHash == nil || *depositEvent.ChainTxHash != "0xchain_dep_user" {
		t.Fatalf("expected deposit event chain tx hash to be exposed, got %+v", depositEvent)
	}
	if depositEvent.Address == nil || *depositEvent.Address != "0xrouter_user" {
		t.Fatalf("expected deposit event address to be exposed, got %+v", depositEvent)
	}
	if depositEvent.Amount == nil || *depositEvent.Amount != "100" {
		t.Fatalf("expected deposit event amount to be exposed, got %+v", depositEvent)
	}
	if depositEvent.Asset == nil || *depositEvent.Asset != "USDC" {
		t.Fatalf("expected deposit event asset to be exposed, got %+v", depositEvent)
	}
	if depositEvent.CreatedAt == "" {
		t.Fatalf("expected deposit event created_at to be exposed, got %+v", depositEvent)
	}
	if withdrawEvent == nil || withdrawEvent.ChainTxHash == nil || *withdrawEvent.ChainTxHash != "0xchain_wd_user" {
		t.Fatalf("expected withdraw event chain tx hash to be exposed, got %+v", withdrawEvent)
	}
	if withdrawEvent.Address == nil || *withdrawEvent.Address != "0xwithdraw_user" {
		t.Fatalf("expected withdraw event address to be exposed, got %+v", withdrawEvent)
	}
	if withdrawEvent.Amount == nil || *withdrawEvent.Amount != "100" {
		t.Fatalf("expected withdraw event amount to be exposed, got %+v", withdrawEvent)
	}
}

func TestWalletReadService_ListDepositAddressesFiltersInvalidRows(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDepositAddressRepository(db, map[int64]int{31337: 1})
	now := time.Now().UTC()
	if err := db.Create(&DepositAddressModel{
		UserID:    7,
		ChainID:   31337,
		Asset:     "USDC",
		Address:   "0x00000000000000000000000000000000000000cd",
		Status:    "ACTIVE",
		CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed deposit address: %v", err)
	}

	readSvc := NewWalletReadService(repo, NewWalletQueryRepository(db), fakeDepositAddressAllocator{
		address: "0x00000000000000000000000000000000000000ab",
		valid:   false,
	})
	items, err := readSvc.ListDepositAddresses(context.Background(), 7)
	if err != nil {
		t.Fatalf("list deposit addresses: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected invalid deposit address to be hidden, got %+v", items)
	}
}
