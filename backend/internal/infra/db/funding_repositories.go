package db

import (
	"context"
	"errors"
	"time"

	fundingdomain "github.com/xiaobao/rgperp/backend/internal/domain/funding"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FundingRepository struct {
	db *gorm.DB
}

func NewFundingRepository(db *gorm.DB) *FundingRepository {
	return &FundingRepository{db: db}
}

func (r *FundingRepository) ListSymbolsForFunding(ctx context.Context) ([]fundingdomain.Symbol, error) {
	var models []SymbolModel
	if err := DB(ctx, r.db).
		Where("status IN ?", []string{"TRADING", "REDUCE_ONLY", "PAUSED"}).
		Order("symbol ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return []fundingdomain.Symbol{}, nil
	}
	ids := make([]uint64, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	var mappingModels []SymbolMappingModel
	if err := DB(ctx, r.db).
		Where("symbol_id IN ? AND source_name IN ? AND status = ?", ids, []string{"binance", "hyperliquid"}, "ACTIVE").
		Order("symbol_id ASC, source_name ASC").
		Find(&mappingModels).Error; err != nil {
		return nil, err
	}
	mappingsBySymbol := make(map[uint64][]fundingdomain.SymbolMapping, len(models))
	for _, model := range mappingModels {
		mappingsBySymbol[model.SymbolID] = append(mappingsBySymbol[model.SymbolID], fundingdomain.SymbolMapping{
			SourceName:   model.SourceName,
			SourceSymbol: model.SourceSymbol,
			Status:       model.Status,
		})
	}
	out := make([]fundingdomain.Symbol, 0, len(models))
	for _, model := range models {
		mappings := mappingsBySymbol[model.ID]
		if len(mappings) == 0 {
			continue
		}
		out = append(out, fundingdomain.Symbol{
			ID:         model.ID,
			Symbol:     model.Symbol,
			AssetClass: model.AssetClass,
			Status:     model.Status,
			Mappings:   mappings,
		})
	}
	return out, nil
}

func (r *FundingRepository) DowngradeSymbolToReduceOnly(ctx context.Context, symbolID uint64) (bool, error) {
	result := DB(ctx, r.db).Model(&SymbolModel{}).
		Where("id = ? AND status = ?", symbolID, "TRADING").
		Update("status", "REDUCE_ONLY")
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *FundingRepository) RestoreSymbolToTrading(ctx context.Context, symbolID uint64) (bool, error) {
	result := DB(ctx, r.db).Model(&SymbolModel{}).
		Where("id = ? AND status = ?", symbolID, "REDUCE_ONLY").
		Update("status", "TRADING")
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *FundingRepository) GetBatchByWindow(ctx context.Context, symbolID uint64, start time.Time, end time.Time) (fundingdomain.Batch, error) {
	var model FundingBatchModel
	if err := DB(ctx, r.db).
		Where("symbol_id = ? AND time_window_start = ? AND time_window_end = ?", symbolID, start.UTC(), end.UTC()).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fundingdomain.Batch{}, errorsx.ErrNotFound
		}
		return fundingdomain.Batch{}, err
	}
	return toFundingBatchDomain(model), nil
}

func (r *FundingRepository) ListExecutableBatches(ctx context.Context, limit int) ([]fundingdomain.Batch, error) {
	if limit <= 0 {
		limit = 100
	}
	var models []FundingBatchModel
	if err := DB(ctx, r.db).
		Where("status IN ?", []string{fundingdomain.BatchStatusReady, fundingdomain.BatchStatusApplying, fundingdomain.BatchStatusFailed}).
		Order("time_window_end ASC, id ASC").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]fundingdomain.Batch, 0, len(models))
	for _, model := range models {
		out = append(out, toFundingBatchDomain(model))
	}
	return out, nil
}

func (r *FundingRepository) GetBatchByIDForUpdate(ctx context.Context, fundingBatchID string) (fundingdomain.Batch, error) {
	var model FundingBatchModel
	if err := DB(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("funding_batch_id = ?", fundingBatchID).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fundingdomain.Batch{}, errorsx.ErrNotFound
		}
		return fundingdomain.Batch{}, err
	}
	return toFundingBatchDomain(model), nil
}

func (r *FundingRepository) GetSettlementPriceAtOrBefore(ctx context.Context, symbolID uint64, cutoff time.Time) (fundingdomain.SettlementPrice, error) {
	var model MarkPriceSnapshotModel
	if err := DB(ctx, r.db).
		Where("symbol_id = ? AND created_at <= ?", symbolID, cutoff.UTC()).
		Order("id DESC").
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fundingdomain.SettlementPrice{}, errorsx.ErrNotFound
		}
		return fundingdomain.SettlementPrice{}, err
	}
	return fundingdomain.SettlementPrice{
		SymbolID:  symbolID,
		Price:     model.IndexPrice,
		CreatedAt: model.CreatedAt.UTC(),
	}, nil
}

