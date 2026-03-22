package wallet

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeIDGen struct {
	values []string
	idx    int
}

func (f *fakeIDGen) NewID(_ string) string {
	v := f.values[f.idx]
	f.idx++
	return v
}

type safeIDGen struct {
	mu     sync.Mutex
	values []string
	idx    int
}

func (f *safeIDGen) NewID(_ string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := f.values[f.idx]
	f.idx++
	return v
}

type stubDepositRepo struct {
	deposit DepositChainTx
	list    []DepositChainTx
	credit  struct {
		id         string
		ledgerTxID string
	}
	getErr    error
	createErr error
	updateErr error
	markErr   error
}

func (s *stubDepositRepo) Create(_ context.Context, deposit DepositChainTx) error {
	s.deposit = deposit
	return s.createErr
}
func (s *stubDepositRepo) GetByID(_ context.Context, _ string) (DepositChainTx, error) {
	return s.deposit, s.getErr
}
func (s *stubDepositRepo) GetByTxLog(_ context.Context, _ int64, _ string, _ int64) (DepositChainTx, error) {
	if s.getErr != nil {
		return DepositChainTx{}, s.getErr
	}
	if s.deposit.DepositID == "" {
		return DepositChainTx{}, errorsx.ErrNotFound
	}
	return s.deposit, nil
}
func (s *stubDepositRepo) ListPendingByChain(_ context.Context, _ int64, _ []string, _ int) ([]DepositChainTx, error) {
	return s.list, nil
}
func (s *stubDepositRepo) UpdateConfirmations(_ context.Context, depositID string, confirmations int, status string) error {
	s.deposit.DepositID = depositID
	s.deposit.Confirmations = confirmations
	s.deposit.Status = status
	return s.updateErr
}
func (s *stubDepositRepo) MarkCredited(_ context.Context, depositID string, ledgerTxID string) error {
	s.credit.id = depositID
	s.credit.ledgerTxID = ledgerTxID
	return s.markErr
}
func (s *stubDepositRepo) MarkReorgReversed(_ context.Context, depositID string) error {
	s.deposit.DepositID = depositID
	s.deposit.Status = StatusReorgReversed
	return s.markErr
}
func (s *stubDepositRepo) ListByUser(_ context.Context, _ uint64) ([]DepositChainTx, error) {
	return s.list, nil
}

type stubWithdrawRepo struct {
	created    WithdrawRequest
	withdraw   WithdrawRequest
	err        error
	broadcasts []string
	refunds    []string
}

func (s *stubWithdrawRepo) Create(_ context.Context, withdraw WithdrawRequest) error {
	s.created = withdraw
	return s.err
}
func (s *stubWithdrawRepo) GetByID(_ context.Context, _ string) (WithdrawRequest, error) {
	return s.withdraw, s.err
}
func (s *stubWithdrawRepo) MarkBroadcasted(_ context.Context, withdrawID string, txHash string) error {
	s.broadcasts = append(s.broadcasts, withdrawID+":"+txHash)
	return s.err
}
func (s *stubWithdrawRepo) UpdateStatus(_ context.Context, withdrawID string, _ []string, to string) error {
	s.withdraw.Status = to
	return s.err
}
func (s *stubWithdrawRepo) MarkCompleted(_ context.Context, _ string) error {
	return s.err
}
func (s *stubWithdrawRepo) MarkRefunded(_ context.Context, withdrawID string) error {
	s.refunds = append(s.refunds, withdrawID)
	return s.err
}
func (s *stubWithdrawRepo) ListByUser(_ context.Context, _ uint64) ([]WithdrawRequest, error) {
	return nil, s.err
}

type stubTransferResolver struct {
	userID uint64
	err    error
}

func (s *stubTransferResolver) ResolveUserIDByAddress(_ context.Context, _ string) (uint64, error) {
	return s.userID, s.err
}

type stubLedger struct {
	req  ledgerdomain.PostingRequest
	reqs []ledgerdomain.PostingRequest
	err  error
}

func (s *stubLedger) Post(_ context.Context, req ledgerdomain.PostingRequest) error {
	s.req = req
	s.reqs = append(s.reqs, req)
	return s.err
}

type stubTxManager struct{ err error }

