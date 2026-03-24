package chain

import (
	"context"
	"testing"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/config"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
)

type stubHealthChecker struct {
	healthErr  error
	balance    string
	balanceErr error
	dailyTotal string
	dailyErr   error
}

func (s stubHealthChecker) CheckChainHealth(_ context.Context, _ int64) error {
	return s.healthErr
}

func (s stubHealthChecker) VaultBalance(_ context.Context, _ int64) (string, error) {
	return s.balance, s.balanceErr
}

func (s stubHealthChecker) SumRequestedAmountByUserSince(_ context.Context, _ uint64, _ string, _ time.Time) (string, error) {
	if s.dailyTotal == "" {
		return "0", s.dailyErr
	}
	return s.dailyTotal, s.dailyErr
}

func TestWithdrawRiskEvaluator_AutoApprovesLowRisk(t *testing.T) {
	evaluator := NewWithdrawRiskEvaluator(config.GlobalRuntimeConfig{}, config.WalletRuntimeConfig{
		WithdrawCircuitMode:           "NORMAL",
		WithdrawManualReviewThreshold: "10000",
		WithdrawDailyLimitPerUser:     "50000",
		HotWalletMinBalance:           "0",
	}, stubHealthChecker{balance: "50000"}, stubHealthChecker{dailyTotal: "0"})

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
		WithdrawDailyLimitPerUser:     "50000",
		HotWalletMinBalance:           "0",
	}, stubHealthChecker{balance: "50000"}, stubHealthChecker{dailyTotal: "0"})

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
		WithdrawDailyLimitPerUser:     "50000",
		HotWalletMinBalance:           "0",
	}, stubHealthChecker{healthErr: ReviewRequiredError{Reason: "chain_unavailable"}}, stubHealthChecker{dailyTotal: "0"})

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

func TestWithdrawRiskEvaluator_EntersRiskReviewWhenDailyLimitExceeded(t *testing.T) {
	evaluator := NewWithdrawRiskEvaluator(config.GlobalRuntimeConfig{}, config.WalletRuntimeConfig{
		WithdrawCircuitMode:           "NORMAL",
		WithdrawManualReviewThreshold: "10000",
		WithdrawDailyLimitPerUser:     "500",
		HotWalletMinBalance:           "0",
	}, stubHealthChecker{balance: "50000"}, stubHealthChecker{dailyTotal: "450"})

	decision, err := evaluator.Evaluate(context.Background(), walletdomain.WithdrawRiskInput{
		ChainID:   31337,
		Amount:    "100",
		FeeAmount: "1",
		Asset:     "USDC",
		UserID:    7,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.Status != walletdomain.StatusRiskReview || decision.RiskFlag != "daily_limit_per_user" {
		t.Fatalf("expected daily limit risk review, got status=%s flag=%s", decision.Status, decision.RiskFlag)
	}
}

func TestWithdrawRiskEvaluator_EntersRiskReviewWhenVaultCannotCoverNetAmount(t *testing.T) {
	evaluator := NewWithdrawRiskEvaluator(config.GlobalRuntimeConfig{}, config.WalletRuntimeConfig{
		WithdrawCircuitMode:           "NORMAL",
		WithdrawManualReviewThreshold: "10000",
		WithdrawDailyLimitPerUser:     "50000",
		HotWalletMinBalance:           "0",
	}, stubHealthChecker{balance: "98"}, stubHealthChecker{dailyTotal: "0"})

	decision, err := evaluator.Evaluate(context.Background(), walletdomain.WithdrawRiskInput{
		ChainID:   31337,
		Amount:    "100",
		FeeAmount: "1",
		Asset:     "USDC",
		UserID:    7,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if decision.Status != walletdomain.StatusRiskReview || decision.RiskFlag != "hot_wallet_insufficient_balance" {
		t.Fatalf("expected hot wallet insufficiency risk review, got status=%s flag=%s", decision.Status, decision.RiskFlag)
	}
}