func (r *FundingRepository) ListLatestRateSnapshots(ctx context.Context, symbolID uint64, cutoff time.Time) ([]fundingdomain.RateSnapshot, error) {
	var models []FundingRateSnapshotModel
	if err := DB(ctx, r.db).Raw(`
SELECT frs.*
FROM funding_rate_snapshots frs
JOIN (
  SELECT source_name, MAX(id) AS max_id
  FROM funding_rate_snapshots
  WHERE symbol_id = ? AND created_at <= ?
  GROUP BY source_name
) latest ON latest.max_id = frs.id
ORDER BY frs.source_name ASC
`, symbolID, cutoff.UTC()).Scan(&models).Error; err != nil {
		return nil, err
	}
	out := make([]fundingdomain.RateSnapshot, 0, len(models))
	for _, model := range models {
		out = append(out, fundingdomain.RateSnapshot{
			SymbolID:        model.SymbolID,
			SourceName:      model.SourceName,
			SourceSymbol:    model.SourceSymbol,
			Rate:            model.FundingRate,
			IntervalSeconds: model.IntervalSeconds,
			SourceTS:        model.SourceTS.UTC(),
			ReceivedTS:      model.ReceivedTS.UTC(),
			CollectedAt:     model.CreatedAt.UTC(),
		})
	}
	return out, nil
}

func (r *FundingRepository) AppendRateSnapshots(ctx context.Context, snapshots []fundingdomain.RateSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	models := make([]FundingRateSnapshotModel, 0, len(snapshots))
	for _, snapshot := range snapshots {
		models = append(models, FundingRateSnapshotModel{
			SymbolID:        snapshot.SymbolID,
			SourceName:      snapshot.SourceName,
			SourceSymbol:    snapshot.SourceSymbol,
			FundingRate:     snapshot.Rate,
			IntervalSeconds: snapshot.IntervalSeconds,
			SourceTS:        snapshot.SourceTS.UTC(),
			ReceivedTS:      snapshot.ReceivedTS.UTC(),
			CreatedAt:       snapshot.CollectedAt.UTC(),
		})
	}
	return DB(ctx, r.db).Create(&models).Error
}

func (r *FundingRepository) CreateBatch(ctx context.Context, batch fundingdomain.Batch) error {
	return DB(ctx, r.db).Create(&FundingBatchModel{
		FundingBatchID:  batch.ID,
		SymbolID:        batch.SymbolID,
		TimeWindowStart: batch.TimeWindowStart,
		TimeWindowEnd:   batch.TimeWindowEnd,
		NormalizedRate:  batch.NormalizedRate,
		SettlementPrice: batch.SettlementPrice,
		Status:          batch.Status,
		ReversedAt:      batch.ReversedAt,
		ReversedBy:      batch.ReversedBy,
		ReversalReason:  batch.ReversalReason,
		CreatedAt:       batch.CreatedAt,
		UpdatedAt:       batch.UpdatedAt,
	}).Error
}

func (r *FundingRepository) UpdateBatch(ctx context.Context, batch fundingdomain.Batch) error {
	return DB(ctx, r.db).Model(&FundingBatchModel{}).
		Where("funding_batch_id = ?", batch.ID).
		Updates(map[string]any{
			"status":          batch.Status,
			"reversed_at":     batch.ReversedAt,
			"reversed_by":     batch.ReversedBy,
			"reversal_reason": batch.ReversalReason,
			"updated_at":      batch.UpdatedAt.UTC(),
		}).Error
}

func (r *FundingRepository) MarkBatchFailed(ctx context.Context, fundingBatchID string, failedAt time.Time) error {
	return DB(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&FundingBatchModel{}).
			Where("funding_batch_id = ? AND status <> ?", fundingBatchID, fundingdomain.BatchStatusApplied).
			Updates(map[string]any{
				"status":     fundingdomain.BatchStatusFailed,
				"updated_at": failedAt.UTC(),
			}).Error; err != nil {
			return err
		}
		return tx.Model(&FundingBatchItemModel{}).
			Where("funding_batch_id = ? AND status <> ?", fundingBatchID, fundingdomain.ItemStatusApplied).
			Update("status", fundingdomain.ItemStatusFailed).Error
	})
}

func (r *FundingRepository) CreateBatchItems(ctx context.Context, items []fundingdomain.BatchItem) error {
	if len(items) == 0 {
		return nil
	}
	models := make([]FundingBatchItemModel, 0, len(items))
	for _, item := range items {
		models = append(models, FundingBatchItemModel{
			FundingBatchID:     item.FundingBatchID,
			PositionID:         item.PositionID,
			UserID:             item.UserID,
			FundingFee:         item.FundingFee,
			LedgerTxID:         item.LedgerTxID,
			ReversalLedgerTxID: item.ReversalLedgerTxID,
			Status:             item.Status,
			CreatedAt:          item.CreatedAt,
			ReversedAt:         item.ReversedAt,
		})
	}
	return DB(ctx, r.db).Create(&models).Error
}

