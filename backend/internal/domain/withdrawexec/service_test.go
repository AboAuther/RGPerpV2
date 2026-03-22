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
	items       []walletdomain.WithdrawRequest
	byID        map[string]walletdomain.WithdrawRequest
	updateCalls []struct {
		withdrawID string
		from       []string
		to         string
	}
	updateErr error
}

func (s *stubWithdrawRepository) GetByID(_ context.Context, withdrawID string) (walletdomain.WithdrawRequest, error) {
	item, ok := s.byID[withdrawID]
	if !ok {
		return walletdomain.WithdrawRequest{}, errorsx.ErrNotFound
	}
	return item, nil
}

func (s *stubWithdrawRepository) ListByChainStatuses(_ context.Context, chainID int64, statuses []string, _ int) ([]walletdomain.WithdrawRequest, error) {
	allowed := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		allowed[status] = struct{}{}
	}
	out := make([]walletdomain.WithdrawRequest, 0, len(s.items))
	for _, item := range s.items {
		if item.ChainID != chainID {
			continue
		}
		if _, ok := allowed[item.Status]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *stubWithdrawRepository) UpdateStatus(_ context.Context, withdrawID string, from []string, to string) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updateCalls = append(s.updateCalls, struct {
		withdrawID string
		from       []string
		to         string
	}{withdrawID: withdrawID, from: from, to: to})
	item := s.byID[withdrawID]
	allowed := len(from) == 0
	for _, status := range from {
		if item.Status == status {
			allowed = true
			break
		}
	}
	if !allowed {
		return errorsx.ErrConflict
	}
	item.Status = to
	s.byID[withdrawID] = item
	for idx := range s.items {
		if s.items[idx].WithdrawID == withdrawID {
			s.items[idx].Status = to
		}
	}
	return nil
}

type stubWallet struct {
	inputs []walletdomain.BroadcastWithdrawInput
	err    error
}

func (s *stubWallet) MarkWithdrawBroadcasted(_ context.Context, input walletdomain.BroadcastWithdrawInput) error {
	if s.err != nil {
		return s.err
	}
	s.inputs = append(s.inputs, input)
	return nil
}

type stubExecutor struct {
	txHash string
	err    error
}

func (s stubExecutor) ExecuteWithdrawal(_ context.Context, _ int64, _ string, _ string, _ string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.txHash, nil
}

func TestProcessChain_BroadcastsApprovedWithdraw(t *testing.T) {
	repo := &stubWithdrawRepository{
		items: []walletdomain.WithdrawRequest{{
			WithdrawID: "wd_1",
			ChainID:    31337,
			Amount:     "100",
			FeeAmount:  "1",
			ToAddress:  "0x0000000000000000000000000000000000000001",
			Status:     walletdomain.StatusApproved,
		}},
		byID: map[string]walletdomain.WithdrawRequest{
			"wd_1": {
				WithdrawID: "wd_1",
				ChainID:    31337,
				Amount:     "100",
				FeeAmount:  "1",
				ToAddress:  "0x0000000000000000000000000000000000000001",
				Status:     walletdomain.StatusApproved,
			},
		},
	}
	wallet := &stubWallet{}
	service, err := NewService(repo, wallet, stubExecutor{txHash: "0xabc"})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := service.ProcessChain(context.Background(), 31337, 10); err != nil {
		t.Fatalf("process chain: %v", err)
	}
	if len(wallet.inputs) != 1 {
		t.Fatalf("expected broadcast mark once, got %d", len(wallet.inputs))
	}
	if wallet.inputs[0].TxHash != "0xabc" {
		t.Fatalf("unexpected tx hash: %s", wallet.inputs[0].TxHash)
	}
}

func TestProcessChain_EscalatesToRiskReviewOnReviewRequired(t *testing.T) {
	repo := &stubWithdrawRepository{
		items: []walletdomain.WithdrawRequest{{
			WithdrawID: "wd_1",
			ChainID:    31337,
			Amount:     "100",
			FeeAmount:  "1",
			ToAddress:  "0x0000000000000000000000000000000000000001",
			Status:     walletdomain.StatusApproved,
		}},
		byID: map[string]walletdomain.WithdrawRequest{
			"wd_1": {
				WithdrawID: "wd_1",
				ChainID:    31337,
				Amount:     "100",
				FeeAmount:  "1",
				ToAddress:  "0x0000000000000000000000000000000000000001",
				Status:     walletdomain.StatusApproved,
			},
		},
	}
	service, err := NewService(repo, &stubWallet{}, stubExecutor{err: chaininfra.ReviewRequiredError{Reason: "chain_unavailable"}})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := service.ProcessChain(context.Background(), 31337, 10); err != nil {
		t.Fatalf("process chain: %v", err)
	}
	if got := repo.byID["wd_1"].Status; got != walletdomain.StatusRiskReview {
		t.Fatalf("expected risk review status, got %s", got)
	}
}

func TestNewService_RequiresDependencies(t *testing.T) {
	_, err := NewService(nil, nil, nil)
	if !errors.Is(err, errorsx.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