func (s stubTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.err != nil {
		return s.err
	}
	return fn(ctx)
}

type stubAccounts struct{}

func (stubAccounts) UserWalletAccountID(_ context.Context, userID uint64, _ string) (uint64, error) {
	return userID*10 + 1, nil
}
func (stubAccounts) UserWithdrawHoldAccountID(_ context.Context, userID uint64, _ string) (uint64, error) {
	return userID*10 + 2, nil
}
func (stubAccounts) DepositPendingAccountID(_ context.Context, _ string) (uint64, error) {
	return 9001, nil
}
func (stubAccounts) WithdrawInTransitAccountID(_ context.Context, _ string) (uint64, error) {
	return 9002, nil
}
func (stubAccounts) WithdrawFeeAccountID(_ context.Context, _ string) (uint64, error) {
	return 9003, nil
}
func (stubAccounts) CustodyHotAccountID(_ context.Context, _ string) (uint64, error) {
	return 9004, nil
}
func (stubAccounts) TestFaucetPoolAccountID(_ context.Context, _ string) (uint64, error) {
	return 9005, nil
}

type stubBalances struct {
	value string
	err   error
}

func (s stubBalances) GetAccountBalanceForUpdate(_ context.Context, _ uint64, _ string) (string, error) {
	if s.value == "" {
		return "1000", s.err
	}
	return s.value, s.err
}

type stubDepositAddresses struct {
	items []DepositAddress
	item  DepositAddress
	err   error
}

func (s stubDepositAddresses) ListByUser(_ context.Context, _ uint64) ([]DepositAddress, error) {
	return s.items, s.err
}

func (s stubDepositAddresses) GetByUserChainAsset(_ context.Context, userID uint64, chainID int64, asset string) (DepositAddress, error) {
	if s.err != nil {
		return DepositAddress{}, s.err
	}
	if s.item.UserID == userID && s.item.ChainID == chainID && s.item.Asset == asset {
		return s.item, nil
	}
	return DepositAddress{}, errorsx.ErrNotFound
}

func (s stubDepositAddresses) Upsert(_ context.Context, _ DepositAddress) error {
	return s.err
}

type stubAllocator struct {
	address string
	valid   bool
	err     error
}

func (s stubAllocator) Allocate(_ context.Context, _ uint64, _ int64, _ string) (string, error) {
	if s.address == "" {
		return "0x00000000000000000000000000000000000000ab", s.err
	}
	return s.address, s.err
}

func (s stubAllocator) Validate(_ context.Context, _ uint64, _ int64, _ string, _ string) (string, bool, error) {
	if s.address == "" {
		return "0x00000000000000000000000000000000000000ab", s.valid, s.err
	}
	return s.address, s.valid, s.err
}

type stubWithdrawRiskEvaluator struct {
	decision WithdrawDecision
	err      error
}

func (s stubWithdrawRiskEvaluator) Evaluate(_ context.Context, _ WithdrawRiskInput) (WithdrawDecision, error) {
	if s.err != nil {
		return WithdrawDecision{}, s.err
	}
	return s.decision, nil
}

