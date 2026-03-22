package wallet

import (
	"context"
	"fmt"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/authx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/clockx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
)

const (
	StatusCreditReady   = "CREDIT_READY"
	StatusCredited      = "CREDITED"
	StatusDetected      = "DETECTED"
	StatusConfirming    = "CONFIRMING"
	StatusReorgReversed = "REORG_REVERSED"
	StatusRequested     = "REQUESTED"
	StatusHold          = "HOLD"
	StatusRiskReview    = "RISK_REVIEW"
	StatusApproved      = "APPROVED"
	StatusSigning       = "SIGNING"
	StatusBroadcasted   = "BROADCASTED"
	StatusCompleted     = "COMPLETED"
	StatusFailed        = "FAILED"
	StatusRefunded      = "REFUNDED"
)

type AccountResolver interface {
	UserWalletAccountID(ctx context.Context, userID uint64, asset string) (uint64, error)
	UserWithdrawHoldAccountID(ctx context.Context, userID uint64, asset string) (uint64, error)
	DepositPendingAccountID(ctx context.Context, asset string) (uint64, error)
	WithdrawInTransitAccountID(ctx context.Context, asset string) (uint64, error)
	WithdrawFeeAccountID(ctx context.Context, asset string) (uint64, error)
	CustodyHotAccountID(ctx context.Context, asset string) (uint64, error)
	TestFaucetPoolAccountID(ctx context.Context, asset string) (uint64, error)
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
	balances        BalanceRepository
	addresses       DepositAddressRepository
	allocator       DepositAddressAllocator
	riskEvaluator   WithdrawRiskEvaluator
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
	balances BalanceRepository,
	addresses DepositAddressRepository,
	allocator ...DepositAddressAllocator,
) *Service {
	var optionalAllocator DepositAddressAllocator
	if len(allocator) > 0 {
		optionalAllocator = allocator[0]
	}
	return &Service{
		deposits:        deposits,
		withdraws:       withdraws,
		transfers:       transfers,
		ledger:          ledger,
		txManager:       txManager,
		clock:           clock,
		idgen:           idgen,
		accountResolver: accountResolver,
		balances:        balances,
		addresses:       addresses,
		allocator:       optionalAllocator,
	}
}

func (s *Service) SetWithdrawRiskEvaluator(evaluator WithdrawRiskEvaluator) {
	s.riskEvaluator = evaluator
}

func (s *Service) DetectDeposit(ctx context.Context, input DetectDepositInput) (DepositChainTx, error) {
	if input.UserID == 0 || input.ChainID <= 0 || input.TxHash == "" || input.Asset == "" || input.Amount == "" {
		return DepositChainTx{}, fmt.Errorf("%w: invalid deposit detection input", errorsx.ErrInvalidArgument)
	}

	existing, err := s.deposits.GetByTxLog(ctx, input.ChainID, input.TxHash, input.LogIndex)
	if err == nil {
		return existing, nil
	}
	if err != nil && err != errorsx.ErrNotFound {
		return DepositChainTx{}, err
	}

	now := s.clock.Now()
	status := StatusDetected
	if input.Confirmations > 0 {
		status = StatusConfirming
	}
	if input.Confirmations >= input.RequiredConfs && input.RequiredConfs > 0 {
		status = StatusCreditReady
	}

	deposit := DepositChainTx{
		DepositID:     s.idgen.NewID("dep"),
		UserID:        input.UserID,
		ChainID:       input.ChainID,
		TxHash:        input.TxHash,
		LogIndex:      input.LogIndex,
		FromAddress:   input.FromAddress,
		ToAddress:     input.ToAddress,
		TokenAddress:  input.TokenAddress,
		Amount:        input.Amount,
		Asset:         input.Asset,
		BlockNumber:   input.BlockNumber,
		Confirmations: input.Confirmations,
		RequiredConfs: input.RequiredConfs,
		Status:        status,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		pendingID, err := s.accountResolver.DepositPendingAccountID(txCtx, input.Asset)
		if err != nil {
			return err
		}
		custodyID, err := s.accountResolver.CustodyHotAccountID(txCtx, input.Asset)
		if err != nil {
			return err
		}
		if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             s.idgen.NewID("ldg"),
				EventID:        s.idgen.NewID("evt"),
				BizType:        "DEPOSIT_DETECTED",
				BizRefID:       deposit.DepositID,
				Asset:          input.Asset,
				IdempotencyKey: input.IdempotencyKey,
				OperatorType:   "system",
				OperatorID:     "indexer",
				TraceID:        input.TraceID,
				Status:         "COMMITTED",
				CreatedAt:      now,
			},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: pendingID, Asset: input.Asset, Amount: input.Amount, EntryType: "DEPOSIT_PENDING_CONFIRM"},
				{AccountID: custodyID, Asset: input.Asset, Amount: negate(input.Amount), EntryType: "CUSTODY_HOT_PENDING"},
			},
		}); err != nil {
			return err
		}
		return s.deposits.Create(txCtx, deposit)
	}); err != nil {
		return DepositChainTx{}, err
	}

	return deposit, nil
}