func (r *FundingRepository) ListBatchItemsForUpdate(ctx context.Context, fundingBatchID string) ([]fundingdomain.BatchItem, error) {
	var models []FundingBatchItemModel
	if err := DB(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("funding_batch_id = ?", fundingBatchID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]fundingdomain.BatchItem, 0, len(models))
	for _, model := range models {
		out = append(out, fundingdomain.BatchItem{
			FundingBatchID:     model.FundingBatchID,
			PositionID:         model.PositionID,
			UserID:             model.UserID,
			FundingFee:         model.FundingFee,
			LedgerTxID:         model.LedgerTxID,
			ReversalLedgerTxID: model.ReversalLedgerTxID,
			Status:             model.Status,
			CreatedAt:          model.CreatedAt.UTC(),
			ReversedAt:         model.ReversedAt,
		})
	}
	return out, nil
}

func (r *FundingRepository) UpdateBatchItem(ctx context.Context, item fundingdomain.BatchItem) error {
	return DB(ctx, r.db).Model(&FundingBatchItemModel{}).
		Where("funding_batch_id = ? AND position_id = ?", item.FundingBatchID, item.PositionID).
		Updates(map[string]any{
			"ledger_tx_id":          item.LedgerTxID,
			"reversal_ledger_tx_id": item.ReversalLedgerTxID,
			"status":                item.Status,
			"reversed_at":           item.ReversedAt,
		}).Error
}

func (r *FundingRepository) ListOpenPositionsForUpdate(ctx context.Context, symbolID uint64) ([]fundingdomain.PositionSnapshot, error) {
	var rows []struct {
		PositionModel
		Symbol             string `gorm:"column:symbol"`
		ContractMultiplier string `gorm:"column:contract_multiplier"`
	}
	if err := DB(ctx, r.db).
		Table("positions").
		Select("positions.*, symbols.symbol, symbols.contract_multiplier").
		Joins("JOIN symbols ON symbols.id = positions.symbol_id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("positions.symbol_id = ? AND positions.status = ?", symbolID, orderdomain.PositionStatusOpen).
		Order("positions.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]fundingdomain.PositionSnapshot, 0, len(rows))
	for _, row := range rows {
		out = append(out, fundingdomain.PositionSnapshot{
			PositionID:             row.PositionID,
			UserID:                 row.UserID,
			SymbolID:               row.SymbolID,
			Symbol:                 row.Symbol,
			Side:                   row.Side,
			Qty:                    row.Qty,
			ContractMultiplier:     row.ContractMultiplier,
			AvgEntryPrice:          row.AvgEntryPrice,
			ExistingFundingAccrual: row.FundingAccrual,
		})
	}
	return out, nil
}

func (r *FundingRepository) GetPositionByIDForUpdate(ctx context.Context, positionID string) (fundingdomain.PositionSnapshot, error) {
	var row struct {
		PositionModel
		Symbol             string `gorm:"column:symbol"`
		ContractMultiplier string `gorm:"column:contract_multiplier"`
	}
	if err := DB(ctx, r.db).
		Table("positions").
		Select("positions.*, symbols.symbol, symbols.contract_multiplier").
		Joins("JOIN symbols ON symbols.id = positions.symbol_id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("positions.position_id = ?", positionID).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fundingdomain.PositionSnapshot{}, errorsx.ErrNotFound
		}
		return fundingdomain.PositionSnapshot{}, err
	}
	return fundingdomain.PositionSnapshot{
		PositionID:             row.PositionID,
		UserID:                 row.UserID,
		SymbolID:               row.SymbolID,
		Symbol:                 row.Symbol,
		Side:                   row.Side,
		Qty:                    row.Qty,
		ContractMultiplier:     row.ContractMultiplier,
		AvgEntryPrice:          row.AvgEntryPrice,
		ExistingFundingAccrual: row.FundingAccrual,
	}, nil
}

func (r *FundingRepository) UpdatePositionFundingAccrual(ctx context.Context, positionID string, fundingAccrual string, updatedAt time.Time) error {
	return DB(ctx, r.db).Model(&PositionModel{}).
		Where("position_id = ?", positionID).
		Updates(map[string]any{
			"funding_accrual": fundingAccrual,
			"updated_at":      updatedAt.UTC(),
		}).Error
}

func toFundingBatchDomain(model FundingBatchModel) fundingdomain.Batch {
	return fundingdomain.Batch{
		ID:              model.FundingBatchID,
		SymbolID:        model.SymbolID,
		TimeWindowStart: model.TimeWindowStart,
		TimeWindowEnd:   model.TimeWindowEnd,
		NormalizedRate:  model.NormalizedRate,
		SettlementPrice: model.SettlementPrice,
		Status:          model.Status,
		ReversedAt:      model.ReversedAt,
		ReversedBy:      model.ReversedBy,
		ReversalReason:  model.ReversalReason,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}
}
