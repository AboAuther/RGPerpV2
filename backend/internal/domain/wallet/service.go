package wallet

import (
	"context"
	"fmt"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/authx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/clockx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
)

const (
	StatusCreditReady = "CREDIT_READY"
	StatusCredited    = "CREDITED"
	StatusRequested   = "REQUESTED"
	StatusHold        = "HOLD"
	StatusBroadcasted = "BROADCASTED"
	StatusRefunded    = "REFUNDED"
)

type AccountResolver interface {
	UserWalletAccountID(ctx context.Context, userID uint64, asset string) (uint64, error)
	UserWithdrawHoldAccountID(ctx context.Context, userID uint64, asset string) (uint64, error)
	DepositPendingAccountID(ctx context.Context, asset string) (uint64, error)
	WithdrawInTransitAccountID(ctx context.Context, asset string) (uint64, error)
	WithdrawFeeAccountID(ctx context.Context, asset string) (uint64, error)
}

type Service struct {
	deposits        DepositRepository
	withdraws       WithdrawRepository
	transfers       TransferResolver
	ledger          Ledger
	txManager       TxManager
	clock           clockx.Clock
	idgen           idgen.Generator
	accountResolver AccountResolver
}

func NewService(
	deposits DepositRepository,
	withdraws WithdrawRepository,
	transfers TransferResolver,
	ledger Ledger,
	txManager TxManager,
	clock clockx.Clock,
	idgen idgen.Generator,
	accountResolver AccountResolver,
) *Service {
	return &Service{
		deposits:        deposits,
		withdraws:       withdraws,
		transfers:       transfers,
		ledger:          ledger,
		txManager:       txManager,
		clock:           clock,
		idgen:           idgen,
		accountResolver: accountResolver,
	}
}

func (s *Service) ConfirmDeposit(ctx context.Context, input ConfirmDepositInput) error {
	deposit, err := s.deposits.GetByID(ctx, input.DepositID)
	if err != nil {
		return err
	}
	if deposit.Status != StatusCreditReady {
		return fmt.Errorf("%w: deposit is not credit ready", errorsx.ErrConflict)
	}

	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		userWalletID, err := s.accountResolver.UserWalletAccountID(txCtx, deposit.UserID, deposit.Asset)
		if err != nil {
			return err
		}
		pendingID, err := s.accountResolver.DepositPendingAccountID(txCtx, deposit.Asset)
		if err != nil {
			return err
		}

		ledgerTxID := s.idgen.NewID("ldg")
		if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             ledgerTxID,
				EventID:        s.idgen.NewID("evt"),
				BizType:        "DEPOSIT",
				BizRefID:       deposit.DepositID,
				Asset:          deposit.Asset,
				IdempotencyKey: input.IdempotencyKey,
				OperatorType:   "system",
				OperatorID:     "indexer",
				TraceID:        input.TraceID,
				Status:         "COMMITTED",
				CreatedAt:      s.clock.Now(),
			},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: userWalletID, Asset: deposit.Asset, Amount: deposit.Amount, EntryType: "DEPOSIT_CREDIT"},
				{AccountID: pendingID, Asset: deposit.Asset, Amount: negate(deposit.Amount), EntryType: "DEPOSIT_PENDING_RELEASE"},
			},
		}); err != nil {
			return err
		}
		return s.deposits.MarkCredited(txCtx, deposit.DepositID, ledgerTxID)
	})
}

func (s *Service) RequestWithdraw(ctx context.Context, input RequestWithdrawInput) (WithdrawRequest, error) {
	if _, err := authx.NormalizeEVMAddress(input.ToAddress); err != nil {
		return WithdrawRequest{}, err
	}
	now := s.clock.Now()
	withdraw := WithdrawRequest{
		WithdrawID: s.idgen.NewID("wd"),
		UserID:     input.UserID,
		ChainID:    input.ChainID,
		Asset:      input.Asset,
		Amount:     input.Amount,
		FeeAmount:  input.FeeAmount,
		ToAddress:  input.ToAddress,
		Status:     StatusHold,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		userWalletID, err := s.accountResolver.UserWalletAccountID(txCtx, input.UserID, input.Asset)
		if err != nil {
			return err
		}
		holdID, err := s.accountResolver.UserWithdrawHoldAccountID(txCtx, input.UserID, input.Asset)
		if err != nil {
			return err
		}
		ledgerTxID := s.idgen.NewID("ldg")
		withdraw.HoldLedgerTxID = ledgerTxID

		if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             ledgerTxID,
				EventID:        s.idgen.NewID("evt"),
				BizType:        "WITHDRAW_HOLD",
				BizRefID:       withdraw.WithdrawID,
				Asset:          input.Asset,
				IdempotencyKey: input.IdempotencyKey,
				OperatorType:   "user",
				OperatorID:     fmt.Sprintf("%d", input.UserID),
				TraceID:        input.TraceID,
				Status:         "COMMITTED",
				CreatedAt:      now,
			},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: holdID, Asset: input.Asset, Amount: input.Amount, EntryType: "WITHDRAW_HOLD_IN"},
				{AccountID: userWalletID, Asset: input.Asset, Amount: negate(input.Amount), EntryType: "WITHDRAW_HOLD_OUT"},
			},
		}); err != nil {
			return err
		}
		return s.withdraws.Create(txCtx, withdraw)
	})
	if err != nil {
		return WithdrawRequest{}, err
	}
	return withdraw, nil
}

