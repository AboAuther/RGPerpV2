package withdrawexec

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/shopspring/decimal"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	chaininfra "github.com/xiaobao/rgperp/backend/internal/infra/chain"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type WithdrawRepository interface {
	GetByID(ctx context.Context, withdrawID string) (walletdomain.WithdrawRequest, error)
	ListByChainStatuses(ctx context.Context, chainID int64, statuses []string, limit int) ([]walletdomain.WithdrawRequest, error)
	UpdateStatus(ctx context.Context, withdrawID string, from []string, to string) error
}

type Wallet interface {
	MarkWithdrawBroadcasted(ctx context.Context, input walletdomain.BroadcastWithdrawInput) error
}

type Executor interface {
	ExecuteWithdrawal(ctx context.Context, chainID int64, toAddress string, amount string, withdrawID string) (string, error)
}

type Service struct {
	withdraws WithdrawRepository
	wallet    Wallet
	executor  Executor
}

func NewService(withdraws WithdrawRepository, wallet Wallet, executor Executor) (*Service, error) {
	if withdraws == nil || wallet == nil || executor == nil {
		return nil, fmt.Errorf("%w: withdraw executor dependencies are required", errorsx.ErrInvalidArgument)
	}
	return &Service{withdraws: withdraws, wallet: wallet, executor: executor}, nil
}

func (s *Service) ProcessChain(ctx context.Context, chainID int64, limit int) error {
	if limit <= 0 {
		limit = 50
	}
	pending, err := s.withdraws.ListByChainStatuses(ctx, chainID, []string{walletdomain.StatusApproved}, limit)
	if err != nil {
		return err
	}
	for _, withdraw := range pending {
		if err := s.processWithdraw(ctx, withdraw); err != nil {
			log.Printf("withdraw-executor process failed: withdraw_id=%s err=%v", withdraw.WithdrawID, err)
		}
	}
	return nil
}

func (s *Service) processWithdraw(ctx context.Context, withdraw walletdomain.WithdrawRequest) error {
	if err := s.withdraws.UpdateStatus(ctx, withdraw.WithdrawID, []string{walletdomain.StatusApproved}, walletdomain.StatusSigning); err != nil {
		if err == errorsx.ErrConflict {
			return nil
		}
		return err
	}

	current, err := s.withdraws.GetByID(ctx, withdraw.WithdrawID)
	if err != nil {
		return err
	}
	netAmount, err := netWithdrawAmount(current.Amount, current.FeeAmount)
	if err != nil {
		_ = s.withdraws.UpdateStatus(ctx, current.WithdrawID, []string{walletdomain.StatusSigning}, walletdomain.StatusRiskReview)
		return err
	}
	txHash, err := s.executor.ExecuteWithdrawal(ctx, current.ChainID, current.ToAddress, netAmount, current.WithdrawID)
	if err != nil {
		nextStatus := walletdomain.StatusApproved
		if chaininfra.IsReviewRequired(err) {
			nextStatus = walletdomain.StatusRiskReview
		}
		_ = s.withdraws.UpdateStatus(ctx, current.WithdrawID, []string{walletdomain.StatusSigning}, nextStatus)
		return err
	}

	// Retry local persistence a few times to shrink orphan-broadcast risk.
	var persistErr error
	for attempt := 0; attempt < 3; attempt++ {
		persistErr = s.wallet.MarkWithdrawBroadcasted(ctx, walletdomain.BroadcastWithdrawInput{
			WithdrawID:     current.WithdrawID,
			TxHash:         txHash,
			IdempotencyKey: fmt.Sprintf("withdraw_broadcast:%s:%s", current.WithdrawID, txHash),
			TraceID:        fmt.Sprintf("withdraw_exec:%s", current.WithdrawID),
		})
		if persistErr == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("withdraw sent on-chain but local broadcast persist failed: %w", persistErr)
}

func netWithdrawAmount(amount string, fee string) (string, error) {
	base, err := decimal.NewFromString(amount)
	if err != nil {
		return "", err
	}
	cost, err := decimal.NewFromString(fee)
	if err != nil {
		return "", err
	}
	if base.LessThan(cost) {
		return "", fmt.Errorf("%w: fee exceeds amount", errorsx.ErrInvalidArgument)
	}
	return base.Sub(cost).String(), nil
}
