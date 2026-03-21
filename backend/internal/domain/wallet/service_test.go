package wallet

import (
	"context"
	"errors"
	"testing"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
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

type stubDepositRepo struct {
	deposit DepositChainTx
	credit  struct {
		id         string
		ledgerTxID string
	}
	err error
}

func (s *stubDepositRepo) GetByID(_ context.Context, _ string) (DepositChainTx, error) {
	return s.deposit, s.err
}
func (s *stubDepositRepo) MarkCredited(_ context.Context, depositID string, ledgerTxID string) error {
	s.credit.id = depositID
	s.credit.ledgerTxID = ledgerTxID
	return s.err
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
func (s *stubWithdrawRepo) MarkRefunded(_ context.Context, withdrawID string) error {
	s.refunds = append(s.refunds, withdrawID)
	return s.err
}

type stubTransferResolver struct {
	userID uint64
	err    error
}

func (s *stubTransferResolver) ResolveUserIDByAddress(_ context.Context, _ string) (uint64, error) {
	return s.userID, s.err
}

type stubLedger struct {
	req ledgerdomain.PostingRequest
	err error
}

func (s *stubLedger) Post(_ context.Context, req ledgerdomain.PostingRequest) error {
	s.req = req
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

var _ = errors.New