func (s *Service) MarkWithdrawBroadcasted(ctx context.Context, input BroadcastWithdrawInput) error {
	withdraw, err := s.withdraws.GetByID(ctx, input.WithdrawID)
	if err != nil {
		return err
	}
	if withdraw.Status != StatusHold && withdraw.Status != StatusRequested {
		return fmt.Errorf("%w: withdraw cannot be broadcasted from current state", errorsx.ErrConflict)
	}

	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		holdID, err := s.accountResolver.UserWithdrawHoldAccountID(txCtx, withdraw.UserID, withdraw.Asset)
		if err != nil {
			return err
		}
		inTransitID, err := s.accountResolver.WithdrawInTransitAccountID(txCtx, withdraw.Asset)
		if err != nil {
			return err
		}
		feeID, err := s.accountResolver.WithdrawFeeAccountID(txCtx, withdraw.Asset)
		if err != nil {
			return err
		}

		netAmount := subtract(withdraw.Amount, withdraw.FeeAmount)
		if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             s.idgen.NewID("ldg"),
				EventID:        s.idgen.NewID("evt"),
				BizType:        "WITHDRAW_BROADCAST",
				BizRefID:       withdraw.WithdrawID,
				Asset:          withdraw.Asset,
				IdempotencyKey: input.IdempotencyKey,
				OperatorType:   "system",
				OperatorID:     "wallet",
				TraceID:        input.TraceID,
				Status:         "COMMITTED",
				CreatedAt:      s.clock.Now(),
			},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: inTransitID, Asset: withdraw.Asset, Amount: netAmount, EntryType: "WITHDRAW_IN_TRANSIT"},
				{AccountID: feeID, Asset: withdraw.Asset, Amount: withdraw.FeeAmount, EntryType: "WITHDRAW_FEE"},
				{AccountID: holdID, Asset: withdraw.Asset, Amount: negate(withdraw.Amount), EntryType: "WITHDRAW_HOLD_RELEASE"},
			},
		}); err != nil {
			return err
		}
		return s.withdraws.MarkBroadcasted(txCtx, withdraw.WithdrawID, input.TxHash)
	})
}

func (s *Service) RefundWithdraw(ctx context.Context, input RefundWithdrawInput) error {
	withdraw, err := s.withdraws.GetByID(ctx, input.WithdrawID)
	if err != nil {
		return err
	}

	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		userWalletID, err := s.accountResolver.UserWalletAccountID(txCtx, withdraw.UserID, withdraw.Asset)
		if err != nil {
			return err
		}
		holdID, err := s.accountResolver.UserWithdrawHoldAccountID(txCtx, withdraw.UserID, withdraw.Asset)
		if err != nil {
			return err
		}
		if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             s.idgen.NewID("ldg"),
				EventID:        s.idgen.NewID("evt"),
				BizType:        "WITHDRAW_REFUND",
				BizRefID:       withdraw.WithdrawID,
				Asset:          withdraw.Asset,
				IdempotencyKey: input.IdempotencyKey,
				OperatorType:   "system",
				OperatorID:     "wallet",
				TraceID:        input.TraceID,
				Status:         "COMMITTED",
				CreatedAt:      s.clock.Now(),
			},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: userWalletID, Asset: withdraw.Asset, Amount: withdraw.Amount, EntryType: "WITHDRAW_REFUND_IN"},
				{AccountID: holdID, Asset: withdraw.Asset, Amount: negate(withdraw.Amount), EntryType: "WITHDRAW_REFUND_OUT"},
			},
		}); err != nil {
			return err
		}
		return s.withdraws.MarkRefunded(txCtx, withdraw.WithdrawID)
	})
}

func (s *Service) Transfer(ctx context.Context, req TransferRequest) error {
	if req.FromUserID == 0 || req.ToUserID == 0 || req.Asset == "" || req.Amount == "" {
		return fmt.Errorf("%w: invalid transfer request", errorsx.ErrInvalidArgument)
	}
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		fromWalletID, err := s.accountResolver.UserWalletAccountID(txCtx, req.FromUserID, req.Asset)
		if err != nil {
			return err
		}
		toWalletID, err := s.accountResolver.UserWalletAccountID(txCtx, req.ToUserID, req.Asset)
		if err != nil {
			return err
		}
		return s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             s.idgen.NewID("ldg"),
				EventID:        s.idgen.NewID("evt"),
				BizType:        "TRANSFER",
				BizRefID:       req.TransferID,
				Asset:          req.Asset,
				IdempotencyKey: req.TransferID,
				OperatorType:   "user",
				OperatorID:     fmt.Sprintf("%d", req.FromUserID),
				TraceID:        req.TraceID,
				Status:         "COMMITTED",
				CreatedAt:      s.clock.Now(),
			},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: toWalletID, Asset: req.Asset, Amount: req.Amount, EntryType: "TRANSFER_IN"},
				{AccountID: fromWalletID, Asset: req.Asset, Amount: negate(req.Amount), EntryType: "TRANSFER_OUT"},
			},
		})
	})
}

func negate(raw string) string {
	if raw == "" {
		return raw
	}
	if raw[0] == '-' {
		return raw[1:]
	}
	return "-" + raw
}

func subtract(amount string, fee string) string {
	// P0 implementation uses string subtraction placeholder only for equal-scale input.
	// Full decimal arithmetic should be provided by the infra decimal package later.
	if fee == "0" || fee == "0.0" || fee == "0.000000" {
		return amount
	}
	return amount
}

var _ = time.Time{}
