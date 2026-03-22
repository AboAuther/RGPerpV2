package chain

import (
	"context"
	"testing"

	"github.com/xiaobao/rgperp/backend/internal/config"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
)

type stubHealthChecker struct {
	healthErr  error
	balance    string
	balanceErr error
}

func (s stubHealthChecker) CheckChainHealth(_ context.Context, _ int64) error {
	return s.healthErr
}

func (s stubHealthChecker) VaultBalance(_ context.Context, _ int64) (string, error) {
	return s.balance, s.balanceErr
}

func TestWithdrawRiskEvaluator_AutoApprovesLowRisk(t *testing.T) {
	evaluator := NewWithdrawRiskEvaluator(config.GlobalRuntimeConfig{}, config.WalletRuntimeConfig{
		WithdrawCircuitMode:           "NORMAL",
		WithdrawManualReviewThreshold: "10000",
		HotWalletMinBalance:           "0",
	}, stubHealthChecker{balance: "50000"})

	decision, err := evaluator.Evaluate(context.Background(), walletdomain.WithdrawRiskInput{
		ChainID:    31337,
		Amount:     "100",
		FeeAmount:  "1",
		ToAddress:  "0x0000000000000000000000000000000000000001",
		Asset:      "USDC",
		UserID:     7,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.Status != walletdomain.StatusApproved {
		t.Fatalf("expected approved, got %s", decision.Status)
	}
}

func TestWithdrawRiskEvaluator_EntersRiskReviewWhenThresholdExceeded(t *testing.T) {
	evaluator := NewWithdrawRiskEvaluator(config.GlobalRuntimeConfig{}, config.WalletRuntimeConfig{
		WithdrawCircuitMode:           "NORMAL",
		WithdrawManualReviewThreshold: "1000",
		HotWalletMinBalance:           "0",
	}, stubHealthChecker{balance: "50000"})

	decision, err := evaluator.Evaluate(context.Background(), walletdomain.WithdrawRiskInput{
		ChainID:   31337,
		Amount:    "1001",
		FeeAmount: "1",
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.Status != walletdomain.StatusRiskReview {
		t.Fatalf("expected risk review, got %s", decision.Status)
	}
}

func TestWithdrawRiskEvaluator_EntersRiskReviewWhenChainUnavailable(t *testing.T) {
	evaluator := NewWithdrawRiskEvaluator(config.GlobalRuntimeConfig{}, config.WalletRuntimeConfig{
		WithdrawCircuitMode:           "NORMAL",
		WithdrawManualReviewThreshold: "10000",
		HotWalletMinBalance:           "0",
	}, stubHealthChecker{healthErr: ReviewRequiredError{Reason: "chain_unavailable"}})

	decision, err := evaluator.Evaluate(context.Background(), walletdomain.WithdrawRiskInput{
		ChainID:   31337,
		Amount:    "100",
		FeeAmount: "1",
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.Status != walletdomain.StatusRiskReview {
		t.Fatalf("expected risk review, got %s", decision.Status)
	}
}