func TestConfirmDeposit_Success(t *testing.T) {
	ledger := &stubLedger{}
	deposits := &stubDepositRepo{deposit: DepositChainTx{
		DepositID: "dep_1", UserID: 7, Asset: "USDC", Amount: "100", Status: StatusCreditReady,
	}}
	svc := NewService(
		deposits,
		&stubWithdrawRepo{},
		&stubTransferResolver{},
		ledger,
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"ldg_1", "evt_1"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)

	err := svc.ConfirmDeposit(context.Background(), ConfirmDepositInput{
		DepositID:      "dep_1",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if deposits.credit.ledgerTxID != "ldg_1" {
		t.Fatalf("expected credited ledger tx")
	}
	if len(ledger.req.Entries) != 2 {
		t.Fatalf("expected ledger posting")
	}
}

func TestDetectDeposit_Success(t *testing.T) {
	ledger := &stubLedger{}
	deposits := &stubDepositRepo{getErr: errorsx.ErrNotFound}
	svc := NewService(
		deposits,
		&stubWithdrawRepo{},
		&stubTransferResolver{},
		ledger,
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"dep_1", "ldg_1", "evt_1"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)

	deposit, err := svc.DetectDeposit(context.Background(), DetectDepositInput{
		UserID:         7,
		ChainID:        8453,
		TxHash:         "0xabc",
		LogIndex:       1,
		FromAddress:    "0x1",
		ToAddress:      "0x2",
		TokenAddress:   "0x3",
		Amount:         "100",
		Asset:          "USDC",
		BlockNumber:    100,
		Confirmations:  1,
		RequiredConfs:  20,
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("detect deposit: %v", err)
	}
	if deposit.Status != StatusConfirming {
		t.Fatalf("expected confirming, got %s", deposit.Status)
	}
	if ledger.req.LedgerTx.BizType != "DEPOSIT_DETECTED" {
		t.Fatalf("unexpected biz type: %s", ledger.req.LedgerTx.BizType)
	}
}

func TestRequestWithdraw_Success(t *testing.T) {
	withdraws := &stubWithdrawRepo{}
	ledger := &stubLedger{}
	svc := NewService(
		&stubDepositRepo{},
		withdraws,
		&stubTransferResolver{},
		ledger,
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"wd_1", "ldg_1", "evt_1"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)

	wd, err := svc.RequestWithdraw(context.Background(), RequestWithdrawInput{
		UserID:         7,
		ChainID:        8453,
		Asset:          "USDC",
		Amount:         "100",
		FeeAmount:      "1",
		ToAddress:      "0x0000000000000000000000000000000000000001",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if wd.WithdrawID != "wd_1" || withdraws.created.WithdrawID != "wd_1" {
		t.Fatalf("expected withdraw persisted")
	}
}

func TestRequestWithdraw_AutoApprovedByRiskEvaluator(t *testing.T) {
	withdraws := &stubWithdrawRepo{}
	svc := NewService(
		&stubDepositRepo{},
		withdraws,
		&stubTransferResolver{},
		&stubLedger{},
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"wd_1", "ldg_1", "evt_1"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)
	svc.SetWithdrawRiskEvaluator(stubWithdrawRiskEvaluator{decision: WithdrawDecision{Status: StatusApproved}})

	wd, err := svc.RequestWithdraw(context.Background(), RequestWithdrawInput{
		UserID:         7,
		ChainID:        31337,
		Asset:          "USDC",
		Amount:         "100",
		FeeAmount:      "1",
		ToAddress:      "0x0000000000000000000000000000000000000001",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if wd.Status != StatusApproved {
		t.Fatalf("expected approved status, got %s", wd.Status)
	}
}

func TestRequestWithdraw_RiskReviewByRiskEvaluator(t *testing.T) {
	withdraws := &stubWithdrawRepo{}
	svc := NewService(
		&stubDepositRepo{},
		withdraws,
		&stubTransferResolver{},
		&stubLedger{},
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"wd_1", "ldg_1", "evt_1"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)
	svc.SetWithdrawRiskEvaluator(stubWithdrawRiskEvaluator{decision: WithdrawDecision{Status: StatusRiskReview, RiskFlag: "manual_review_threshold"}})

	wd, err := svc.RequestWithdraw(context.Background(), RequestWithdrawInput{
		UserID:         7,
		ChainID:        31337,
		Asset:          "USDC",
		Amount:         "100",
		FeeAmount:      "1",
		ToAddress:      "0x0000000000000000000000000000000000000001",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if wd.Status != StatusRiskReview {
		t.Fatalf("expected risk review status, got %s", wd.Status)
	}
}

func TestTransfer_RejectsInvalidInput(t *testing.T) {
	svc := NewService(
		&stubDepositRepo{},
		&stubWithdrawRepo{},
		&stubTransferResolver{},
		&stubLedger{},
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)
	err := svc.Transfer(context.Background(), TransferRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMarkWithdrawBroadcasted_Success(t *testing.T) {
	withdraws := &stubWithdrawRepo{withdraw: WithdrawRequest{
		WithdrawID: "wd_1",
		UserID:     7,
		Asset:      "USDC",
		Amount:     "100",
		FeeAmount:  "1",
		Status:     StatusApproved,
	}}
	ledger := &stubLedger{}
	svc := NewService(
		&stubDepositRepo{},
		withdraws,
		&stubTransferResolver{},
		ledger,
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"ldg_1", "evt_1"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)

	err := svc.MarkWithdrawBroadcasted(context.Background(), BroadcastWithdrawInput{
		WithdrawID:     "wd_1",
		TxHash:         "0xabc",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(withdraws.broadcasts) != 1 {
		t.Fatalf("expected withdraw marked broadcasted")
	}
}

func TestMarkWithdrawBroadcasted_RejectsWithoutApproval(t *testing.T) {
	withdraws := &stubWithdrawRepo{withdraw: WithdrawRequest{
		WithdrawID: "wd_1",
		UserID:     7,
		Asset:      "USDC",
		Amount:     "100",
		FeeAmount:  "1",
		Status:     StatusHold,
	}}
	svc := NewService(
		&stubDepositRepo{},
		withdraws,
		&stubTransferResolver{},
		&stubLedger{},
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"ldg_1", "evt_1"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)

	err := svc.MarkWithdrawBroadcasted(context.Background(), BroadcastWithdrawInput{
		WithdrawID:     "wd_1",
		TxHash:         "0xabc",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if !errors.Is(err, errorsx.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestRefundWithdraw_Success(t *testing.T) {
	withdraws := &stubWithdrawRepo{withdraw: WithdrawRequest{
		WithdrawID: "wd_1",
		UserID:     7,
		Asset:      "USDC",
		Amount:     "100",
		Status:     StatusHold,
	}}
	ledger := &stubLedger{}
	svc := NewService(
		&stubDepositRepo{},
		withdraws,
		&stubTransferResolver{},
		ledger,
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"ldg_1", "evt_1"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)

	err := svc.RefundWithdraw(context.Background(), RefundWithdrawInput{
		WithdrawID:     "wd_1",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(withdraws.refunds) != 1 {
		t.Fatalf("expected withdraw marked refunded")
	}
}

func TestRefundWithdraw_Broadcasted_ReversesTransitBeforeRefund(t *testing.T) {
	withdraws := &stubWithdrawRepo{withdraw: WithdrawRequest{
		WithdrawID: "wd_1",
		UserID:     7,
		Asset:      "USDC",
		Amount:     "100",
		FeeAmount:  "1",
		Status:     StatusBroadcasted,
	}}
	ledger := &stubLedger{}
	svc := NewService(
		&stubDepositRepo{},
		withdraws,
		&stubTransferResolver{},
		ledger,
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"ldg_1", "evt_1", "ldg_2", "evt_2"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)

	err := svc.RefundWithdraw(context.Background(), RefundWithdrawInput{
		WithdrawID:     "wd_1",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(ledger.reqs) != 2 {
		t.Fatalf("expected two ledger postings, got %d", len(ledger.reqs))
	}
	if ledger.reqs[0].LedgerTx.BizType != "WITHDRAW_REFUND_REVERSAL" {
		t.Fatalf("expected reversal first, got %s", ledger.reqs[0].LedgerTx.BizType)
	}
	if ledger.reqs[1].LedgerTx.BizType != "WITHDRAW_REFUND" {
		t.Fatalf("expected refund second, got %s", ledger.reqs[1].LedgerTx.BizType)
	}
	if got := ledger.reqs[0].Entries[0].Amount; got != "100" {
		t.Fatalf("expected hold restore amount 100, got %s", got)
	}
	if got := ledger.reqs[0].Entries[1].Amount; got != "-99" {
		t.Fatalf("expected in-transit reversal amount -99, got %s", got)
	}
	if got := ledger.reqs[0].Entries[2].Amount; got != "-1" {
		t.Fatalf("expected fee reversal amount -1, got %s", got)
	}
}

func TestCompleteWithdraw_Success(t *testing.T) {
	withdraws := &stubWithdrawRepo{withdraw: WithdrawRequest{
		WithdrawID: "wd_1",
		UserID:     7,
		Asset:      "USDC",
		Amount:     "100",
		FeeAmount:  "1",
		Status:     StatusBroadcasted,
	}}
	ledger := &stubLedger{}
	svc := NewService(
		&stubDepositRepo{},
		withdraws,
		&stubTransferResolver{},
		ledger,
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"ldg_1", "evt_1"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)

	if err := svc.CompleteWithdraw(context.Background(), CompleteWithdrawInput{
		WithdrawID:     "wd_1",
		TxHash:         "0xabc",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	}); err != nil {
		t.Fatalf("complete withdraw: %v", err)
	}
	if withdraws.withdraw.Status != StatusCompleted {
		t.Fatalf("expected completed status, got %s", withdraws.withdraw.Status)
	}
}

func TestApproveWithdraw_Success(t *testing.T) {
	withdraws := &stubWithdrawRepo{withdraw: WithdrawRequest{
		WithdrawID: "wd_1",
		UserID:     7,
		Asset:      "USDC",
		Status:     StatusHold,
	}}
	svc := NewService(
		&stubDepositRepo{},
		withdraws,
		&stubTransferResolver{},
		&stubLedger{},
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
	)

	if err := svc.ApproveWithdraw(context.Background(), ApproveWithdrawInput{
		WithdrawID:     "wd_1",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	}); err != nil {
		t.Fatalf("approve withdraw: %v", err)
	}
	if withdraws.withdraw.Status != StatusApproved {
		t.Fatalf("expected approved status, got %s", withdraws.withdraw.Status)
	}
}

func TestRequestWithdraw_RejectsInsufficientBalance(t *testing.T) {
	svc := NewService(
		&stubDepositRepo{},
		&stubWithdrawRepo{},
		&stubTransferResolver{},
		&stubLedger{},
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"wd_1"}},
		stubAccounts{},
		stubBalances{value: "1"},
		stubDepositAddresses{},
	)

	_, err := svc.RequestWithdraw(context.Background(), RequestWithdrawInput{
		UserID:         7,
		ChainID:        8453,
		Asset:          "USDC",
		Amount:         "100",
		FeeAmount:      "1",
		ToAddress:      "0x0000000000000000000000000000000000000001",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err == nil {
		t.Fatal("expected insufficient balance error")
	}
}

func TestGenerateDepositAddress_Success(t *testing.T) {
	svc := NewService(
		&stubDepositRepo{},
		&stubWithdrawRepo{},
		&stubTransferResolver{},
		&stubLedger{},
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"unused"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		stubDepositAddresses{},
		stubAllocator{address: "0x00000000000000000000000000000000000000ab"},
	)

	result, err := svc.GenerateDepositAddress(context.Background(), GenerateDepositAddressInput{
		UserID:  7,
		ChainID: 31337,
		Asset:   "USDC",
	})
	if err != nil {
		t.Fatalf("generate deposit address: %v", err)
	}
	if result.Address != "0x00000000000000000000000000000000000000ab" {
		t.Fatalf("unexpected generated address: %+v", result)
	}
}

func TestGenerateDepositAddress_ReplacesInvalidStoredAddress(t *testing.T) {
	addresses := stubDepositAddresses{
		item: DepositAddress{
			UserID:  7,
			ChainID: 31337,
			Asset:   "USDC",
			Address: "0x00000000000000000000000000000000000000cd",
			Status:  "ACTIVE",
		},
	}
	svc := NewService(
		&stubDepositRepo{},
		&stubWithdrawRepo{},
		&stubTransferResolver{},
		&stubLedger{},
		stubTxManager{},
		fakeClock{now: time.Now()},
		&fakeIDGen{values: []string{"unused"}},
		stubAccounts{},
		stubBalances{value: "1000"},
		addresses,
		stubAllocator{address: "0x00000000000000000000000000000000000000ab", valid: false},
	)

	result, err := svc.GenerateDepositAddress(context.Background(), GenerateDepositAddressInput{
		UserID:  7,
		ChainID: 31337,
		Asset:   "USDC",
	})
	if err != nil {
		t.Fatalf("generate deposit address: %v", err)
	}
	if result.Address != "0x00000000000000000000000000000000000000ab" {
		t.Fatalf("expected canonical address rewrite, got %+v", result)
	}
}

type concurrentBalanceState struct {
	mu       sync.Mutex
	balances map[uint64]string
}

func (s *concurrentBalanceState) GetAccountBalanceForUpdate(_ context.Context, accountID uint64, _ string) (string, error) {
	if value, ok := s.balances[accountID]; ok {
		return value, nil
	}
	return "0", nil
}

type transactionalLedger struct {
	state *concurrentBalanceState
}

func (l *transactionalLedger) Post(_ context.Context, req ledgerdomain.PostingRequest) error {
	for _, entry := range req.Entries {
		current := "0"
		if value, ok := l.state.balances[entry.AccountID]; ok {
			current = value
		}
		next, err := addDecimalStrings(current, entry.Amount)
		if err != nil {
			return err
		}
		l.state.balances[entry.AccountID] = next
	}
	return nil
}

type lockingTxManager struct {
	mu *sync.Mutex
}

func (m lockingTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(ctx)
}

func addDecimalStrings(left string, right string) (string, error) {
	lhs, err := decimalx.NewFromString(left)
	if err != nil {
		return "", err
	}
	rhs, err := decimalx.NewFromString(right)
	if err != nil {
		return "", err
	}
	return lhs.Add(rhs).String(), nil
}

func TestRequestWithdraw_ConcurrentDoesNotOverspend(t *testing.T) {
	sharedLock := &sync.Mutex{}
	state := &concurrentBalanceState{balances: map[uint64]string{71: "100"}}
	svc := NewService(
		&stubDepositRepo{},
		&stubWithdrawRepo{},
		&stubTransferResolver{},
		&transactionalLedger{state: state},
		lockingTxManager{mu: sharedLock},
		fakeClock{now: time.Now()},
		&safeIDGen{values: []string{"wd_1", "ldg_1", "evt_1", "wd_2", "ldg_2", "evt_2"}},
		stubAccounts{},
		state,
		stubDepositAddresses{},
	)

	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := svc.RequestWithdraw(context.Background(), RequestWithdrawInput{
				UserID:         7,
				ChainID:        8453,
				Asset:          "USDC",
				Amount:         "80",
				FeeAmount:      "1",
				ToAddress:      "0x0000000000000000000000000000000000000001",
				IdempotencyKey: "idem_" + time.Now().String(),
				TraceID:        "trace",
			})
			results <- err
		}()
	}

	var successCount int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successCount++
			continue
		}
		if !errors.Is(err, errorsx.ErrForbidden) {
			t.Fatalf("expected insufficient balance error, got %v", err)
		}
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one successful withdraw, got %d", successCount)
	}
	if got := state.balances[71]; got != "20" {
		t.Fatalf("expected wallet balance 20, got %s", got)
	}
	if got := state.balances[72]; got != "80" {
		t.Fatalf("expected hold balance 80, got %s", got)
	}
}

func TestTransfer_ConcurrentDoesNotOverspend(t *testing.T) {
	sharedLock := &sync.Mutex{}
	state := &concurrentBalanceState{balances: map[uint64]string{71: "100", 81: "0"}}
	svc := NewService(
		&stubDepositRepo{},
		&stubWithdrawRepo{},
		&stubTransferResolver{},
		&transactionalLedger{state: state},
		lockingTxManager{mu: sharedLock},
		fakeClock{now: time.Now()},
		&safeIDGen{values: []string{"ldg_1", "evt_1", "ldg_2", "evt_2"}},
		stubAccounts{},
		state,
		stubDepositAddresses{},
	)

	results := make(chan error, 2)
	transferIDs := []string{"trf_1", "trf_2"}
	for i := 0; i < 2; i++ {
		transferID := transferIDs[i]
		go func(id string) {
			err := svc.Transfer(context.Background(), TransferRequest{
				TransferID: id,
				FromUserID: 7,
				ToUserID:   8,
				Asset:      "USDC",
				Amount:     "80",
				TraceID:    "trace",
			})
			results <- err
		}(transferID)
	}

	var successCount int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successCount++
			continue
		}
		if !errors.Is(err, errorsx.ErrForbidden) {
			t.Fatalf("expected insufficient balance error, got %v", err)
		}
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one successful transfer, got %d", successCount)
	}
	if got := state.balances[71]; got != "20" {
		t.Fatalf("expected sender balance 20, got %s", got)
	}
	if got := state.balances[81]; got != "80" {
		t.Fatalf("expected receiver balance 80, got %s", got)
	}
}

var _ = errors.New