func (s *Service) AdvanceDeposit(ctx context.Context, input AdvanceDepositInput) error {
	deposit, err := s.deposits.GetByID(ctx, input.DepositID)
	if err != nil {
		return err
	}
	if deposit.Status == StatusCredited {
		return nil
	}

	nextStatus := StatusDetected
	if input.Confirmations > 0 {
		nextStatus = StatusConfirming
	}
	if input.RequiredConfs > 0 && input.Confirmations >= input.RequiredConfs {
		nextStatus = StatusCreditReady
	}
	return s.deposits.UpdateConfirmations(ctx, input.DepositID, input.Confirmations, nextStatus)
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

func (s *Service) ReverseDeposit(ctx context.Context, input ReverseDepositInput) error {
	deposit, err := s.deposits.GetByID(ctx, input.DepositID)
	if err != nil {
		return err
	}
	if deposit.Status == StatusReorgReversed {
		return nil
	}
	if deposit.Status == StatusCredited {
		return fmt.Errorf("%w: credited deposit requires manual reconciliation", errorsx.ErrConflict)
	}

	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		pendingID, err := s.accountResolver.DepositPendingAccountID(txCtx, deposit.Asset)
		if err != nil {
			return err
		}
		custodyID, err := s.accountResolver.CustodyHotAccountID(txCtx, deposit.Asset)
		if err != nil {
			return err
		}
		if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             s.idgen.NewID("ldg"),
				EventID:        s.idgen.NewID("evt"),
				BizType:        "DEPOSIT_REORG_REVERSAL",
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
				{AccountID: pendingID, Asset: deposit.Asset, Amount: negate(deposit.Amount), EntryType: "DEPOSIT_PENDING_REORG_REVERSE"},
				{AccountID: custodyID, Asset: deposit.Asset, Amount: deposit.Amount, EntryType: "CUSTODY_HOT_REORG_RESTORE"},
			},
		}); err != nil {
			return err
		}
		return s.deposits.MarkReorgReversed(txCtx, deposit.DepositID)
	})
}

