package withdrawexec

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/shopspring/decimal"
	chaininfra "github.com/xiaobao/rgperp/backend/internal/infra/chain"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type WithdrawRepository interface {
	GetByID(ctx context.Context, withdrawID string) (walletdomain.WithdrawRequest, error)
	ListByChainStatuses(ctx context.Context, chainID int64, statuses []string, limit int) ([]walletdomain.WithdrawRequest, error)
	UpdateStatus(ctx context.Context, withdrawID string, from []string, to string) error
	ReserveNextNonce(ctx context.Context, chainID int64, signerAddress string, chainPendingNonce uint64) (walletdomain.WithdrawRequest, error)
	ListReservedForBroadcastRetry(ctx context.Context, chainID int64, limit int) ([]walletdomain.WithdrawRequest, error)
}

type Wallet interface {
	MarkWithdrawBroadcasted(ctx context.Context, input walletdomain.BroadcastWithdrawInput) error
	FailWithdraw(ctx context.Context, input walletdomain.FailWithdrawInput) error
}

type Executor interface {
	SignerAddress() string
	NextNonce(ctx context.Context, chainID int64) (uint64, error)
	ExecuteWithdrawal(ctx context.Context, chainID int64, toAddress string, amount string, withdrawID string, nonce uint64) (string, error)
}

type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type Service struct {
	withdraws WithdrawRepository
	wallet    Wallet
	executor  Executor
	txManager TxManager
}

func NewService(withdraws WithdrawRepository, wallet Wallet, executor Executor, txManager TxManager) (*Service, error) {
	if withdraws == nil || wallet == nil || executor == nil || txManager == nil {
		return nil, fmt.Errorf("%w: withdraw executor dependencies are required", errorsx.ErrInvalidArgument)
	}
	return &Service{withdraws: withdraws, wallet: wallet, executor: executor, txManager: txManager}, nil
}

func (s *Service) ProcessChain(ctx context.Context, chainID int64, limit int) error {
	if limit <= 0 {
		limit = 50
	}
	retries, err := s.withdraws.ListReservedForBroadcastRetry(ctx, chainID, limit)
	if err != nil {
		return err
	}
	for _, withdraw := range retries {
		if err := s.broadcastReservedWithdraw(ctx, withdraw); err != nil {
			log.Printf("withdraw-executor process failed: withdraw_id=%s err=%v", withdraw.WithdrawID, err)
		}
	}

	for processed := 0; processed < limit; processed++ {
		withdraw, err := s.reserveNextWithdraw(ctx, chainID)
		if err != nil {
			if errors.Is(err, errorsx.ErrNotFound) || errors.Is(err, errorsx.ErrConflict) {
				break
			}
			return err
		}
		if err := s.broadcastReservedWithdraw(ctx, withdraw); err != nil {
			log.Printf("withdraw-executor process failed: withdraw_id=%s err=%v", withdraw.WithdrawID, err)
		}
	}
	return nil
}

func (s *Service) reserveNextWithdraw(ctx context.Context, chainID int64) (walletdomain.WithdrawRequest, error) {
	chainNonce, err := s.executor.NextNonce(ctx, chainID)
	if err != nil {
		return walletdomain.WithdrawRequest{}, err
	}
	var reserved walletdomain.WithdrawRequest
	if err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		var reserveErr error
		reserved, reserveErr = s.withdraws.ReserveNextNonce(txCtx, chainID, s.executor.SignerAddress(), chainNonce)
		return reserveErr
	}); err != nil {
		return walletdomain.WithdrawRequest{}, err
	}
	return reserved, nil
}

func (s *Service) broadcastReservedWithdraw(ctx context.Context, withdraw walletdomain.WithdrawRequest) error {
	if withdraw.BroadcastNonce == nil {
		return fmt.Errorf("%w: reserved withdraw nonce is required", errorsx.ErrConflict)
	}
	netAmount, err := netWithdrawAmount(withdraw.Amount, withdraw.FeeAmount)
	if err != nil {
		return err
	}
	txHash, err := s.executor.ExecuteWithdrawal(ctx, withdraw.ChainID, withdraw.ToAddress, netAmount, withdraw.WithdrawID, *withdraw.BroadcastNonce)
	if err != nil {
		if chaininfra.IsReviewRequired(err) {
			riskFlag := chaininfra.ReviewRequiredReason(err)
			if riskFlag == "" {
				riskFlag = "withdraw_execution_review_required"
			}
			return s.wallet.FailWithdraw(ctx, walletdomain.FailWithdrawInput{
				WithdrawID: withdraw.WithdrawID,
				RiskFlag:   riskFlag,
				TraceID:    fmt.Sprintf("withdraw_exec_fail:%s", withdraw.WithdrawID),
			})
		}
		return err
	}

	// Retry local persistence a few times to shrink orphan-broadcast risk.
	var persistErr error
	for attempt := 0; attempt < 3; attempt++ {
		persistErr = s.wallet.MarkWithdrawBroadcasted(ctx, walletdomain.BroadcastWithdrawInput{
			WithdrawID:     withdraw.WithdrawID,
			TxHash:         txHash,
			BroadcastNonce: withdraw.BroadcastNonce,
			IdempotencyKey: fmt.Sprintf("withdraw_broadcast:%s:%s", withdraw.WithdrawID, txHash),
			TraceID:        fmt.Sprintf("withdraw_exec:%s", withdraw.WithdrawID),
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
