package chain

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/xiaobao/rgperp/backend/internal/config"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
)

type WithdrawRiskEvaluator struct {
	wallet  config.WalletRuntimeConfig
	checker interface {
		CheckChainHealth(ctx context.Context, chainID int64) error
		VaultBalance(ctx context.Context, chainID int64) (string, error)
	}
}

func NewWithdrawRiskEvaluator(global config.GlobalRuntimeConfig, wallet config.WalletRuntimeConfig, checker interface {
	CheckChainHealth(ctx context.Context, chainID int64) error
	VaultBalance(ctx context.Context, chainID int64) (string, error)
}) *WithdrawRiskEvaluator {
	_ = global
	return &WithdrawRiskEvaluator{wallet: wallet, checker: checker}
}

func (e *WithdrawRiskEvaluator) Evaluate(ctx context.Context, input walletdomain.WithdrawRiskInput) (walletdomain.WithdrawDecision, error) {
	if e == nil {
		return walletdomain.WithdrawDecision{Status: walletdomain.StatusApproved}, nil
	}
	circuitMode := strings.ToUpper(strings.TrimSpace(e.wallet.WithdrawCircuitMode))
	if circuitMode != "" && circuitMode != "NORMAL" {
		return walletdomain.WithdrawDecision{Status: walletdomain.StatusRiskReview, RiskFlag: "withdraw_circuit_mode"}, nil
	}

	manualThreshold, err := decimal.NewFromString(e.wallet.WithdrawManualReviewThreshold)
	if err == nil {
		amount, amountErr := decimal.NewFromString(input.Amount)
		if amountErr == nil && amount.GreaterThan(manualThreshold) {
			return walletdomain.WithdrawDecision{Status: walletdomain.StatusRiskReview, RiskFlag: "manual_review_threshold"}, nil
		}
	}

	if e.checker == nil {
		return walletdomain.WithdrawDecision{Status: walletdomain.StatusRiskReview, RiskFlag: "withdraw_checker_unavailable"}, nil
	}
	if err := e.checker.CheckChainHealth(ctx, input.ChainID); err != nil {
		return walletdomain.WithdrawDecision{Status: walletdomain.StatusRiskReview, RiskFlag: "chain_unavailable"}, nil
	}
	vaultBalanceRaw, err := e.checker.VaultBalance(ctx, input.ChainID)
	if err != nil {
		return walletdomain.WithdrawDecision{Status: walletdomain.StatusRiskReview, RiskFlag: "hot_wallet_balance_unknown"}, nil
	}
	vaultBalance, err := decimal.NewFromString(vaultBalanceRaw)
	if err != nil {
		return walletdomain.WithdrawDecision{Status: walletdomain.StatusRiskReview, RiskFlag: "hot_wallet_balance_invalid"}, nil
	}
	amount, err := decimal.NewFromString(input.Amount)
	if err != nil {
		return walletdomain.WithdrawDecision{}, err
	}
	fee, err := decimal.NewFromString(input.FeeAmount)
	if err != nil {
		return walletdomain.WithdrawDecision{}, err
	}
	netAmount := amount.Sub(fee)
	minBalance, err := decimal.NewFromString(e.wallet.HotWalletMinBalance)
	if err == nil && vaultBalance.Sub(netAmount).LessThan(minBalance) {
		return walletdomain.WithdrawDecision{Status: walletdomain.StatusRiskReview, RiskFlag: "hot_wallet_below_threshold"}, nil
	}
	return walletdomain.WithdrawDecision{Status: walletdomain.StatusApproved}, nil
}