func (s *Service) RequestWithdraw(ctx context.Context, input RequestWithdrawInput) (WithdrawRequest, error) {
	if _, err := authx.NormalizeEVMAddress(input.ToAddress); err != nil {
		return WithdrawRequest{}, err
	}
	if input.UserID == 0 || input.ChainID <= 0 || input.Asset == "" {
		return WithdrawRequest{}, fmt.Errorf("%w: invalid withdraw request", errorsx.ErrInvalidArgument)
	}
	if err := ensurePositiveAmount(input.Amount); err != nil {
		return WithdrawRequest{}, err
	}
	if err := ensureNonNegativeAmount(input.FeeAmount); err != nil {
		return WithdrawRequest{}, err
	}
	now := s.clock.Now()
	status := StatusHold
	riskFlag := ""
	if s.riskEvaluator != nil {
		decision, err := s.riskEvaluator.Evaluate(ctx, WithdrawRiskInput{
			UserID:    input.UserID,
			ChainID:   input.ChainID,
			Asset:     input.Asset,
			Amount:    input.Amount,
			FeeAmount: input.FeeAmount,
			ToAddress: input.ToAddress,
		})
		if err != nil {
			return WithdrawRequest{}, err
		}
		switch decision.Status {
		case "", StatusHold:
			status = StatusHold
		case StatusApproved, StatusRiskReview:
			status = decision.Status
			riskFlag = decision.RiskFlag
		default:
			return WithdrawRequest{}, fmt.Errorf("%w: invalid withdraw risk decision", errorsx.ErrConflict)
		}
	}
	withdraw := WithdrawRequest{
		WithdrawID: s.idgen.NewID("wd"),
		UserID:     input.UserID,
		ChainID:    input.ChainID,
		Asset:      input.Asset,
		Amount:     input.Amount,
		FeeAmount:  input.FeeAmount,
		ToAddress:  input.ToAddress,
		Status:     status,
		RiskFlag:   riskFlag,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		userWalletID, err := s.accountResolver.UserWalletAccountID(txCtx, input.UserID, input.Asset)
		if err != nil {
			return err
		}
		if err := s.ensureSufficientBalance(txCtx, userWalletID, input.Asset, input.Amount); err != nil {
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

func (s *Service) ApproveWithdraw(ctx context.Context, input ApproveWithdrawInput) error {
	withdraw, err := s.withdraws.GetByID(ctx, input.WithdrawID)
	if err != nil {
		return err
	}
	if withdraw.Status != StatusHold && withdraw.Status != StatusRiskReview {
		return fmt.Errorf("%w: withdraw cannot be approved from current state", errorsx.ErrConflict)
	}
	return s.withdraws.UpdateStatus(ctx, input.WithdrawID, []string{StatusHold, StatusRiskReview}, StatusApproved)
}

func (s *Service) GenerateDepositAddress(ctx context.Context, input GenerateDepositAddressInput) (DepositAddress, error) {
	if input.UserID == 0 || input.ChainID <= 0 {
		return DepositAddress{}, fmt.Errorf("%w: invalid deposit address request", errorsx.ErrInvalidArgument)
	}
	asset := input.Asset
	if asset == "" {
		asset = "USDC"
	}
	if s.allocator == nil {
		return DepositAddress{}, fmt.Errorf("%w: deposit address allocator not configured", errorsx.ErrForbidden)
	}
	if existing, err := s.addresses.GetByUserChainAsset(ctx, input.UserID, input.ChainID, asset); err == nil {
		canonical, valid, err := s.allocator.Validate(ctx, input.UserID, input.ChainID, asset, existing.Address)
		if err != nil {
			return DepositAddress{}, err
		}
		if valid {
			if canonical != "" && canonical != existing.Address {
				existing.Address = canonical
				if err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
					return s.addresses.Upsert(txCtx, existing)
				}); err != nil {
					return DepositAddress{}, err
				}
			}
			return existing, nil
		}
	} else if err != errorsx.ErrNotFound {
		return DepositAddress{}, err
	}
	address, err := s.allocator.Allocate(ctx, input.UserID, input.ChainID, asset)
	if err != nil {
		return DepositAddress{}, err
	}
	if _, err := authx.NormalizeEVMAddress(address); err != nil {
		return DepositAddress{}, err
	}
	item := DepositAddress{
		UserID:        input.UserID,
		ChainID:       input.ChainID,
		Asset:         asset,
		Address:       address,
		Status:        "ACTIVE",
		Confirmations: 0,
		CreatedAt:     s.clock.Now(),
	}
	if err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		return s.addresses.Upsert(txCtx, item)
	}); err != nil {
		return DepositAddress{}, err
	}
	return item, nil
}

