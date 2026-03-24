package withdrawexec

import (
	"context"
	"errors"
	"testing"

	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	chaininfra "github.com/xiaobao/rgperp/backend/internal/infra/chain"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type stubWithdrawRepository struct {
	reserveQueue []walletdomain.WithdrawRequest
	retryItems   []walletdomain.WithdrawRequest
	byID         map[string]walletdomain.WithdrawRequest
}

func (s *stubWithdrawRepository) GetByID(_ context.Context, withdrawID string) (walletdomain.WithdrawRequest, error) {
	item, ok := s.byID[withdrawID]
	if !ok {
		return walletdomain.WithdrawRequest{}, errorsx.ErrNotFound
	}
	return item, nil
}

func (s *stubWithdrawRepository) ListByChainStatuses(_ context.Context, _ int64, _ []string, _ int) ([]walletdomain.WithdrawRequest, error) {
	return nil, nil
}

func (s *stubWithdrawRepository) UpdateStatus(_ context.Context, withdrawID string, _ []string, to string) error {
	item, ok := s.byID[withdrawID]
	if !ok {
		return errorsx.ErrNotFound
	}
	item.Status = to
	s.byID[withdrawID] = item
	return nil
}

func (s *stubWithdrawRepository) ReserveNextNonce(_ context.Context, chainID int64, _ string, chainPendingNonce uint64) (walletdomain.WithdrawRequest, error) {
	if len(s.reserveQueue) == 0 {
		return walletdomain.WithdrawRequest{}, errorsx.ErrNotFound
	}
	item := s.reserveQueue[0]
	s.reserveQueue = s.reserveQueue[1:]
	if item.ChainID != chainID {
		return walletdomain.WithdrawRequest{}, errorsx.ErrConflict
	}
	if item.BroadcastNonce == nil {
		nonce := chainPendingNonce
		item.BroadcastNonce = &nonce
	}
	item.Status = walletdomain.StatusSigning
	s.byID[item.WithdrawID] = item
	return item, nil
}

func (s *stubWithdrawRepository) ListReservedForBroadcastRetry(_ context.Context, chainID int64, _ int) ([]walletdomain.WithdrawRequest, error) {
	out := make([]walletdomain.WithdrawRequest, 0, len(s.retryItems))
	for _, item := range s.retryItems {
		if item.ChainID == chainID {
			out = append(out, item)
		}
	}
	return out, nil
}

type stubWallet struct {
	inputs      []walletdomain.BroadcastWithdrawInput
	failInputs  []walletdomain.FailWithdrawInput
	err         error
}

func (s *stubWallet) MarkWithdrawBroadcasted(_ context.Context, input walletdomain.BroadcastWithdrawInput) error {
	if s.err != nil {
		return s.err
	}
	s.inputs = append(s.inputs, input)
	return nil
}

func (s *stubWallet) FailWithdraw(_ context.Context, input walletdomain.FailWithdrawInput) error {
	if s.err != nil {
		return s.err
	}
	s.failInputs = append(s.failInputs, input)
	return nil
}

type stubExecutor struct {
	signer       string
	nextNonce    uint64
	nextNonceErr error
	txHash       string
	err          error
	calls        []struct {
		withdrawID string
		nonce      uint64
	}
}

func (s *stubExecutor) SignerAddress() string {
	if s.signer == "" {
		return "0x00000000000000000000000000000000000000aa"
	}
	return s.signer
}

func (s *stubExecutor) NextNonce(_ context.Context, _ int64) (uint64, error) {
	if s.nextNonceErr != nil {
		return 0, s.nextNonceErr
	}
	return s.nextNonce, nil
}

func (s *stubExecutor) ExecuteWithdrawal(_ context.Context, _ int64, _ string, _ string, withdrawID string, nonce uint64) (string, error) {
	s.calls = append(s.calls, struct {
		withdrawID string
		nonce      uint64
	}{withdrawID: withdrawID, nonce: nonce})
	if s.err != nil {
		return "", s.err
	}
	return s.txHash, nil
}

type stubTxManager struct{}

func (stubTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

func TestProcessChain_BroadcastsReservedNonce(t *testing.T) {
	repo := &stubWithdrawRepository{
		reserveQueue: []walletdomain.WithdrawRequest{{
			WithdrawID: "wd_1",
			ChainID:    31337,
			Amount:     "100",
			FeeAmount:  "1",
			ToAddress:  "0x0000000000000000000000000000000000000001",
			Status:     walletdomain.StatusApproved,
		}},
		byID: map[string]walletdomain.WithdrawRequest{},
	}
	wallet := &stubWallet{}
	executor := &stubExecutor{nextNonce: 42, txHash: "0xabc"}
	service, err := NewService(repo, wallet, executor, stubTxManager{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := service.ProcessChain(context.Background(), 31337, 10); err != nil {
		t.Fatalf("process chain: %v", err)
	}
	if len(wallet.inputs) != 1 {
		t.Fatalf("expected broadcast mark once, got %d", len(wallet.inputs))
	}
	if wallet.inputs[0].BroadcastNonce == nil || *wallet.inputs[0].BroadcastNonce != 42 {
		t.Fatalf("expected broadcast nonce 42, got %+v", wallet.inputs[0].BroadcastNonce)
	}
	if len(executor.calls) != 1 || executor.calls[0].nonce != 42 {
		t.Fatalf("expected executor call with nonce 42, got %+v", executor.calls)
	}
}

func TestProcessChain_RetriesReservedSigningBeforeAllocatingNewNonce(t *testing.T) {
	retryNonce := uint64(7)
	repo := &stubWithdrawRepository{
		retryItems: []walletdomain.WithdrawRequest{{
			WithdrawID:     "wd_retry",
			ChainID:        31337,
			Amount:         "100",
			FeeAmount:      "1",
			ToAddress:      "0x0000000000000000000000000000000000000001",
			Status:         walletdomain.StatusSigning,
			BroadcastNonce: &retryNonce,
		}},
		reserveQueue: []walletdomain.WithdrawRequest{{
			WithdrawID: "wd_new",
			ChainID:    31337,
			Amount:     "50",
			FeeAmount:  "1",
			ToAddress:  "0x0000000000000000000000000000000000000002",
			Status:     walletdomain.StatusApproved,
		}},
		byID: map[string]walletdomain.WithdrawRequest{},
	}
	wallet := &stubWallet{}
	executor := &stubExecutor{nextNonce: 8, txHash: "0xabc"}
	service, err := NewService(repo, wallet, executor, stubTxManager{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := service.ProcessChain(context.Background(), 31337, 10); err != nil {
		t.Fatalf("process chain: %v", err)
	}
	if len(executor.calls) < 2 {
		t.Fatalf("expected retry and new broadcast calls, got %+v", executor.calls)
	}
	if executor.calls[0].withdrawID != "wd_retry" || executor.calls[0].nonce != 7 {
		t.Fatalf("expected retry first with nonce 7, got %+v", executor.calls[0])
	}
	if executor.calls[1].withdrawID != "wd_new" || executor.calls[1].nonce != 8 {
		t.Fatalf("expected new withdraw second with nonce 8, got %+v", executor.calls[1])
	}
}

func TestProcessChain_ReviewRequiredFailureMarksWithdrawFailed(t *testing.T) {
	repo := &stubWithdrawRepository{
		reserveQueue: []walletdomain.WithdrawRequest{{
			WithdrawID: "wd_1",
			ChainID:    31337,
			Amount:     "100",
			FeeAmount:  "1",
			ToAddress:  "0x0000000000000000000000000000000000000001",
			Status:     walletdomain.StatusApproved,
		}},
		byID: map[string]walletdomain.WithdrawRequest{},
	}
	wallet := &stubWallet{}
	executor := &stubExecutor{nextNonce: 12, err: chaininfra.ReviewRequiredError{Reason: "chain_unavailable"}}
	service, err := NewService(repo, wallet, executor, stubTxManager{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := service.ProcessChain(context.Background(), 31337, 10); err != nil {
		t.Fatalf("process chain: %v", err)
	}
	if len(wallet.failInputs) != 1 {
		t.Fatalf("expected fail input once, got %d", len(wallet.failInputs))
	}
	if wallet.failInputs[0].RiskFlag != "chain_unavailable" {
		t.Fatalf("expected chain_unavailable flag, got %s", wallet.failInputs[0].RiskFlag)
	}
}

func TestNewService_RequiresDependencies(t *testing.T) {
	_, err := NewService(nil, nil, nil, nil)
	if !errors.Is(err, errorsx.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
