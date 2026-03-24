package funding

import (
	"context"
	"fmt"
	"strings"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type ExecutorConfig struct {
	Asset string
}

type Executor struct {
	cfg      ExecutorConfig
	clock    Clock
	idgen    IDGenerator
	txm      TxManager
	repo     Repository
	accounts AccountResolver
	ledger   LedgerPoster
	events   EventPublisher
}

func NewExecutor(cfg ExecutorConfig, clock Clock, idgen IDGenerator, txm TxManager, repo Repository, accounts AccountResolver, ledger LedgerPoster, events EventPublisher) (*Executor, error) {
	if strings.TrimSpace(cfg.Asset) == "" {
		return nil, fmt.Errorf("%w: funding asset is required", errorsx.ErrInvalidArgument)
	}
	if clock == nil || idgen == nil || txm == nil || repo == nil || accounts == nil || ledger == nil || events == nil {
		return nil, fmt.Errorf("%w: missing funding executor dependency", errorsx.ErrInvalidArgument)
	}
	return &Executor{
		cfg:      cfg,
		clock:    clock,
		idgen:    idgen,
		txm:      txm,
		repo:     repo,
		accounts: accounts,
		ledger:   ledger,
		events:   events,
	}, nil
}

func (e *Executor) ApplyReadyBatches(ctx context.Context, limit int) ([]ApplyResult, error) {
	if limit <= 0 {
		limit = 100
	}
	batches, err := e.repo.ListExecutableBatches(ctx, limit)
	if err != nil {
		return nil, err
	}
	results := make([]ApplyResult, 0, len(batches))
	for _, batch := range batches {
		result, err := e.ApplyBatch(ctx, batch.ID)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (e *Executor) ApplyBatch(ctx context.Context, fundingBatchID string) (ApplyResult, error) {
	if strings.TrimSpace(fundingBatchID) == "" {
		return ApplyResult{}, fmt.Errorf("%w: funding batch id is required", errorsx.ErrInvalidArgument)
	}

	now := e.clock.Now().UTC()
	result := ApplyResult{}
	executionStarted := false
	err := e.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		batch, err := e.repo.GetBatchByIDForUpdate(txCtx, fundingBatchID)
		if err != nil {
			return err
		}
		result.Batch = batch
		switch batch.Status {
		case BatchStatusApplied:
			return nil
		case BatchStatusReady, BatchStatusApplying, BatchStatusFailed:
		default:
			return fmt.Errorf("%w: funding batch %s is not executable in status %s", errorsx.ErrConflict, batch.ID, batch.Status)
		}
		executionStarted = true

		batch.Status = BatchStatusApplying
		batch.UpdatedAt = now
		if err := e.repo.UpdateBatch(txCtx, batch); err != nil {
			return err
		}

		items, err := e.repo.ListBatchItemsForUpdate(txCtx, batch.ID)
		if err != nil {
			return err
		}
		seenUsers := make(map[uint64]struct{}, len(items))
		appliedCount := 0
		for _, item := range items {
			switch item.Status {
			case ItemStatusApplied:
				if item.UserID != 0 {
					seenUsers[item.UserID] = struct{}{}
				}
				appliedCount++
				continue
			case ItemStatusPending, ItemStatusFailed:
			default:
				return fmt.Errorf("%w: unsupported funding item status %s", errorsx.ErrConflict, item.Status)
			}

			position, err := e.repo.GetPositionByIDForUpdate(txCtx, item.PositionID)
			if err != nil {
				return err
			}
			if position.UserID != item.UserID {
				return fmt.Errorf("%w: funding item user mismatch", errorsx.ErrConflict)
			}

			accounts, err := e.accounts.ResolveFundingAccounts(txCtx, item.UserID, e.cfg.Asset)
			if err != nil {
				return err
			}

			signedFee := decimalx.MustFromString(item.FundingFee)
			if signedFee.IsZero() {
				item.Status = ItemStatusApplied
				item.LedgerTxID = nil
				if err := e.repo.UpdateBatchItem(txCtx, item); err != nil {
					return err
				}
				appliedCount++
				continue
			}

			fundingFeeAbs := signedFee.Abs()
			ledgerTxID := e.idgen.NewID("ldg")
			entries := buildFundingSettlementEntries(accounts, e.cfg.Asset, item.UserID, signedFee, fundingFeeAbs)
			if err := e.ledger.Post(txCtx, ledgerdomain.PostingRequest{
				LedgerTx: ledgerdomain.LedgerTx{
					ID:             ledgerTxID,
					EventID:        e.idgen.NewID("evt"),
					BizType:        "funding.settlement",
					BizRefID:       batch.ID,
					Asset:          e.cfg.Asset,
					IdempotencyKey: fmt.Sprintf("funding:%s:%s", batch.ID, item.PositionID),
					OperatorType:   "SYSTEM",
					OperatorID:     "funding-worker",
					TraceID:        "funding:" + batch.ID,
					Status:         "COMMITTED",
					CreatedAt:      now,
				},
				Entries: entries,
			}); err != nil {
				return err
			}

			accrual := decimalx.MustFromString(position.ExistingFundingAccrual).Add(signedFee)
			if err := e.repo.UpdatePositionFundingAccrual(txCtx, position.PositionID, accrual.String(), now); err != nil {
				return err
			}

			item.Status = ItemStatusApplied
			item.LedgerTxID = &ledgerTxID
			if err := e.repo.UpdateBatchItem(txCtx, item); err != nil {
				return err
			}
			seenUsers[item.UserID] = struct{}{}
			appliedCount++
		}

		batch.Status = BatchStatusApplied
		batch.UpdatedAt = now
		if err := e.repo.UpdateBatch(txCtx, batch); err != nil {
			return err
		}
		if err := e.events.PublishBatchApplied(txCtx, BatchAppliedEvent{
			FundingBatchID:  batch.ID,
			Symbol:          batch.Symbol,
			TimeWindowStart: batch.TimeWindowStart,
			TimeWindowEnd:   batch.TimeWindowEnd,
			NormalizedRate:  batch.NormalizedRate,
			Status:          batch.Status,
			AppliedCount:    appliedCount,
		}); err != nil {
			return err
		}
		result.Batch = batch
		result.UserIDs = make([]uint64, 0, len(seenUsers))
		for userID := range seenUsers {
			result.UserIDs = append(result.UserIDs, userID)
		}
		return nil
	})
	if err != nil {
		if executionStarted {
			if markErr := e.repo.MarkBatchFailed(ctx, fundingBatchID, now); markErr != nil {
				return ApplyResult{}, fmt.Errorf("mark funding batch failed after apply error: %w: %v", err, markErr)
			}
		}
		return ApplyResult{}, err
	}
	return result, nil
}

func buildFundingSettlementEntries(accounts FundingAccounts, asset string, userID uint64, signedFee decimalx.Decimal, feeAbs decimalx.Decimal) []ledgerdomain.LedgerEntry {
	entries := []ledgerdomain.LedgerEntry{
		{
			AccountID: accounts.FundingPoolAccountID,
			Asset:     asset,
			Amount:    feeAbs.String(),
			EntryType: "FUNDING_SETTLEMENT",
		},
		{
			AccountID: accounts.UserPositionMarginAccountID,
			UserID:    uint64Ptr(userID),
			Asset:     asset,
			Amount:    feeAbs.Neg().String(),
			EntryType: "FUNDING_SETTLEMENT",
		},
	}
	if signedFee.GreaterThan(decimalx.MustFromString("0")) {
		entries[0].Amount = feeAbs.Neg().String()
		entries[1].Amount = feeAbs.String()
	}
	return entries
}

func uint64Ptr(v uint64) *uint64 { return &v }