func (s *Service) MarkWithdrawBroadcasted(ctx context.Context, input BroadcastWithdrawInput) error {
	withdraw, err := s.withdraws.GetByID(ctx, input.WithdrawID)
	if err != nil {
		return err
	}
	if withdraw.Status != StatusApproved && withdraw.Status != StatusSigning {
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

		netAmount, err := subtract(withdraw.Amount, withdraw.FeeAmount)
		if err != nil {
			return err
		}
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
		if err := s.withdraws.UpdateStatus(txCtx, withdraw.WithdrawID, []string{StatusApproved, StatusSigning}, StatusBroadcasted); err != nil {
			return err
		}
		return s.withdraws.MarkBroadcasted(txCtx, withdraw.WithdrawID, input.TxHash)
	})
}

func (s *Service) CompleteWithdraw(ctx context.Context, input CompleteWithdrawInput) error {
	withdraw, err := s.withdraws.GetByID(ctx, input.WithdrawID)
	if err != nil {
		return err
	}
	if withdraw.Status != StatusBroadcasted {
		return fmt.Errorf("%w: withdraw is not awaiting completion", errorsx.ErrConflict)
	}

	netAmount, err := subtract(withdraw.Amount, withdraw.FeeAmount)
	if err != nil {
		return err
	}

	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		custodyID, err := s.accountResolver.CustodyHotAccountID(txCtx, withdraw.Asset)
		if err != nil {
			return err
		}
		inTransitID, err := s.accountResolver.WithdrawInTransitAccountID(txCtx, withdraw.Asset)
		if err != nil {
			return err
		}
		if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             s.idgen.NewID("ldg"),
				EventID:        s.idgen.NewID("evt"),
				BizType:        "WITHDRAW_COMPLETE",
				BizRefID:       withdraw.WithdrawID,
				Asset:          withdraw.Asset,
				IdempotencyKey: input.IdempotencyKey,
				OperatorType:   "system",
				OperatorID:     "indexer",
				TraceID:        input.TraceID,
				Status:         "COMMITTED",
				CreatedAt:      s.clock.Now(),
			},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: custodyID, Asset: withdraw.Asset, Amount: netAmount, EntryType: "WITHDRAW_CUSTODY_SETTLEMENT"},
				{AccountID: inTransitID, Asset: withdraw.Asset, Amount: negate(netAmount), EntryType: "WITHDRAW_IN_TRANSIT_RELEASE"},
			},
		}); err != nil {
			return err
		}
		if err := s.withdraws.UpdateStatus(txCtx, withdraw.WithdrawID, []string{StatusBroadcasted}, StatusCompleted); err != nil {
			return err
		}
		return s.withdraws.MarkCompleted(txCtx, withdraw.WithdrawID)
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
		switch withdraw.Status {
		case StatusHold, StatusRequested, StatusApproved, StatusRiskReview:
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
		case StatusBroadcasted, StatusFailed:
			inTransitID, err := s.accountResolver.WithdrawInTransitAccountID(txCtx, withdraw.Asset)
			if err != nil {
				return err
			}
			feeID, err := s.accountResolver.WithdrawFeeAccountID(txCtx, withdraw.Asset)
			if err != nil {
				return err
			}
			netAmount, err := subtract(withdraw.Amount, withdraw.FeeAmount)
			if err != nil {
				return err
			}
			if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
				LedgerTx: ledgerdomain.LedgerTx{
					ID:             s.idgen.NewID("ldg"),
					EventID:        s.idgen.NewID("evt"),
					BizType:        "WITHDRAW_REFUND_REVERSAL",
					BizRefID:       withdraw.WithdrawID,
					Asset:          withdraw.Asset,
					IdempotencyKey: input.IdempotencyKey + ":reversal",
					OperatorType:   "system",
					OperatorID:     "wallet",
					TraceID:        input.TraceID,
					Status:         "COMMITTED",
					CreatedAt:      s.clock.Now(),
				},
				Entries: []ledgerdomain.LedgerEntry{
					{AccountID: holdID, Asset: withdraw.Asset, Amount: withdraw.Amount, EntryType: "WITHDRAW_HOLD_RESTORE"},
					{AccountID: inTransitID, Asset: withdraw.Asset, Amount: negate(netAmount), EntryType: "WITHDRAW_IN_TRANSIT_REVERSE"},
					{AccountID: feeID, Asset: withdraw.Asset, Amount: negate(withdraw.FeeAmount), EntryType: "WITHDRAW_FEE_REVERSE"},
				},
			}); err != nil {
				return err
			}
			if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
				LedgerTx: ledgerdomain.LedgerTx{
					ID:             s.idgen.NewID("ldg"),
					EventID:        s.idgen.NewID("evt"),
					BizType:        "WITHDRAW_REFUND",
					BizRefID:       withdraw.WithdrawID,
					Asset:          withdraw.Asset,
					IdempotencyKey: input.IdempotencyKey + ":refund",
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
		default:
			return fmt.Errorf("%w: withdraw cannot be refunded from current state", errorsx.ErrConflict)
		}
		if err := s.withdraws.UpdateStatus(txCtx, withdraw.WithdrawID, []string{StatusHold, StatusBroadcasted, StatusFailed}, StatusRefunded); err != nil {
			return err
		}
		return s.withdraws.MarkRefunded(txCtx, withdraw.WithdrawID)
	})
}

