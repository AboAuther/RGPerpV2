package indexer

import (
	"context"
	"fmt"
	"testing"
	"time"

	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeIDGen struct{ n int }

func (f *fakeIDGen) NewID(prefix string) string {
	f.n++
	return fmt.Sprintf("%s_%d", prefix, f.n)
}

type passthroughTxManager struct{}

func (passthroughTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

type memoryDeposits struct {
	byID    map[string]walletdomain.DepositChainTx
	byTxLog map[string]string
}

func newMemoryDeposits() *memoryDeposits {
	return &memoryDeposits{
		byID:    map[string]walletdomain.DepositChainTx{},
		byTxLog: map[string]string{},
	}
}

func (m *memoryDeposits) GetByID(_ context.Context, depositID string) (walletdomain.DepositChainTx, error) {
	deposit, ok := m.byID[depositID]
	if !ok {
		return walletdomain.DepositChainTx{}, errorsx.ErrNotFound
	}
	return deposit, nil
}

func (m *memoryDeposits) GetByTxLog(_ context.Context, chainID int64, txHash string, logIndex int64) (walletdomain.DepositChainTx, error) {
	id, ok := m.byTxLog[fmt.Sprintf("%d:%s:%d", chainID, txHash, logIndex)]
	if !ok {
		return walletdomain.DepositChainTx{}, errorsx.ErrNotFound
	}
	return m.byID[id], nil
}

func (m *memoryDeposits) ListPendingByChain(_ context.Context, chainID int64, statuses []string, limit int) ([]walletdomain.DepositChainTx, error) {
	statusSet := map[string]bool{}
	for _, status := range statuses {
		statusSet[status] = true
	}
	out := make([]walletdomain.DepositChainTx, 0)
	for _, deposit := range m.byID {
		if deposit.ChainID != chainID {
			continue
		}
		if len(statusSet) > 0 && !statusSet[deposit.Status] {
			continue
		}
		out = append(out, deposit)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

type memoryWithdraws struct {
	items       map[string]walletdomain.WithdrawRequest
	updateCalls []string
}

func newMemoryWithdraws() *memoryWithdraws {
	return &memoryWithdraws{items: map[string]walletdomain.WithdrawRequest{}}
}

func (m *memoryWithdraws) GetByID(_ context.Context, withdrawID string) (walletdomain.WithdrawRequest, error) {
	withdraw, ok := m.items[withdrawID]
	if !ok {
		return walletdomain.WithdrawRequest{}, errorsx.ErrNotFound
	}
	return withdraw, nil
}

func (m *memoryWithdraws) ListByChainStatuses(_ context.Context, chainID int64, statuses []string, limit int) ([]walletdomain.WithdrawRequest, error) {
	statusSet := map[string]bool{}
	for _, status := range statuses {
		statusSet[status] = true
	}
	out := make([]walletdomain.WithdrawRequest, 0)
	for _, withdraw := range m.items {
		if withdraw.ChainID != chainID {
			continue
		}
		if len(statusSet) > 0 && !statusSet[withdraw.Status] {
			continue
		}
		out = append(out, withdraw)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memoryWithdraws) UpdateStatus(_ context.Context, withdrawID string, _ []string, to string) error {
	withdraw, ok := m.items[withdrawID]
	if !ok {
		return errorsx.ErrNotFound
	}
	withdraw.Status = to
	m.items[withdrawID] = withdraw
	m.updateCalls = append(m.updateCalls, withdrawID+":"+to)
	return nil
}

type addressResolver struct {
	items map[string]walletdomain.DepositAddress
}

func (a addressResolver) GetByChainAddress(_ context.Context, chainID int64, address string) (walletdomain.DepositAddress, error) {
	item, ok := a.items[fmt.Sprintf("%d:%s", chainID, address)]
	if !ok {
		return walletdomain.DepositAddress{}, errorsx.ErrNotFound
	}
	return item, nil
}

func (a addressResolver) AssignToUser(_ context.Context, userID uint64, chainID int64, asset string, address string) error {
	key := fmt.Sprintf("%d:%s", chainID, address)
	a.items[key] = walletdomain.DepositAddress{
		UserID: userID, ChainID: chainID, Asset: asset, Address: address, Status: "ACTIVE",
	}
	return nil
}

type publisher struct {
	events []EventEnvelope
}

func (p *publisher) Publish(_ context.Context, envelope EventEnvelope) error {
	p.events = append(p.events, envelope)
	return nil
}

type cursorRepo struct {
	items map[string]Cursor
}

func newCursorRepo() *cursorRepo {
	return &cursorRepo{items: map[string]Cursor{}}
}

func (c *cursorRepo) Get(_ context.Context, chainID int64, cursorType string) (Cursor, error) {
	item, ok := c.items[fmt.Sprintf("%d:%s", chainID, cursorType)]
	if !ok {
		return Cursor{}, errorsx.ErrNotFound
	}
	return item, nil
}

func (c *cursorRepo) Upsert(_ context.Context, chainID int64, cursorType string, cursorValue string, updatedAt time.Time) error {
	c.items[fmt.Sprintf("%d:%s", chainID, cursorType)] = Cursor{
		ChainID:     chainID,
		CursorType:  cursorType,
		CursorValue: cursorValue,
		UpdatedAt:   updatedAt,
	}
	return nil
}

type source struct {
	latest    int64
	blockHash string
	deposits  []DepositObserved
	withdraws []WithdrawExecuted
	receipts  map[string]ReceiptStatus
}

func (s source) LatestBlockNumber(_ context.Context, _ int64) (int64, error) {
	return s.latest, nil
}

func (s source) BlockHash(_ context.Context, _ int64, _ int64) (string, error) {
	if s.blockHash == "" {
		return "0xgenesis", nil
	}
	return s.blockHash, nil
}

func (s source) ListRouterCreatedEvents(_ context.Context, _ int64, fromBlock int64, toBlock int64) ([]RouterCreated, error) {
	return nil, nil
}

func (s source) ListDepositEvents(_ context.Context, _ int64, fromBlock int64, toBlock int64) ([]DepositObserved, error) {
	out := make([]DepositObserved, 0)
	for _, event := range s.deposits {
		if event.BlockNumber >= fromBlock && event.BlockNumber <= toBlock {
			out = append(out, event)
		}
	}
	return out, nil
}

func (s source) ListWithdrawEvents(_ context.Context, _ int64, fromBlock int64, toBlock int64) ([]WithdrawExecuted, error) {
	out := make([]WithdrawExecuted, 0)
	for _, event := range s.withdraws {
		if event.BlockNumber >= fromBlock && event.BlockNumber <= toBlock {
			out = append(out, event)
		}
	}
	return out, nil
}

func (s source) GetReceiptStatus(_ context.Context, _ int64, txHash string) (ReceiptStatus, error) {
	if s.receipts == nil {
		return ReceiptStatus{Found: false}, nil
	}
	if status, ok := s.receipts[txHash]; ok {
		return status, nil
	}
	return ReceiptStatus{Found: false}, nil
}

type fakeWallet struct {
	deposits       *memoryDeposits
	withdraws      *memoryWithdraws
	detectCalls    int
	advanceCalls   int
	confirmCalls   int
	reverseCalls   int
	broadcastCalls int
	completeCalls  int
	refundCalls    int
}

func (f *fakeWallet) DetectDeposit(_ context.Context, input walletdomain.DetectDepositInput) (walletdomain.DepositChainTx, error) {
	f.detectCalls++
	status := walletdomain.StatusDetected
	if input.Confirmations > 0 {
		status = walletdomain.StatusConfirming
	}
	if input.RequiredConfs > 0 && input.Confirmations >= input.RequiredConfs {
		status = walletdomain.StatusCreditReady
	}
	deposit := walletdomain.DepositChainTx{
		DepositID:     "dep_" + input.TxHash,
		UserID:        input.UserID,
		ChainID:       input.ChainID,
		TxHash:        input.TxHash,
		LogIndex:      input.LogIndex,
		FromAddress:   input.FromAddress,
		ToAddress:     input.ToAddress,
		TokenAddress:  input.TokenAddress,
		Amount:        input.Amount,
		Asset:         input.Asset,
		BlockNumber:   input.BlockNumber,
		Confirmations: input.Confirmations,
		RequiredConfs: input.RequiredConfs,
		Status:        status,
	}
	f.deposits.byID[deposit.DepositID] = deposit
	f.deposits.byTxLog[fmt.Sprintf("%d:%s:%d", input.ChainID, input.TxHash, input.LogIndex)] = deposit.DepositID
	return deposit, nil
}

func (f *fakeWallet) AdvanceDeposit(_ context.Context, input walletdomain.AdvanceDepositInput) error {
	f.advanceCalls++
	deposit := f.deposits.byID[input.DepositID]
	deposit.Confirmations = input.Confirmations
	deposit.RequiredConfs = input.RequiredConfs
	deposit.Status = depositStatusFor(input.Confirmations, input.RequiredConfs)
	f.deposits.byID[input.DepositID] = deposit
	return nil
}

func (f *fakeWallet) ConfirmDeposit(_ context.Context, input walletdomain.ConfirmDepositInput) error {
	f.confirmCalls++
	deposit := f.deposits.byID[input.DepositID]
	deposit.Status = walletdomain.StatusCredited
	deposit.CreditedLedgerTxID = "ldg_" + input.DepositID
	f.deposits.byID[input.DepositID] = deposit
	return nil
}

func (f *fakeWallet) ReverseDeposit(_ context.Context, input walletdomain.ReverseDepositInput) error {
	f.reverseCalls++
	deposit := f.deposits.byID[input.DepositID]
	deposit.Status = walletdomain.StatusReorgReversed
	f.deposits.byID[input.DepositID] = deposit
	return nil
}

func (f *fakeWallet) MarkWithdrawBroadcasted(_ context.Context, input walletdomain.BroadcastWithdrawInput) error {
	f.broadcastCalls++
	withdraw := f.withdraws.items[input.WithdrawID]
	withdraw.Status = walletdomain.StatusBroadcasted
	withdraw.BroadcastTxHash = input.TxHash
	f.withdraws.items[input.WithdrawID] = withdraw
	return nil
}

func (f *fakeWallet) CompleteWithdraw(_ context.Context, input walletdomain.CompleteWithdrawInput) error {
	f.completeCalls++
	withdraw := f.withdraws.items[input.WithdrawID]
	withdraw.Status = walletdomain.StatusCompleted
	withdraw.BroadcastTxHash = input.TxHash
	f.withdraws.items[input.WithdrawID] = withdraw
	return nil
}

func (f *fakeWallet) RefundWithdraw(_ context.Context, input walletdomain.RefundWithdrawInput) error {
	f.refundCalls++
	withdraw := f.withdraws.items[input.WithdrawID]
	withdraw.Status = walletdomain.StatusRefunded
	f.withdraws.items[input.WithdrawID] = withdraw
	return nil
}

func TestHandleDepositObserved_CreditsFinalizedDeposit(t *testing.T) {
	deposits := newMemoryDeposits()
	withdraws := newMemoryWithdraws()
	pub := &publisher{}
	svc, err := NewService(
		&fakeWallet{deposits: deposits, withdraws: withdraws},
		deposits,
		withdraws,
		addressResolver{items: map[string]walletdomain.DepositAddress{
			"8453:0x00000000000000000000000000000000000000aa": {
				UserID: 7, ChainID: 8453, Asset: "USDC", Address: "0x00000000000000000000000000000000000000aa", Status: "ACTIVE",
			},
		}},
		pub,
		passthroughTxManager{},
		fakeClock{now: time.Unix(100, 0).UTC()},
		&fakeIDGen{},
		[]ChainRule{{ChainID: 8453, Asset: "USDC", RequiredConfirmations: 20, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}},
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.HandleDepositObserved(context.Background(), DepositObserved{
		ChainID:       8453,
		UserID:        7,
		TxHash:        "0xabc",
		LogIndex:      1,
		BlockNumber:   100,
		Confirmations: 20,
		RouterAddress: "0x00000000000000000000000000000000000000aa",
		VaultAddress:  "0x00000000000000000000000000000000000000bb",
		TokenAddress:  "0x00000000000000000000000000000000000000cc",
		FromAddress:   "0x00000000000000000000000000000000000000dd",
		Amount:        "100",
		TraceID:       "trace_1",
	})
	if err != nil {
		t.Fatalf("handle deposit: %v", err)
	}

	deposit, err := deposits.GetByTxLog(context.Background(), 8453, "0xabc", 1)
	if err != nil {
		t.Fatalf("get deposit: %v", err)
	}
	if deposit.Status != walletdomain.StatusCredited {
		t.Fatalf("expected credited status, got %s", deposit.Status)
	}
	if len(pub.events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(pub.events))
	}
	if pub.events[0].EventType != "wallet.deposit.detected" || pub.events[1].EventType != "wallet.deposit.credit_ready" || pub.events[2].EventType != "wallet.deposit.credited" {
		t.Fatalf("unexpected event order: %#v", []string{pub.events[0].EventType, pub.events[1].EventType, pub.events[2].EventType})
	}
}

func TestReconcileDeposits_AdvancesAndCreditsPendingDeposit(t *testing.T) {
	deposits := newMemoryDeposits()
	deposits.byID["dep_1"] = walletdomain.DepositChainTx{
		DepositID: "dep_1", UserID: 7, ChainID: 8453, TxHash: "0xdef", LogIndex: 2, FromAddress: "0x00000000000000000000000000000000000000dd",
		ToAddress: "0x00000000000000000000000000000000000000aa", TokenAddress: "0x00000000000000000000000000000000000000cc", Amount: "50", Asset: "USDC",
		BlockNumber: 100, Confirmations: 2, RequiredConfs: 20, Status: walletdomain.StatusConfirming,
	}
	deposits.byTxLog["8453:0xdef:2"] = "dep_1"
	withdraws := newMemoryWithdraws()
	pub := &publisher{}
	wallet := &fakeWallet{deposits: deposits, withdraws: withdraws}
	svc, err := NewService(
		wallet,
		deposits,
		withdraws,
		addressResolver{items: map[string]walletdomain.DepositAddress{
			"8453:0x00000000000000000000000000000000000000aa": {
				UserID: 7, ChainID: 8453, Asset: "USDC", Address: "0x00000000000000000000000000000000000000aa", Status: "ACTIVE",
			},
		}},
		pub,
		passthroughTxManager{},
		fakeClock{now: time.Unix(100, 0).UTC()},
		&fakeIDGen{},
		[]ChainRule{{ChainID: 8453, Asset: "USDC", RequiredConfirmations: 20, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}},
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := svc.ReconcileDeposits(context.Background(), 8453, 130, 10); err != nil {
		t.Fatalf("reconcile deposits: %v", err)
	}
	if wallet.advanceCalls != 1 || wallet.confirmCalls != 1 {
		t.Fatalf("expected advance and confirm once, got advance=%d confirm=%d", wallet.advanceCalls, wallet.confirmCalls)
	}
	if deposits.byID["dep_1"].Status != walletdomain.StatusCredited {
		t.Fatalf("expected deposit credited, got %s", deposits.byID["dep_1"].Status)
	}
}

func TestHandleDepositObserved_UnknownRouterPublishesAnomaly(t *testing.T) {
	deposits := newMemoryDeposits()
	withdraws := newMemoryWithdraws()
	pub := &publisher{}
	wallet := &fakeWallet{deposits: deposits, withdraws: withdraws}
	svc, err := NewService(
		wallet,
		deposits,
		withdraws,
		addressResolver{items: map[string]walletdomain.DepositAddress{}},
		pub,
		passthroughTxManager{},
		fakeClock{now: time.Unix(100, 0).UTC()},
		&fakeIDGen{},
		[]ChainRule{{ChainID: 8453, Asset: "USDC", RequiredConfirmations: 20, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}},
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.HandleDepositObserved(context.Background(), DepositObserved{
		ChainID:       8453,
		TxHash:        "0xabc",
		LogIndex:      1,
		BlockNumber:   100,
		Confirmations: 1,
		RouterAddress: "0x00000000000000000000000000000000000000aa",
		VaultAddress:  "0x00000000000000000000000000000000000000bb",
		TokenAddress:  "0x00000000000000000000000000000000000000cc",
		FromAddress:   "0x00000000000000000000000000000000000000dd",
		Amount:        "100",
		TraceID:       "trace_1",
	})
	if err != nil {
		t.Fatalf("handle deposit: %v", err)
	}
	if wallet.detectCalls != 0 {
		t.Fatalf("expected no wallet mutation for unknown router")
	}
	if len(pub.events) != 1 || pub.events[0].EventType != "wallet.indexer.anomaly" {
		t.Fatalf("expected anomaly event, got %#v", pub.events)
	}
}

func TestHandleWithdrawExecuted_BackfillsBroadcastAndCompletes(t *testing.T) {
	deposits := newMemoryDeposits()
	withdraws := newMemoryWithdraws()
	withdraws.items["wd_1"] = walletdomain.WithdrawRequest{
		WithdrawID: "wd_1", UserID: 7, ChainID: 8453, Asset: "USDC", Amount: "100", FeeAmount: "1",
		ToAddress: "0x00000000000000000000000000000000000000ee", Status: walletdomain.StatusApproved,
	}
	pub := &publisher{}
	wallet := &fakeWallet{deposits: deposits, withdraws: withdraws}
	svc, err := NewService(
		wallet,
		deposits,
		withdraws,
		addressResolver{items: map[string]walletdomain.DepositAddress{}},
		pub,
		passthroughTxManager{},
		fakeClock{now: time.Unix(100, 0).UTC()},
		&fakeIDGen{},
		[]ChainRule{{ChainID: 8453, Asset: "USDC", RequiredConfirmations: 20, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}},
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.HandleWithdrawExecuted(context.Background(), WithdrawExecuted{
		ChainID:      8453,
		WithdrawID:   "wd_1",
		TxHash:       "0xwd",
		LogIndex:     1,
		BlockNumber:  100,
		VaultAddress: "0x00000000000000000000000000000000000000bb",
		TokenAddress: "0x00000000000000000000000000000000000000cc",
		ToAddress:    "0x00000000000000000000000000000000000000ee",
		Amount:       "99",
		Operator:     "0x00000000000000000000000000000000000000ff",
		TraceID:      "trace_2",
	})
	if err != nil {
		t.Fatalf("handle withdraw executed: %v", err)
	}
	if wallet.broadcastCalls != 1 || wallet.completeCalls != 1 {
		t.Fatalf("expected backfill broadcast and complete, got broadcast=%d complete=%d", wallet.broadcastCalls, wallet.completeCalls)
	}
	if withdraws.items["wd_1"].Status != walletdomain.StatusCompleted {
		t.Fatalf("expected withdraw completed, got %s", withdraws.items["wd_1"].Status)
	}
	if len(pub.events) != 2 || pub.events[0].EventType != "wallet.withdraw.broadcasted" || pub.events[1].EventType != "wallet.withdraw.completed" {
		t.Fatalf("unexpected withdraw events: %#v", pub.events)
	}
}

func TestHandleWithdrawFailed_RefundsFailedWithdraw(t *testing.T) {
	deposits := newMemoryDeposits()
	withdraws := newMemoryWithdraws()
	withdraws.items["wd_1"] = walletdomain.WithdrawRequest{
		WithdrawID: "wd_1", UserID: 7, ChainID: 8453, Asset: "USDC", Amount: "100", FeeAmount: "1",
		ToAddress: "0x00000000000000000000000000000000000000ee", Status: walletdomain.StatusBroadcasted, BroadcastTxHash: "0xwd",
	}
	pub := &publisher{}
	wallet := &fakeWallet{deposits: deposits, withdraws: withdraws}
	svc, err := NewService(
		wallet,
		deposits,
		withdraws,
		addressResolver{items: map[string]walletdomain.DepositAddress{}},
		pub,
		passthroughTxManager{},
		fakeClock{now: time.Unix(100, 0).UTC()},
		&fakeIDGen{},
		[]ChainRule{{ChainID: 8453, Asset: "USDC", RequiredConfirmations: 20, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}},
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.HandleWithdrawFailed(context.Background(), WithdrawFailed{
		ChainID:    8453,
		WithdrawID: "wd_1",
		TxHash:     "0xwd",
		Reason:     "receipt_status_failed",
		TraceID:    "trace_3",
	})
	if err != nil {
		t.Fatalf("handle withdraw failed: %v", err)
	}
	if wallet.refundCalls != 1 {
		t.Fatalf("expected refund call")
	}
	if withdraws.items["wd_1"].Status != walletdomain.StatusRefunded {
		t.Fatalf("expected refunded status, got %s", withdraws.items["wd_1"].Status)
	}
	if len(pub.events) != 1 || pub.events[0].EventType != "wallet.withdraw.failed" {
		t.Fatalf("expected withdraw.failed event, got %#v", pub.events)
	}
}

func TestRunner_SyncChainAdvancesCursors(t *testing.T) {
	deposits := newMemoryDeposits()
	withdraws := newMemoryWithdraws()
	pub := &publisher{}
	wallet := &fakeWallet{deposits: deposits, withdraws: withdraws}
	chainRules := []ChainRule{{ChainID: 8453, Asset: "USDC", RequiredConfirmations: 20, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}}
	svc, err := NewService(
		wallet,
		deposits,
		withdraws,
		addressResolver{items: map[string]walletdomain.DepositAddress{
			"8453:0x00000000000000000000000000000000000000aa": {
				UserID: 7, ChainID: 8453, Asset: "USDC", Address: "0x00000000000000000000000000000000000000aa", Status: "ACTIVE",
			},
		}},
		pub,
		passthroughTxManager{},
		fakeClock{now: time.Unix(100, 0).UTC()},
		&fakeIDGen{},
		chainRules,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	cursors := newCursorRepo()
	runner, err := NewRunner(source{
		latest: 120,
		deposits: []DepositObserved{{
			ChainID:       8453,
			UserID:        7,
			TxHash:        "0xabc",
			LogIndex:      1,
			BlockNumber:   100,
			RouterAddress: "0x00000000000000000000000000000000000000aa",
			VaultAddress:  "0x00000000000000000000000000000000000000bb",
			TokenAddress:  "0x00000000000000000000000000000000000000cc",
			FromAddress:   "0x00000000000000000000000000000000000000dd",
			Amount:        "100",
			TraceID:       "trace_4",
		}},
	}, svc, cursors, fakeClock{now: time.Unix(100, 0).UTC()}, chainRules, 500)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	if err := runner.SyncChain(context.Background(), 8453); err != nil {
		t.Fatalf("sync chain: %v", err)
	}
	if cursors.items["8453:"+CursorTypeDepositScan].CursorValue != "120" {
		t.Fatalf("expected deposit cursor advanced to 120, got %#v", cursors.items)
	}
	if cursors.items["8453:"+CursorTypeWithdrawScan].CursorValue != "120" {
		t.Fatalf("expected withdraw cursor advanced to 120, got %#v", cursors.items)
	}
	if wallet.confirmCalls != 1 {
		t.Fatalf("expected one credited deposit from sync, got %d", wallet.confirmCalls)
	}
}

func TestRunner_ScanRangeResetsCursorWhenChainHeightRegresses(t *testing.T) {
	chainRules := []ChainRule{{ChainID: 31337, Asset: "USDC", RequiredConfirmations: 1, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}}
	svc, err := NewService(
		&fakeWallet{deposits: newMemoryDeposits(), withdraws: newMemoryWithdraws()},
		newMemoryDeposits(),
		newMemoryWithdraws(),
		addressResolver{items: map[string]walletdomain.DepositAddress{}},
		&publisher{},
		passthroughTxManager{},
		fakeClock{now: time.Unix(100, 0).UTC()},
		&fakeIDGen{},
		chainRules,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	cursors := newCursorRepo()
	if err := cursors.Upsert(context.Background(), 31337, CursorTypeDepositScan, "48", time.Unix(90, 0).UTC()); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}
	runner, err := NewRunner(source{latest: 12}, svc, cursors, fakeClock{now: time.Unix(100, 0).UTC()}, chainRules, 500)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}

	fromBlock, toBlock, err := runner.scanRange(context.Background(), 31337, CursorTypeDepositScan, 12)
	if err != nil {
		t.Fatalf("scan range: %v", err)
	}
	if fromBlock != 1 || toBlock != 12 {
		t.Fatalf("expected rescan from genesis-ish range 1..12, got %d..%d", fromBlock, toBlock)
	}
	if got := cursors.items["31337:"+CursorTypeDepositScan].CursorValue; got != "0" {
		t.Fatalf("expected cursor reset to 0, got %s", got)
	}
}

func TestRunner_EnsureChainIdentityResetsScanCursors(t *testing.T) {
	chainRules := []ChainRule{{ChainID: 31337, Asset: "USDC", RequiredConfirmations: 1, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}}
	svc, err := NewService(
		&fakeWallet{deposits: newMemoryDeposits(), withdraws: newMemoryWithdraws()},
		newMemoryDeposits(),
		newMemoryWithdraws(),
		addressResolver{items: map[string]walletdomain.DepositAddress{}},
		&publisher{},
		passthroughTxManager{},
		fakeClock{now: time.Unix(100, 0).UTC()},
		&fakeIDGen{},
		chainRules,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	cursors := newCursorRepo()
	_ = cursors.Upsert(context.Background(), 31337, CursorTypeChainIDHash, "0xold", time.Unix(90, 0).UTC())
	_ = cursors.Upsert(context.Background(), 31337, CursorTypeDepositScan, "48", time.Unix(90, 0).UTC())
	_ = cursors.Upsert(context.Background(), 31337, CursorTypeWithdrawScan, "48", time.Unix(90, 0).UTC())
	runner, err := NewRunner(source{latest: 12, blockHash: "0xnew"}, svc, cursors, fakeClock{now: time.Unix(100, 0).UTC()}, chainRules, 500)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if err := runner.ensureChainIdentity(context.Background(), 31337); err != nil {
		t.Fatalf("ensure chain identity: %v", err)
	}
	if got := cursors.items["31337:"+CursorTypeDepositScan].CursorValue; got != "0" {
		t.Fatalf("expected deposit cursor reset, got %s", got)
	}
	if got := cursors.items["31337:"+CursorTypeWithdrawScan].CursorValue; got != "0" {
		t.Fatalf("expected withdraw cursor reset, got %s", got)
	}
	if got := cursors.items["31337:"+CursorTypeChainIDHash].CursorValue; got != "0xnew" {
		t.Fatalf("expected chain identity updated, got %s", got)
	}
}

func TestRunner_SyncDepositReorgs_ReversesPendingReceiptLoss(t *testing.T) {
	deposits := newMemoryDeposits()
	deposits.byID["dep_1"] = walletdomain.DepositChainTx{
		DepositID: "dep_1", UserID: 7, ChainID: 8453, TxHash: "0xdep", LogIndex: 1,
		BlockNumber: 100, Status: walletdomain.StatusConfirming, Asset: "USDC",
	}
	deposits.byTxLog["8453:0xdep:1"] = "dep_1"
	withdraws := newMemoryWithdraws()
	wallet := &fakeWallet{deposits: deposits, withdraws: withdraws}
	svc, err := NewService(
		wallet,
		deposits,
		withdraws,
		addressResolver{items: map[string]walletdomain.DepositAddress{}},
		&publisher{},
		passthroughTxManager{},
		fakeClock{now: time.Unix(100, 0).UTC()},
		&fakeIDGen{},
		[]ChainRule{{ChainID: 8453, Asset: "USDC", RequiredConfirmations: 20, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}},
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	runner, err := NewRunner(source{latest: 130, receipts: map[string]ReceiptStatus{"0xdep": {Found: false}}}, svc, newCursorRepo(), fakeClock{now: time.Unix(100, 0).UTC()}, []ChainRule{{ChainID: 8453, Asset: "USDC", RequiredConfirmations: 20, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}}, 500)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if err := runner.syncDepositReorgs(context.Background(), 8453, 130); err != nil {
		t.Fatalf("sync deposit reorgs: %v", err)
	}
	if wallet.reverseCalls != 1 {
		t.Fatalf("expected reverse deposit once, got %d", wallet.reverseCalls)
	}
}

func TestRunner_SyncStuckSigningWithdrawalsEscalatesReview(t *testing.T) {
	withdraws := newMemoryWithdraws()
	withdraws.items["wd_1"] = walletdomain.WithdrawRequest{
		WithdrawID: "wd_1", UserID: 7, ChainID: 8453, Asset: "USDC", Amount: "100", FeeAmount: "1",
		Status: walletdomain.StatusSigning, CreatedAt: time.Unix(10, 0).UTC(), UpdatedAt: time.Unix(10, 0).UTC(),
	}
	svc, err := NewService(
		&fakeWallet{deposits: newMemoryDeposits(), withdraws: withdraws},
		newMemoryDeposits(),
		withdraws,
		addressResolver{items: map[string]walletdomain.DepositAddress{}},
		&publisher{},
		passthroughTxManager{},
		fakeClock{now: time.Unix(100, 0).UTC()},
		&fakeIDGen{},
		[]ChainRule{{ChainID: 8453, Asset: "USDC", RequiredConfirmations: 20, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}},
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	runner, err := NewRunner(source{latest: 100}, svc, newCursorRepo(), fakeClock{now: time.Unix(100, 0).UTC()}, []ChainRule{{ChainID: 8453, Asset: "USDC", RequiredConfirmations: 20, VaultAddress: "0x00000000000000000000000000000000000000bb", TokenAddress: "0x00000000000000000000000000000000000000cc"}}, 500)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	if err := runner.syncStuckSigningWithdrawals(context.Background(), 8453); err != nil {
		t.Fatalf("sync stuck signing: %v", err)
	}
	if got := withdraws.items["wd_1"].Status; got != walletdomain.StatusRiskReview {
		t.Fatalf("expected stuck signing to move to risk review, got %s", got)
	}
}
