package funding

import (
	"context"
	"fmt"
	"strings"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type ReverserConfig struct {
	Asset string
}

type Reverser struct {
	cfg      ReverserConfig
	clock    Clock
	idgen    IDGenerator
	txm      TxManager
	repo     Repository
	accounts AccountResolver
	ledger   LedgerPoster
	events   EventPublisher
}

func NewReverser(cfg ReverserConfig, clock Clock, idgen IDGenerator, txm TxManager, repo Repository, accounts AccountResolver, ledger LedgerPoster, events EventPublisher) (*Reverser, error) {
	if strings.TrimSpace(cfg.Asset) == "" {
		return nil, fmt.Errorf("%w: funding asset is required", errorsx.ErrInvalidArgument)
	}
	if clock == nil || idgen == nil || txm == nil || repo == nil || accounts == nil || ledger == nil || events == nil {
		return nil, fmt.Errorf("%w: missing funding reverser dependency", errorsx.ErrInvalidArgument)
	}
	return &Reverser{
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

func (r *Reverser) ReverseBatch(ctx context.Context, input ReverseBatchInput) (ReverseResult, error) {
	if strings.TrimSpace(input.FundingBatchID) == "" {
		return ReverseResult{}, fmt.Errorf("%w: funding batch id is required", errorsx.ErrInvalidArgument)
	}
	if strings.TrimSpace(input.OperatorID) == "" {
		return ReverseResult{}, fmt.Errorf("%w: operator id is required", errorsx.ErrInvalidArgument)
	}

	now := r.clock.Now().UTC()
	result := ReverseResult{}
	err := r.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		batch, err := r.repo.GetBatchByIDForUpdate(txCtx, input.FundingBatchID)
		if err != nil {
			return err
		}
		result.Batch = batch
		switch batch.Status {
		case BatchStatusReversed:
			return nil
		case BatchStatusApplied:
		default:
			return fmt.Errorf("%w: funding batch %s cannot be reversed from status %s", errorsx.ErrConflict, batch.ID, batch.Status)
		}

		items, err := r.repo.ListBatchItemsForUpdate(txCtx, batch.ID)
		if err != nil {
			return err
		}
		seenUsers := make(map[uint64]struct{}, len(items))
		reversedCount := 0
		for _, item := range items {
			switch item.Status {
			case ItemStatusReversed:
				if item.UserID != 0 {
					seenUsers[item.UserID] = struct{}{}
				}
				reversedCount++
				continue
			case ItemStatusApplied:
			default:
				return fmt.Errorf("%w: funding item %s is not reversible in status %s", errorsx.ErrConflict, item.PositionID, item.Status)
			}

			position, err := r.repo.GetPositionByIDForUpdate(txCtx, item.PositionID)
			if err != nil {
				return err
			}
			accounts, err := r.accounts.ResolveFundingAccounts(txCtx, item.UserID, r.cfg.Asset)
			if err != nil {
				return err
			}

			signedFee := decimalx.MustFromString(item.FundingFee)
			reversalSignedFee := signedFee.Neg()
			feeAbs := signedFee.Abs()
			if !feeAbs.IsZero() {
				ledgerTxID := r.idgen.NewID("ldg")
				entries := buildFundingSettlementEntries(accounts, r.cfg.Asset, item.UserID, reversalSignedFee, feeAbs)
				if err := r.ledger.Post(txCtx, ledgerdomain.PostingRequest{
					LedgerTx: ledgerdomain.LedgerTx{
						ID:             ledgerTxID,
						EventID:        r.idgen.NewID("evt"),
						BizType:        "funding.reversal",
						BizRefID:       batch.ID,
						Asset:          r.cfg.Asset,
						IdempotencyKey: fmt.Sprintf("funding:reverse:%s:%s", batch.ID, item.PositionID),
						OperatorType:   "SYSTEM",
						OperatorID:     input.OperatorID,
						TraceID:        input.TraceID,
						Status:         "COMMITTED",
						CreatedAt:      now,
					},
					Entries: entries,
				}); err != nil {
					return err
				}
				item.ReversalLedgerTxID = &ledgerTxID
			}

			accrual := decimalx.MustFromString(position.ExistingFundingAccrual).Add(reversalSignedFee)
			if err := r.repo.UpdatePositionFundingAccrual(txCtx, position.PositionID, accrual.String(), now); err != nil {
				return err
			}

			item.Status = ItemStatusReversed
			item.ReversedAt = &now
			if err := r.repo.UpdateBatchItem(txCtx, item); err != nil {
				return err
			}
			seenUsers[item.UserID] = struct{}{}
			reversedCount++
		}

		batch.Status = BatchStatusReversed
		batch.ReversedAt = &now
		batch.ReversedBy = &input.OperatorID
		if reason := strings.TrimSpace(input.Reason); reason != "" {
			batch.ReversalReason = &reason
		}
		batch.UpdatedAt = now
		if err := r.repo.UpdateBatch(txCtx, batch); err != nil {
			return err
		}
		if err := r.events.PublishBatchReversed(txCtx, BatchReversedEvent{
			FundingBatchID:  batch.ID,
			Symbol:          batch.Symbol,
			TimeWindowStart: batch.TimeWindowStart,
			TimeWindowEnd:   batch.TimeWindowEnd,
			NormalizedRate:  batch.NormalizedRate,
			Status:          batch.Status,
			ReversedCount:   reversedCount,
			ReversedAt:      now,
			ReversedBy:      batch.ReversedBy,
			ReversalReason:  batch.ReversalReason,
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
		return ReverseResult{}, err
	}
	return result, nil
}