func (s *Service) Transfer(ctx context.Context, req TransferRequest) error {
	if req.FromUserID == 0 || req.ToUserID == 0 || req.Asset == "" || req.Amount == "" {
		return fmt.Errorf("%w: invalid transfer request", errorsx.ErrInvalidArgument)
	}
	if err := ensurePositiveAmount(req.Amount); err != nil {
		return err
	}
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		fromWalletID, err := s.accountResolver.UserWalletAccountID(txCtx, req.FromUserID, req.Asset)
		if err != nil {
			return err
		}
		if err := s.ensureSufficientBalance(txCtx, fromWalletID, req.Asset, req.Amount); err != nil {
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

func (s *Service) ListDepositAddresses(ctx context.Context, userID uint64) ([]DepositAddress, error) {
	return s.addresses.ListByUser(ctx, userID)
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

func subtract(amount string, fee string) (string, error) {
	base, err := decimalx.NewFromString(amount)
	if err != nil {
		return "", err
	}
	cost, err := decimalx.NewFromString(fee)
	if err != nil {
		return "", err
	}
	if base.LessThan(cost) {
		return "", fmt.Errorf("%w: fee exceeds amount", errorsx.ErrInvalidArgument)
	}
	return base.Sub(cost).String(), nil
}

func (s *Service) ensureSufficientBalance(ctx context.Context, accountID uint64, asset string, amount string) error {
	currentRaw, err := s.balances.GetAccountBalanceForUpdate(ctx, accountID, asset)
	if err != nil {
		return err
	}
	current, err := decimalx.NewFromString(currentRaw)
	if err != nil {
		return err
	}
	required, err := decimalx.NewFromString(amount)
	if err != nil {
		return err
	}
	if current.LessThan(required) {
		return fmt.Errorf("%w: insufficient available balance", errorsx.ErrForbidden)
	}
	return nil
}

func ensurePositiveAmount(raw string) error {
	amount, err := decimalx.NewFromString(raw)
	if err != nil {
		return err
	}
	if !amount.GreaterThan(decimalx.MustFromString("0")) {
		return fmt.Errorf("%w: amount must be positive", errorsx.ErrInvalidArgument)
	}
	return nil
}

func ensureNonNegativeAmount(raw string) error {
	amount, err := decimalx.NewFromString(raw)
	if err != nil {
		return err
	}
	if amount.LessThan(decimalx.MustFromString("0")) {
		return fmt.Errorf("%w: amount must be non-negative", errorsx.ErrInvalidArgument)
	}
	return nil
}
