package db

import (
	"context"
	"encoding/json"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LedgerRepository struct {
	db *gorm.DB
}

type snapshotDeltaKey struct {
	AccountID uint64
	Asset     string
}

func NewLedgerRepository(db *gorm.DB) *LedgerRepository {
	return &LedgerRepository{db: db}
}

func (r *LedgerRepository) CreatePosting(ctx context.Context, posting ledgerdomain.PostingRequest) error {
	tx := DB(ctx, r.db)
	now := posting.LedgerTx.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if err := tx.Create(&LedgerTxModel{
		LedgerTxID:     posting.LedgerTx.ID,
		EventID:        posting.LedgerTx.EventID,
		BizType:        posting.LedgerTx.BizType,
		BizRefID:       posting.LedgerTx.BizRefID,
		Asset:          posting.LedgerTx.Asset,
		IdempotencyKey: posting.LedgerTx.IdempotencyKey,
		OperatorType:   posting.LedgerTx.OperatorType,
		OperatorID:     posting.LedgerTx.OperatorID,
		TraceID:        posting.LedgerTx.TraceID,
		Status:         posting.LedgerTx.Status,
		CreatedAt:      now,
	}).Error; err != nil {
		return err
	}

	entries := make([]LedgerEntryModel, 0, len(posting.Entries))
	deltas := map[snapshotDeltaKey]decimalx.Decimal{}
	for _, entry := range posting.Entries {
		entries = append(entries, LedgerEntryModel{
			LedgerTxID: posting.LedgerTx.ID,
			AccountID:  entry.AccountID,
			UserID:     entry.UserID,
			Asset:      entry.Asset,
			Amount:     entry.Amount,
			EntryType:  entry.EntryType,
			CreatedAt:  now,
		})

		key := snapshotDeltaKey{AccountID: entry.AccountID, Asset: entry.Asset}
		amount := decimalx.MustFromString(entry.Amount)
		if current, ok := deltas[key]; ok {
			deltas[key] = current.Add(amount)
		} else {
			deltas[key] = amount
		}
	}

	if err := tx.Create(&entries).Error; err != nil {
		return err
	}

	for key, delta := range deltas {
		snapshot := AccountBalanceSnapshotModel{
			AccountID: key.AccountID,
			Asset:     key.Asset,
			Balance:   delta.String(),
			Version:   1,
			UpdatedAt: now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "account_id"},
				{Name: "asset"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"balance":    gorm.Expr("balance + ?", delta.String()),
				"version":    gorm.Expr("version + 1"),
				"updated_at": now,
			}),
		}).Create(&snapshot).Error; err != nil {
			return err
		}
	}

	payload, err := json.Marshal(map[string]string{
		"ledger_tx_id": posting.LedgerTx.ID,
		"biz_type":     posting.LedgerTx.BizType,
		"biz_ref_id":   posting.LedgerTx.BizRefID,
		"trace_id":     posting.LedgerTx.TraceID,
	})
	if err != nil {
		return err
	}

	return tx.Create(&OutboxEventModel{
		EventID:       posting.LedgerTx.EventID,
		AggregateType: "ledger_tx",
		AggregateID:   posting.LedgerTx.ID,
		EventType:     "ledger.committed",
		PayloadJSON:   string(payload),
		Status:        "PENDING",
		CreatedAt:     now,
	}).Error
}

func (r *LedgerRepository) GetTxByIdempotencyKey(ctx context.Context, idempotencyKey string) (ledgerdomain.LedgerTx, error) {
	var model LedgerTxModel
	if err := DB(ctx, r.db).
		Where("idempotency_key = ?", idempotencyKey).
		Take(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ledgerdomain.LedgerTx{}, errorsx.ErrNotFound
		}
		return ledgerdomain.LedgerTx{}, err
	}
	return ledgerdomain.LedgerTx{
		ID:             model.LedgerTxID,
		EventID:        model.EventID,
		BizType:        model.BizType,
		BizRefID:       model.BizRefID,
		Asset:          model.Asset,
		IdempotencyKey: model.IdempotencyKey,
		OperatorType:   model.OperatorType,
		OperatorID:     model.OperatorID,
		TraceID:        model.TraceID,
		Status:         model.Status,
		CreatedAt:      model.CreatedAt,
	}, nil
}
