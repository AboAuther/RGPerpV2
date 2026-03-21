package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LedgerRepository struct {
	db *gorm.DB
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
	deltas := map[string]decimalx.Decimal{}
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

		key := fmt.Sprintf("%d:%s", entry.AccountID, entry.Asset)
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
		var accountID uint64
		var asset string
		if _, err := fmt.Sscanf(key, "%d:%s", &accountID, &asset); err != nil {
			return err
		}

		var snapshot AccountBalanceSnapshotModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("account_id = ? AND asset = ?", accountID, asset).
			Take(&snapshot).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				snapshot = AccountBalanceSnapshotModel{
					AccountID: accountID,
					Asset:     asset,
					Balance:   delta.String(),
					Version:   1,
					UpdatedAt: now,
				}
				if err := tx.Create(&snapshot).Error; err != nil {
					return err
				}
				continue
			}
			return err
		}

		current := decimalx.MustFromString(snapshot.Balance)
		snapshot.Balance = current.Add(delta).String()
		snapshot.Version++
		snapshot.UpdatedAt = now
		if err := tx.Save(&snapshot).Error; err != nil {
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
