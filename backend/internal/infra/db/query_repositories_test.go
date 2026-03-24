package db

import (
	"context"
	"testing"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	chaininfra "github.com/xiaobao/rgperp/backend/internal/infra/chain"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
)

func uint64Ptr(value uint64) *uint64 {
	return &value
}

type fakeDepositAddressAllocator struct {
	address string
	valid   bool
	err     error
}

type fakeLedgerChainReader struct {
	items []chaininfra.VaultBalanceSnapshot
	err   error
}

func (f fakeLedgerChainReader) ListVaultBalances(_ context.Context, _ string) ([]chaininfra.VaultBalanceSnapshot, error) {
	return f.items, f.err
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
		{LedgerTxID: "ldg_funding_user", EventID: "evt_funding_user", BizType: "funding.settlement", BizRefID: "fb_user", Asset: "USDC", IdempotencyKey: "idem_funding_user", OperatorType: "system", OperatorID: "funding-worker", TraceID: "trace_7", Status: "COMMITTED", CreatedAt: now},
		{LedgerTxID: "ldg_funding_other", EventID: "evt_funding_other", BizType: "funding.settlement", BizRefID: "fb_other", Asset: "USDC", IdempotencyKey: "idem_funding_other", OperatorType: "system", OperatorID: "funding-worker", TraceID: "trace_8", Status: "COMMITTED", CreatedAt: now},
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
	if err := db.Create(&[]OutboxEventModel{
		{EventID: "ob_funding_user", AggregateType: "ledger_tx", AggregateID: "ldg_funding_user", EventType: "ledger.committed", PayloadJSON: "{\"ledger_tx_id\":\"ldg_funding_user\",\"biz_type\":\"funding.settlement\",\"biz_ref_id\":\"fb_user\"}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_funding_other", AggregateType: "ledger_tx", AggregateID: "ldg_funding_other", EventType: "ledger.committed", PayloadJSON: "{\"ledger_tx_id\":\"ldg_funding_other\",\"biz_type\":\"funding.settlement\",\"biz_ref_id\":\"fb_other\"}", Status: "PENDING", CreatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed funding outbox: %v", err)
	}

	if err := db.Create(&[]OrderModel{
		{OrderID: "ord_user", ClientOrderID: "cli_user", UserID: 7, SymbolID: 1, Side: "BUY", PositionEffect: "OPEN", Type: "LIMIT", TimeInForce: "GTC", Qty: "1", FilledQty: "0", AvgFillPrice: "0", ReduceOnly: false, MaxSlippageBps: 100, Status: "RESTING", FrozenMargin: "10", CreatedAt: now, UpdatedAt: now},
		{OrderID: "ord_other", ClientOrderID: "cli_other", UserID: 8, SymbolID: 1, Side: "BUY", PositionEffect: "OPEN", Type: "LIMIT", TimeInForce: "GTC", Qty: "1", FilledQty: "0", AvgFillPrice: "0", ReduceOnly: false, MaxSlippageBps: 100, Status: "RESTING", FrozenMargin: "10", CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed orders: %v", err)
	}
	if err := db.Create(&[]PositionModel{
		{PositionID: "pos_user", UserID: 7, SymbolID: 1, Side: "LONG", Qty: "1", AvgEntryPrice: "100", MarkPrice: "101", Notional: "101", InitialMargin: "10", MaintenanceMargin: "5", RealizedPnL: "0", UnrealizedPnL: "1", FundingAccrual: "0", LiquidationPrice: "0", BankruptcyPrice: "0", Status: "OPEN", CreatedAt: now, UpdatedAt: now},
		{PositionID: "pos_other", UserID: 8, SymbolID: 1, Side: "LONG", Qty: "1", AvgEntryPrice: "100", MarkPrice: "101", Notional: "101", InitialMargin: "10", MaintenanceMargin: "5", RealizedPnL: "0", UnrealizedPnL: "1", FundingAccrual: "0", LiquidationPrice: "0", BankruptcyPrice: "0", Status: "OPEN", CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed positions: %v", err)
	}
	if err := db.Create(&[]FillModel{
		{FillID: "fill_user", OrderID: "ord_user", UserID: 7, SymbolID: 1, Side: "BUY", Qty: "1", Price: "101", FeeAmount: "0.1", LedgerTxID: "ldg_fill_user", CreatedAt: now},
		{FillID: "fill_other", OrderID: "ord_other", UserID: 8, SymbolID: 1, Side: "BUY", Qty: "1", Price: "101", FeeAmount: "0.1", LedgerTxID: "ldg_fill_other", CreatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed fills: %v", err)
	}
	if err := db.Create(&[]OutboxEventModel{
		{EventID: "ob_trade_order_user", AggregateType: "order", AggregateID: "ord_user", EventType: "trade.order.accepted", PayloadJSON: "{\"order_id\":\"ord_user\",\"asset\":\"USDC\",\"symbol\":\"BTC-PERP\",\"status\":\"RESTING\"}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_trade_order_other", AggregateType: "order", AggregateID: "ord_other", EventType: "trade.order.accepted", PayloadJSON: "{\"order_id\":\"ord_other\",\"asset\":\"USDC\",\"symbol\":\"BTC-PERP\",\"status\":\"RESTING\"}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_trade_fill_user", AggregateType: "fill", AggregateID: "fill_user", EventType: "trade.fill.created", PayloadJSON: "{\"fill_id\":\"fill_user\",\"order_id\":\"ord_user\",\"position_id\":\"pos_user\",\"ledger_tx_id\":\"ldg_fill_user\",\"asset\":\"USDC\",\"symbol\":\"BTC-PERP\",\"fee_amount\":\"0.1\"}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_trade_fill_other", AggregateType: "fill", AggregateID: "fill_other", EventType: "trade.fill.created", PayloadJSON: "{\"fill_id\":\"fill_other\",\"order_id\":\"ord_other\",\"position_id\":\"pos_other\",\"ledger_tx_id\":\"ldg_fill_other\",\"asset\":\"USDC\",\"symbol\":\"BTC-PERP\",\"fee_amount\":\"0.1\"}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_trade_pos_user", AggregateType: "position", AggregateID: "pos_user", EventType: "trade.position.updated", PayloadJSON: "{\"order_id\":\"ord_user\",\"fill_id\":\"fill_user\",\"position_id\":\"pos_user\",\"asset\":\"USDC\",\"symbol\":\"BTC-PERP\"}", Status: "PENDING", CreatedAt: now},
		{EventID: "ob_trade_pos_other", AggregateType: "position", AggregateID: "pos_other", EventType: "trade.position.updated", PayloadJSON: "{\"order_id\":\"ord_other\",\"fill_id\":\"fill_other\",\"position_id\":\"pos_other\",\"asset\":\"USDC\",\"symbol\":\"BTC-PERP\"}", Status: "PENDING", CreatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed trade outbox: %v", err)
	}
	if err := db.Create(&OutboxEventModel{
		EventID:       "ob_risk_user",
		AggregateType: "risk_snapshot",
		AggregateID:   "99",
		EventType:     "risk.snapshot.updated",
		PayloadJSON:   "{\"risk_snapshot_id\":99,\"user_id\":7,\"risk_level\":\"NO_NEW_RISK\",\"previous_risk_level\":\"SAFE\",\"triggered_by\":\"mark_price\"}",
		Status:        "PENDING",
		CreatedAt:     now,
	}).Error; err != nil {
		t.Fatalf("seed risk outbox: %v", err)
	}
	if err := db.Create(&[]LedgerEntryModel{
		{LedgerTxID: "ldg_funding_user", AccountID: 1, UserID: uint64Ptr(7), Asset: "USDC", Amount: "-4.12", EntryType: "FUNDING_SETTLEMENT", CreatedAt: now},
		{LedgerTxID: "ldg_funding_user", AccountID: 2, Asset: "USDC", Amount: "4.12", EntryType: "FUNDING_SETTLEMENT", CreatedAt: now},
		{LedgerTxID: "ldg_funding_other", AccountID: 3, UserID: uint64Ptr(8), Asset: "USDC", Amount: "1.87", EntryType: "FUNDING_SETTLEMENT", CreatedAt: now},
		{LedgerTxID: "ldg_funding_other", AccountID: 4, Asset: "USDC", Amount: "-1.87", EntryType: "FUNDING_SETTLEMENT", CreatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed funding ledger entries: %v", err)
	}

	items, err := repo.ListEvents(context.Background(), 7, false, 100)
	if err != nil {
		t.Fatalf("list scoped explorer events: %v", err)
	}
	if len(items) != 7 {
		t.Fatalf("expected 7 user-scoped events, got %d", len(items))
	}
	got := map[string]bool{}
	for _, item := range items {
		got[item.EventID] = true
	}
	if !got["ob_dep_user"] || !got["ob_wd_user"] || !got["ob_trf_user"] || !got["ob_trade_order_user"] || !got["ob_trade_fill_user"] || !got["ob_trade_pos_user"] || !got["ob_funding_user"] {
		t.Fatalf("expected only user-owned events, got %#v", got)
	}
	if got["ob_dep_other"] || got["ob_wd_other"] || got["ob_trf_other"] || got["ob_trade_order_other"] || got["ob_trade_fill_other"] || got["ob_trade_pos_other"] || got["ob_funding_other"] {
		t.Fatalf("unexpected leakage of other users' events, got %#v", got)
	}
	if got["ob_risk_user"] {
		t.Fatalf("expected risk snapshot event to remain admin-only, got %#v", got)
	}

	adminItems, err := repo.ListEvents(context.Background(), 7, true, 100)
	if err != nil {
		t.Fatalf("list admin explorer events: %v", err)
	}
	if len(adminItems) != 15 {
		t.Fatalf("expected 15 admin-visible events, got %d", len(adminItems))
	}
	adminGot := map[string]bool{}
	for _, item := range adminItems {
		adminGot[item.EventID] = true
	}
	if !adminGot["ob_risk_user"] {
		t.Fatalf("expected admin explorer to include risk snapshot event, got %#v", adminGot)
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

	var tradeOrderEvent, tradeFillEvent, tradePositionEvent, fundingEvent *readmodel.ExplorerEvent
	for idx := range items {
		switch items[idx].EventID {
		case "ob_trade_order_user":
			tradeOrderEvent = &items[idx]
		case "ob_trade_fill_user":
			tradeFillEvent = &items[idx]
		case "ob_trade_pos_user":
			tradePositionEvent = &items[idx]
		case "ob_funding_user":
			fundingEvent = &items[idx]
		}
	}
	if tradeOrderEvent == nil || tradeOrderEvent.OrderID == nil || *tradeOrderEvent.OrderID != "ord_user" {
		t.Fatalf("expected trade order event order_id to be exposed, got %+v", tradeOrderEvent)
	}
	if tradeFillEvent == nil || tradeFillEvent.FillID == nil || *tradeFillEvent.FillID != "fill_user" {
		t.Fatalf("expected trade fill event fill_id to be exposed, got %+v", tradeFillEvent)
	}
	if tradeFillEvent.OrderID == nil || *tradeFillEvent.OrderID != "ord_user" {
		t.Fatalf("expected trade fill event order_id to be exposed, got %+v", tradeFillEvent)
	}
	if tradeFillEvent.LedgerTxID == nil || *tradeFillEvent.LedgerTxID != "ldg_fill_user" {
		t.Fatalf("expected trade fill event ledger_tx_id to be exposed, got %+v", tradeFillEvent)
	}
	if tradeFillEvent.Amount == nil || *tradeFillEvent.Amount != "0.1" {
		t.Fatalf("expected trade fill event fee amount to be exposed, got %+v", tradeFillEvent)
	}
	if tradePositionEvent == nil || tradePositionEvent.PositionID == nil || *tradePositionEvent.PositionID != "pos_user" {
		t.Fatalf("expected trade position event position_id to be exposed, got %+v", tradePositionEvent)
	}
	if fundingEvent == nil || fundingEvent.LedgerTxID == nil || *fundingEvent.LedgerTxID != "ldg_funding_user" {
		t.Fatalf("expected funding event ledger_tx_id to be exposed, got %+v", fundingEvent)
	}
	if fundingEvent.Amount == nil || *fundingEvent.Amount != "-4.12" {
		t.Fatalf("expected funding event amount to be exposed, got %+v", fundingEvent)
	}
}

func TestAccountQueryRepository_ListTransfersIncludesReceiver(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAccountQueryRepository(db)
	ledgerRepo := NewLedgerRepository(db)
	now := time.Now().UTC()
	userSender := uint64(7)
	userReceiver := uint64(8)

	if err := db.Create(&[]UserModel{
		{ID: userSender, EVMAddress: "0x0000000000000000000000000000000000000007", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: userReceiver, EVMAddress: "0x0000000000000000000000000000000000000008", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}

	accounts := []AccountModel{
		{UserID: &userSender, AccountCode: "USER_WALLET", AccountType: "USER", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{UserID: &userReceiver, AccountCode: "USER_WALLET", AccountType: "USER", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&accounts).Error; err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	var senderWallet AccountModel
	if err := db.Where("user_id = ? AND account_code = ? AND asset = ?", userSender, "USER_WALLET", "USDC").First(&senderWallet).Error; err != nil {
		t.Fatalf("load sender wallet: %v", err)
	}
	var receiverWallet AccountModel
	if err := db.Where("user_id = ? AND account_code = ? AND asset = ?", userReceiver, "USER_WALLET", "USDC").First(&receiverWallet).Error; err != nil {
		t.Fatalf("load receiver wallet: %v", err)
	}

	if err := ledgerRepo.CreatePosting(context.Background(), ledgerdomain.PostingRequest{
		LedgerTx: ledgerdomain.LedgerTx{
			ID:             "ldg_transfer_both",
			EventID:        "evt_transfer_both",
			BizType:        "TRANSFER",
			BizRefID:       "trf_both",
			Asset:          "USDC",
			IdempotencyKey: "transfer:7:test_receiver",
			OperatorType:   "user",
			OperatorID:     "7",
			TraceID:        "trace_transfer_both",
			Status:         "COMMITTED",
			CreatedAt:      now,
		},
		Entries: []ledgerdomain.LedgerEntry{
			{AccountID: senderWallet.ID, UserID: &userSender, Asset: "USDC", Amount: "-12.5", EntryType: "TRANSFER_OUT"},
			{AccountID: receiverWallet.ID, UserID: &userReceiver, Asset: "USDC", Amount: "12.5", EntryType: "TRANSFER_IN"},
		},
	}); err != nil {
		t.Fatalf("create transfer posting: %v", err)
	}

	senderItems, err := repo.ListTransfers(context.Background(), userSender)
	if err != nil {
		t.Fatalf("list sender transfers: %v", err)
	}
	if len(senderItems) != 1 || senderItems[0].TransferID != "trf_both" {
		t.Fatalf("unexpected sender transfers: %+v", senderItems)
	}
	if senderItems[0].Direction != "OUT" || senderItems[0].CounterpartyAddress != "0x0000000000000000000000000000000000000008" {
		t.Fatalf("unexpected sender transfer details: %+v", senderItems[0])
	}

	receiverItems, err := repo.ListTransfers(context.Background(), userReceiver)
	if err != nil {
		t.Fatalf("list receiver transfers: %v", err)
	}
	if len(receiverItems) != 1 || receiverItems[0].TransferID != "trf_both" {
		t.Fatalf("unexpected receiver transfers: %+v", receiverItems)
	}
	if receiverItems[0].Amount != "12.5" {
		t.Fatalf("expected receiver amount 12.5, got %+v", receiverItems[0])
	}
	if receiverItems[0].Direction != "IN" || receiverItems[0].CounterpartyAddress != "0x0000000000000000000000000000000000000007" {
		t.Fatalf("unexpected receiver transfer details: %+v", receiverItems[0])
	}
}

func TestAccountQueryRepository_ListTransfersFallsBackToAccountUserWhenEntryUserMissing(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAccountQueryRepository(db)
	ledgerRepo := NewLedgerRepository(db)
	now := time.Now().UTC()
	userSender := uint64(17)
	userReceiver := uint64(18)

	if err := db.Create(&[]UserModel{
		{ID: userSender, EVMAddress: "0x0000000000000000000000000000000000000017", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{ID: userReceiver, EVMAddress: "0x0000000000000000000000000000000000000018", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}

	accounts := []AccountModel{
		{UserID: &userSender, AccountCode: "USER_WALLET", AccountType: "USER", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{UserID: &userReceiver, AccountCode: "USER_WALLET", AccountType: "USER", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&accounts).Error; err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	var senderWallet AccountModel
	if err := db.Where("user_id = ? AND account_code = ? AND asset = ?", userSender, "USER_WALLET", "USDC").First(&senderWallet).Error; err != nil {
		t.Fatalf("load sender wallet: %v", err)
	}
	var receiverWallet AccountModel
	if err := db.Where("user_id = ? AND account_code = ? AND asset = ?", userReceiver, "USER_WALLET", "USDC").First(&receiverWallet).Error; err != nil {
		t.Fatalf("load receiver wallet: %v", err)
	}

	if err := ledgerRepo.CreatePosting(context.Background(), ledgerdomain.PostingRequest{
		LedgerTx: ledgerdomain.LedgerTx{
			ID:             "ldg_transfer_fallback",
			EventID:        "evt_transfer_fallback",
			BizType:        "TRANSFER",
			BizRefID:       "trf_fallback",
			Asset:          "USDC",
			IdempotencyKey: "transfer:17:test_fallback",
			OperatorType:   "user",
			OperatorID:     "17",
			TraceID:        "trace_transfer_fallback",
			Status:         "COMMITTED",
			CreatedAt:      now,
		},
		Entries: []ledgerdomain.LedgerEntry{
			{AccountID: senderWallet.ID, Asset: "USDC", Amount: "-3.5", EntryType: "TRANSFER_OUT"},
			{AccountID: receiverWallet.ID, Asset: "USDC", Amount: "3.5", EntryType: "TRANSFER_IN"},
		},
	}); err != nil {
		t.Fatalf("create transfer posting: %v", err)
	}

	senderItems, err := repo.ListTransfers(context.Background(), userSender)
	if err != nil {
		t.Fatalf("list sender transfers: %v", err)
	}
	if len(senderItems) != 1 || senderItems[0].Direction != "OUT" || senderItems[0].CounterpartyAddress != "0x0000000000000000000000000000000000000018" {
		t.Fatalf("unexpected sender fallback transfer details: %+v", senderItems)
	}

	receiverItems, err := repo.ListTransfers(context.Background(), userReceiver)
	if err != nil {
		t.Fatalf("list receiver transfers: %v", err)
	}
	if len(receiverItems) != 1 || receiverItems[0].Direction != "IN" || receiverItems[0].CounterpartyAddress != "0x0000000000000000000000000000000000000017" {
		t.Fatalf("unexpected receiver fallback transfer details: %+v", receiverItems)
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

	readSvc := NewWalletReadService(repo, NewWalletQueryRepository(db, nil), fakeDepositAddressAllocator{
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

func TestDepositAddressRepository_AllowsSameAddressAcrossChains(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDepositAddressRepository(db, map[int64]int{31337: 1, 31338: 2})
	ctx := context.Background()
	now := time.Now().UTC()
	sharedAddress := "0x00000000000000000000000000000000000000ef"

	if err := repo.Upsert(ctx, walletdomain.DepositAddress{
		UserID:    7,
		ChainID:   31337,
		Asset:     "USDC",
		Address:   sharedAddress,
		Status:    "ACTIVE",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("upsert first chain address: %v", err)
	}
	if err := repo.Upsert(ctx, walletdomain.DepositAddress{
		UserID:    7,
		ChainID:   31338,
		Asset:     "USDC",
		Address:   sharedAddress,
		Status:    "ACTIVE",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("upsert second chain address: %v", err)
	}

	first, err := repo.GetByChainAddress(ctx, 31337, sharedAddress)
	if err != nil {
		t.Fatalf("get first chain address: %v", err)
	}
	second, err := repo.GetByChainAddress(ctx, 31338, sharedAddress)
	if err != nil {
		t.Fatalf("get second chain address: %v", err)
	}
	if first.ChainID != 31337 || second.ChainID != 31338 {
		t.Fatalf("expected chain-specific address lookup, got first=%+v second=%+v", first, second)
	}
}

func TestDepositAddressRepository_AssignToUserCreatesMissingRow(t *testing.T) {
	db := setupTestDB(t)
	repo := NewDepositAddressRepository(db, map[int64]int{31339: 2})
	ctx := context.Background()
	address := "0x00000000000000000000000000000000000000aa"

	if err := repo.AssignToUser(ctx, 9, 31339, "USDC", address); err != nil {
		t.Fatalf("assign to user: %v", err)
	}

	item, err := repo.GetByChainAddress(ctx, 31339, address)
	if err != nil {
		t.Fatalf("get assigned address: %v", err)
	}
	if item.UserID != 9 || item.ChainID != 31339 || item.Asset != "USDC" {
		t.Fatalf("unexpected assigned address: %+v", item)
	}
}

func TestWalletQueryRepository_GetRiskMonitorDashboard(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&[]SymbolModel{
		{Symbol: "BTC-PERP", AssetClass: "CRYPTO", BaseAsset: "BTC", QuoteAsset: "USDC", ContractMultiplier: "1", TickSize: "0.1", StepSize: "0.001", MinNotional: "10", Status: "TRADING"},
		{Symbol: "ETH-PERP", AssetClass: "CRYPTO", BaseAsset: "ETH", QuoteAsset: "USDC", ContractMultiplier: "1", TickSize: "0.1", StepSize: "0.001", MinNotional: "10", Status: "TRADING"},
	}).Error; err != nil {
		t.Fatalf("seed symbols: %v", err)
	}
	if err := db.Create(&[]MarkPriceSnapshotModel{
		{SymbolID: 1, MarkPrice: "100", CreatedAt: now},
		{SymbolID: 2, MarkPrice: "50", CreatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed mark prices: %v", err)
	}
	if err := db.Create(&[]PositionModel{
		{PositionID: "pos_long", UserID: 7, SymbolID: 1, Side: "LONG", Qty: "2.6", AvgEntryPrice: "100", MarkPrice: "100", Notional: "260", InitialMargin: "26", MaintenanceMargin: "13", RealizedPnL: "0", UnrealizedPnL: "0", FundingAccrual: "0", LiquidationPrice: "0", BankruptcyPrice: "0", Status: "OPEN", CreatedAt: now, UpdatedAt: now},
		{PositionID: "pos_short", UserID: 8, SymbolID: 1, Side: "SHORT", Qty: "0.25", AvgEntryPrice: "100", MarkPrice: "100", Notional: "25", InitialMargin: "2.5", MaintenanceMargin: "1.25", RealizedPnL: "0", UnrealizedPnL: "0", FundingAccrual: "0", LiquidationPrice: "0", BankruptcyPrice: "0", Status: "OPEN", CreatedAt: now, UpdatedAt: now},
		{PositionID: "pos_eth_short", UserID: 9, SymbolID: 2, Side: "SHORT", Qty: "4.2", AvgEntryPrice: "50", MarkPrice: "50", Notional: "210", InitialMargin: "21", MaintenanceMargin: "10.5", RealizedPnL: "0", UnrealizedPnL: "0", FundingAccrual: "0", LiquidationPrice: "0", BankruptcyPrice: "0", Status: "OPEN", CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed positions: %v", err)
	}

	repo := NewWalletQueryRepositoryWithRiskConfig(db, nil, RiskMonitorConfig{
		HardLimitNotional:      "200",
		MaxExposureSlippageBps: 40,
	})
	dashboard, err := repo.GetRiskMonitorDashboard(context.Background())
	if err != nil {
		t.Fatalf("get risk dashboard: %v", err)
	}
	if dashboard.HardLimitNotional != "200" {
		t.Fatalf("unexpected hard limit: %+v", dashboard)
	}
	if len(dashboard.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(dashboard.Items))
	}
	if dashboard.Items[0].Symbol != "BTC-PERP" {
		t.Fatalf("expected BTC-PERP to sort first by utilization, got %+v", dashboard.Items)
	}
	if dashboard.Items[0].BlockedOpenSide == nil || *dashboard.Items[0].BlockedOpenSide != "BUY" {
		t.Fatalf("expected BTC-PERP buy side to be blocked, got %+v", dashboard.Items[0])
	}
	if dashboard.Items[0].BuyAdjustmentBps <= 0 || dashboard.Items[0].SellAdjustmentBps >= 0 {
		t.Fatalf("expected BTC-PERP dynamic slippage to penalize buys and favor sells, got %+v", dashboard.Items[0])
	}
	if decimalx.MustFromString(dashboard.Items[0].UtilizationRatio).LessThan(decimalx.MustFromString("0.6")) {
		t.Fatalf("expected BTC-PERP utilization >= 0.6, got %+v", dashboard.Items[0])
	}
	if dashboard.Items[1].Symbol != "ETH-PERP" {
		t.Fatalf("expected ETH-PERP second, got %+v", dashboard.Items)
	}
	if dashboard.Items[1].BlockedOpenSide == nil || *dashboard.Items[1].BlockedOpenSide != "SELL" {
		t.Fatalf("expected ETH-PERP sell side to be blocked, got %+v", dashboard.Items[1])
	}
}

func TestAccountQueryRepository_GetRiskReturnsNoNewRiskSnapshot(t *testing.T) {
	db := setupTestDB(t)
	now := time.Now().UTC()
	if err := db.Create(&RiskSnapshotModel{
		UserID:            7,
		Equity:            "120",
		AvailableBalance:  "-5",
		MaintenanceMargin: "50",
		MarginRatio:       "1.8",
		RiskLevel:         "NO_NEW_RISK",
		TriggeredBy:       "mark_price",
		CreatedAt:         now,
	}).Error; err != nil {
		t.Fatalf("seed risk snapshot: %v", err)
	}

	repo := NewAccountQueryRepository(db)
	risk, err := repo.GetRisk(context.Background(), 7)
	if err != nil {
		t.Fatalf("get risk: %v", err)
	}
	if risk.RiskState != "NO_NEW_RISK" {
		t.Fatalf("expected NO_NEW_RISK, got %+v", risk)
	}
	if risk.CanOpenRisk {
		t.Fatalf("expected can_open_risk=false, got %+v", risk)
	}
}

func TestWalletQueryRepository_LedgerOverviewAndAudit(t *testing.T) {
	db := setupTestDB(t)
	repo := NewWalletQueryRepository(db, nil, fakeLedgerChainReader{
		items: []chaininfra.VaultBalanceSnapshot{{
			ChainID:      31337,
			ChainKey:     "local",
			ChainName:    "Local Anvil",
			Asset:        "USDC",
			VaultAddress: "0x00000000000000000000000000000000000000aa",
			Balance:      "100",
		}},
	})
	ledgerRepo := NewLedgerRepository(db)
	now := time.Now().UTC()
	userID := uint64(7)

	accounts := []AccountModel{
		{UserID: &userID, AccountCode: "USER_WALLET", AccountType: "USER", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{UserID: &userID, AccountCode: "USER_ORDER_MARGIN", AccountType: "USER", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{UserID: &userID, AccountCode: "USER_POSITION_MARGIN", AccountType: "USER", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{UserID: &userID, AccountCode: "USER_WITHDRAW_HOLD", AccountType: "USER", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "SYSTEM_POOL", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "TRADING_FEE_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "WITHDRAW_FEE_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "PENALTY_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "FUNDING_POOL", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "INSURANCE_FUND", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "ROUNDING_DIFF_ACCOUNT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "DEPOSIT_PENDING_CONFIRM", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "WITHDRAW_IN_TRANSIT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "SWEEP_IN_TRANSIT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "CUSTODY_HOT", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "CUSTODY_WARM", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "CUSTODY_COLD", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
		{AccountCode: "TEST_FAUCET_POOL", AccountType: "SYSTEM", Asset: "USDC", Status: "ACTIVE", CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&accounts).Error; err != nil {
		t.Fatalf("seed accounts: %v", err)
	}

	accountID := map[string]uint64{}
	for _, item := range accounts {
		key := item.AccountCode
		if item.UserID != nil {
			key = item.AccountCode + ":user"
		}
		accountID[key] = item.ID
	}
	userRef := func() *uint64 { return &userID }

	postings := []ledgerdomain.PostingRequest{
		{
			LedgerTx: ledgerdomain.LedgerTx{ID: "ldg_dep", EventID: "evt_dep", BizType: "DEPOSIT", BizRefID: "dep_1", Asset: "USDC", IdempotencyKey: "idem_dep", OperatorType: "system", OperatorID: "indexer", TraceID: "trace_dep", Status: "COMMITTED", CreatedAt: now},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: accountID["USER_WALLET:user"], UserID: userRef(), Asset: "USDC", Amount: "100", EntryType: "DEPOSIT_CREDIT"},
				{AccountID: accountID["CUSTODY_HOT"], Asset: "USDC", Amount: "-100", EntryType: "CUSTODY_HOT_PENDING"},
			},
		},
		{
			LedgerTx: ledgerdomain.LedgerTx{ID: "ldg_order_hold", EventID: "evt_order_hold", BizType: "trade.order.hold", BizRefID: "ord_1", Asset: "USDC", IdempotencyKey: "idem_order_hold", OperatorType: "user", OperatorID: "7", TraceID: "trace_hold", Status: "COMMITTED", CreatedAt: now},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: accountID["USER_WALLET:user"], UserID: userRef(), Asset: "USDC", Amount: "-10", EntryType: "TRADE_ORDER_HOLD"},
				{AccountID: accountID["USER_ORDER_MARGIN:user"], UserID: userRef(), Asset: "USDC", Amount: "10", EntryType: "TRADE_ORDER_HOLD"},
			},
		},
		{
			LedgerTx: ledgerdomain.LedgerTx{ID: "ldg_position_margin", EventID: "evt_position_margin", BizType: "trade.fill", BizRefID: "ord_2", Asset: "USDC", IdempotencyKey: "idem_position_margin", OperatorType: "user", OperatorID: "7", TraceID: "trace_fill", Status: "COMMITTED", CreatedAt: now},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: accountID["USER_WALLET:user"], UserID: userRef(), Asset: "USDC", Amount: "-20", EntryType: "TRADE_POSITION_MARGIN"},
				{AccountID: accountID["USER_POSITION_MARGIN:user"], UserID: userRef(), Asset: "USDC", Amount: "20", EntryType: "TRADE_POSITION_MARGIN"},
			},
		},
		{
			LedgerTx: ledgerdomain.LedgerTx{ID: "ldg_wd_hold", EventID: "evt_wd_hold", BizType: "WITHDRAW_HOLD", BizRefID: "wd_1", Asset: "USDC", IdempotencyKey: "idem_wd_hold", OperatorType: "user", OperatorID: "7", TraceID: "trace_wd", Status: "COMMITTED", CreatedAt: now},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: accountID["USER_WALLET:user"], UserID: userRef(), Asset: "USDC", Amount: "-5", EntryType: "WITHDRAW_HOLD_OUT"},
				{AccountID: accountID["USER_WITHDRAW_HOLD:user"], UserID: userRef(), Asset: "USDC", Amount: "5", EntryType: "WITHDRAW_HOLD_IN"},
			},
		},
		{
			LedgerTx: ledgerdomain.LedgerTx{ID: "ldg_trade_fee", EventID: "evt_trade_fee", BizType: "trade.fee", BizRefID: "ord_3", Asset: "USDC", IdempotencyKey: "idem_trade_fee", OperatorType: "system", OperatorID: "matching", TraceID: "trace_fee", Status: "COMMITTED", CreatedAt: now},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: accountID["SYSTEM_POOL"], Asset: "USDC", Amount: "-2", EntryType: "TRADE_REALIZED_PNL"},
				{AccountID: accountID["TRADING_FEE_ACCOUNT"], Asset: "USDC", Amount: "2", EntryType: "TRADE_FEE"},
			},
		},
		{
			LedgerTx: ledgerdomain.LedgerTx{ID: "ldg_withdraw_fee", EventID: "evt_withdraw_fee", BizType: "WITHDRAW_BROADCAST", BizRefID: "wd_2", Asset: "USDC", IdempotencyKey: "idem_withdraw_fee", OperatorType: "system", OperatorID: "wallet", TraceID: "trace_withdraw_fee", Status: "COMMITTED", CreatedAt: now},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: accountID["SYSTEM_POOL"], Asset: "USDC", Amount: "-1", EntryType: "WITHDRAW_FEE_POOL"},
				{AccountID: accountID["WITHDRAW_FEE_ACCOUNT"], Asset: "USDC", Amount: "1", EntryType: "WITHDRAW_FEE"},
			},
		},
	}
	for _, posting := range postings {
		if err := ledgerRepo.CreatePosting(context.Background(), posting); err != nil {
			t.Fatalf("create posting %s: %v", posting.LedgerTx.ID, err)
		}
	}

	if err := db.Create(&DepositChainTxModel{
		DepositID:          "dep_1",
		UserID:             userID,
		ChainID:            31337,
		TxHash:             "0xdep",
		LogIndex:           1,
		FromAddress:        "0x1",
		ToAddress:          "0x2",
		TokenAddress:       "0x3",
		Amount:             "100",
		BlockNumber:        1,
		Confirmations:      1,
		Status:             "CREDITED",
		CreditedLedgerTxID: "ldg_dep",
		CreatedAt:          now,
		UpdatedAt:          now,
	}).Error; err != nil {
		t.Fatalf("seed deposit: %v", err)
	}
	if err := db.Create(&WithdrawRequestModel{
		WithdrawID:     "wd_1",
		UserID:         userID,
		ChainID:        31337,
		Asset:          "USDC",
		Amount:         "5",
		FeeAmount:      "1",
		ToAddress:      "0x4",
		Status:         "HOLD",
		HoldLedgerTxID: "ldg_wd_hold",
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error; err != nil {
		t.Fatalf("seed withdraw: %v", err)
	}

	overview, err := repo.GetLedgerOverview(context.Background(), "")
	if err != nil {
		t.Fatalf("get ledger overview: %v", err)
	}
	if len(overview.Assets) != 1 {
		t.Fatalf("expected 1 asset overview, got %d", len(overview.Assets))
	}
	item := overview.Assets[0]
	if item.Asset != "USDC" || item.UserLiability != "100" || item.UserMargin != "30" || item.PlatformRevenue != "3" || item.NetBalance != "0" {
		t.Fatalf("unexpected overview: %+v", item)
	}
	filtered, err := repo.GetLedgerOverview(context.Background(), "USDC")
	if err != nil {
		t.Fatalf("get scoped ledger overview: %v", err)
	}
	if filtered.ScopeAsset != "USDC" || len(filtered.Assets) != 1 {
		t.Fatalf("unexpected scoped overview: %+v", filtered)
	}

	report, err := repo.RunLedgerAudit(context.Background(), "0xadmin", "")
	if err != nil {
		t.Fatalf("run ledger audit: %v", err)
	}
	if report.Status != "PASS" {
		t.Fatalf("expected PASS audit, got %+v", report)
	}
	if len(report.Checks) != 7 {
		t.Fatalf("expected 7 audit checks, got %d", len(report.Checks))
	}
	if len(report.ChainBalances) != 2 {
		t.Fatalf("expected chain balance details, got %+v", report.ChainBalances)
	}

	latest, err := repo.GetLatestLedgerAuditReport(context.Background(), "")
	if err != nil {
		t.Fatalf("get latest audit: %v", err)
	}
	if latest.AuditReportID != report.AuditReportID {
		t.Fatalf("expected latest audit %s, got %s", report.AuditReportID, latest.AuditReportID)
	}
}
