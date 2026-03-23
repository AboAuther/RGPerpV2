package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	fundingdomain "github.com/xiaobao/rgperp/backend/internal/domain/funding"
	marketdomain "github.com/xiaobao/rgperp/backend/internal/domain/market"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	marketcache "github.com/xiaobao/rgperp/backend/internal/infra/marketcache"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/positionrisk"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MarketCatalogRepository struct {
	db *gorm.DB
}

func NewMarketCatalogRepository(db *gorm.DB) *MarketCatalogRepository {
	return &MarketCatalogRepository{db: db}
}

func (r *MarketCatalogRepository) UpsertSymbols(ctx context.Context, symbols []marketdomain.Symbol) error {
	now := time.Now().UTC()
	for _, symbol := range symbols {
		model := SymbolModel{
			Symbol:             symbol.Symbol,
			AssetClass:         symbol.AssetClass,
			BaseAsset:          symbol.BaseAsset,
			QuoteAsset:         symbol.QuoteAsset,
			ContractMultiplier: symbol.ContractMultiplier,
			TickSize:           symbol.TickSize,
			StepSize:           symbol.StepSize,
			MinNotional:        symbol.MinNotional,
			Status:             symbol.Status,
			SessionPolicy:      symbol.SessionPolicy,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := DB(ctx, r.db).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "symbol"}},
			DoUpdates: clause.Assignments(map[string]any{
				"asset_class":         model.AssetClass,
				"base_asset":          model.BaseAsset,
				"quote_asset":         model.QuoteAsset,
				"contract_multiplier": model.ContractMultiplier,
				"tick_size":           model.TickSize,
				"step_size":           model.StepSize,
				"min_notional":        model.MinNotional,
				"status":              model.Status,
				"session_policy":      model.SessionPolicy,
				"updated_at":          now,
			}),
		}).Create(&model).Error; err != nil {
			return err
		}
		var created SymbolModel
		if err := DB(ctx, r.db).Where("symbol = ?", symbol.Symbol).First(&created).Error; err != nil {
			return err
		}
		for _, mapping := range symbol.Mappings {
			mappingModel := SymbolMappingModel{
				SymbolID:     created.ID,
				SourceName:   mapping.SourceName,
				SourceSymbol: mapping.SourceSymbol,
				PriceScale:   mapping.PriceScale,
				QtyScale:     mapping.QtyScale,
				Status:       mapping.Status,
			}
			if err := DB(ctx, r.db).Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "symbol_id"}, {Name: "source_name"}},
				DoUpdates: clause.Assignments(map[string]any{
					"source_symbol": mappingModel.SourceSymbol,
					"price_scale":   mappingModel.PriceScale,
					"qty_scale":     mappingModel.QtyScale,
					"status":        mappingModel.Status,
				}),
			}).Create(&mappingModel).Error; err != nil {
				return err
			}
		}
		if err := r.ensureDefaultRiskTier(ctx, created.ID, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *MarketCatalogRepository) ensureDefaultRiskTier(ctx context.Context, symbolID uint64, now time.Time) error {
	model := RiskTierModel{
		SymbolID:           symbolID,
		TierLevel:          1,
		MaxNotional:        "1000000",
		MaxLeverage:        "20",
		IMR:                "0.1",
		MMR:                "0.05",
		LiquidationFeeRate: "0.005",
		CreatedAt:          now,
	}
	return DB(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "symbol_id"}, {Name: "tier_level"}},
		DoNothing: true,
	}).Create(&model).Error
}

func (r *MarketCatalogRepository) ListActiveSymbols(ctx context.Context) ([]marketdomain.Symbol, error) {
	var models []SymbolModel
	if err := DB(ctx, r.db).Where("status IN ?", []string{"TRADING", "REDUCE_ONLY", "PAUSED"}).Order("symbol ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, nil
	}
	ids := make([]uint64, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	var mappingModels []SymbolMappingModel
	if err := DB(ctx, r.db).Where("symbol_id IN ?", ids).Order("source_name ASC").Find(&mappingModels).Error; err != nil {
		return nil, err
	}
	mappingsBySymbol := make(map[uint64][]marketdomain.SymbolMapping)
	for _, model := range mappingModels {
		mappingsBySymbol[model.SymbolID] = append(mappingsBySymbol[model.SymbolID], marketdomain.SymbolMapping{
			SourceName:   model.SourceName,
			SourceSymbol: model.SourceSymbol,
			PriceScale:   model.PriceScale,
			QtyScale:     model.QtyScale,
			Status:       model.Status,
		})
	}
	out := make([]marketdomain.Symbol, 0, len(models))
	for _, model := range models {
		out = append(out, marketdomain.Symbol{
			ID:                 model.ID,
			Symbol:             model.Symbol,
			AssetClass:         model.AssetClass,
			BaseAsset:          model.BaseAsset,
			QuoteAsset:         model.QuoteAsset,
			ContractMultiplier: model.ContractMultiplier,
			TickSize:           model.TickSize,
			StepSize:           model.StepSize,
			MinNotional:        model.MinNotional,
			Status:             model.Status,
			SessionPolicy:      model.SessionPolicy,
			Mappings:           mappingsBySymbol[model.ID],
		})
	}
	return out, nil
}

type MarketSnapshotRepository struct {
	db          *gorm.DB
	latestCache *marketcache.Cache
}

func NewMarketSnapshotRepository(db *gorm.DB, latestCache *marketcache.Cache) *MarketSnapshotRepository {
	return &MarketSnapshotRepository{db: db, latestCache: latestCache}
}

func (r *MarketSnapshotRepository) AppendSourceSnapshots(ctx context.Context, snapshots []marketdomain.SourcePriceSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	models := make([]MarketPriceSnapshotModel, 0, len(snapshots))
	for _, snapshot := range snapshots {
		models = append(models, MarketPriceSnapshotModel{
			SymbolID:   snapshot.SymbolID,
			SourceName: snapshot.SourceName,
			Bid:        snapshot.Bid,
			Ask:        snapshot.Ask,
			Last:       snapshot.Last,
			Mid:        snapshot.Mid,
			SourceTS:   snapshot.SourceTS,
			ReceivedTS: snapshot.ReceivedTS,
			CreatedAt:  snapshot.CanonicalTS,
		})
	}
	return DB(ctx, r.db).Create(&models).Error
}

func (r *MarketSnapshotRepository) ApplyAggregatedState(ctx context.Context, snapshots []marketdomain.AggregatedPrice, runtimeStates []marketdomain.SymbolRuntimeState) error {
	if err := DB(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		if len(snapshots) > 0 {
			models := make([]MarkPriceSnapshotModel, 0, len(snapshots))
			for _, snapshot := range snapshots {
				models = append(models, MarkPriceSnapshotModel{
					SymbolID:    snapshot.SymbolID,
					IndexPrice:  snapshot.IndexPrice,
					MarkPrice:   snapshot.MarkPrice,
					CalcVersion: snapshot.CalcVersion,
					CreatedAt:   snapshot.CreatedAt,
				})
			}
			if err := tx.Create(&models).Error; err != nil {
				return err
			}
		}

		if err := applySymbolRuntimeStates(tx, runtimeStates); err != nil {
			return err
		}
		if len(snapshots) > 0 {
			if err := refreshOpenPositions(tx, snapshots); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	r.refreshLatestCache(ctx, snapshots, runtimeStates)
	return nil
}

func applySymbolRuntimeStates(tx *gorm.DB, states []marketdomain.SymbolRuntimeState) error {
	if len(states) == 0 {
		return nil
	}
	for _, state := range states {
		if state.DesiredStatus == "" {
			continue
		}
		if err := tx.Model(&SymbolModel{}).
			Where("id = ? AND status IN ?", state.SymbolID, []string{"TRADING", "REDUCE_ONLY"}).
			Update("status", state.DesiredStatus).Error; err != nil {
			return err
		}
	}
	return nil
}

func refreshOpenPositions(tx *gorm.DB, snapshots []marketdomain.AggregatedPrice) error {
	if len(snapshots) == 0 {
		return nil
	}

	bySymbol := make(map[uint64]marketdomain.AggregatedPrice, len(snapshots))
	symbolIDs := make([]uint64, 0, len(snapshots))
	for _, snapshot := range snapshots {
		bySymbol[snapshot.SymbolID] = snapshot
		symbolIDs = append(symbolIDs, snapshot.SymbolID)
	}

	var symbols []SymbolModel
	if err := tx.Where("id IN ?", symbolIDs).Find(&symbols).Error; err != nil {
		return err
	}
	symbolByID := make(map[uint64]SymbolModel, len(symbols))
	for _, symbol := range symbols {
		symbolByID[symbol.ID] = symbol
	}

	tierBySymbolID, err := loadRiskTiersBySymbol(context.Background(), tx, symbolIDs)
	if err != nil {
		return err
	}

	var positions []PositionModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("symbol_id IN ? AND status = ?", symbolIDs, orderdomain.PositionStatusOpen).
		Find(&positions).Error; err != nil {
		return err
	}

	for _, position := range positions {
		snapshot, ok := bySymbol[position.SymbolID]
		if !ok {
			continue
		}
		symbol, ok := symbolByID[position.SymbolID]
		if !ok {
			return fmt.Errorf("symbol metadata missing for symbol_id=%d", position.SymbolID)
		}
		tiers, ok := tierBySymbolID[position.SymbolID]
		if !ok || len(tiers) == 0 {
			return fmt.Errorf("risk tier missing for symbol_id=%d", position.SymbolID)
		}

		qty := decimalx.MustFromString(position.Qty)
		mark := decimalx.MustFromString(snapshot.MarkPrice)
		entry := decimalx.MustFromString(position.AvgEntryPrice)
		multiplier := decimalx.MustFromString(symbol.ContractMultiplier)
		sign := decimalx.MustFromString("1")
		if position.Side == orderdomain.PositionSideShort {
			sign = decimalx.MustFromString("-1")
		}
		notional := qty.Mul(mark).Mul(multiplier)
		tier, err := selectRiskTierByNotional(tiers, notional)
		if err != nil {
			return err
		}
		unrealized := sign.Mul(qty).Mul(mark.Sub(entry)).Mul(multiplier)
		maintenance := notional.Mul(decimalx.MustFromString(tier.MMR))
		liquidationPrice, bankruptcyPrice := positionrisk.ComputeDisplayPrices(
			position.Side,
			position.Qty,
			position.AvgEntryPrice,
			position.InitialMargin,
			tier.MMR,
			tier.LiquidationFeeRate,
			symbol.ContractMultiplier,
		)

		if err := tx.Model(&PositionModel{}).
			Where("id = ?", position.ID).
			Updates(map[string]any{
				"mark_price":         mark.String(),
				"notional":           notional.String(),
				"maintenance_margin": maintenance.String(),
				"unrealized_pnl":     unrealized.String(),
				"liquidation_price":  liquidationPrice,
				"bankruptcy_price":   bankruptcyPrice,
				"updated_at":         snapshot.CreatedAt,
			}).Error; err != nil {
			return err
		}
	}

	return nil
}

type MarketReadRepository struct {
	db          *gorm.DB
	latestCache *marketcache.Cache
	maxDataAge  time.Duration
}

func NewMarketReadRepository(db *gorm.DB, latestCache *marketcache.Cache, maxDataAge time.Duration) *MarketReadRepository {
	return &MarketReadRepository{db: db, latestCache: latestCache, maxDataAge: maxDataAge}
}

func (r *MarketReadRepository) ListSymbols(ctx context.Context) ([]readmodel.SymbolItem, error) {
	var models []SymbolModel
	if err := DB(ctx, r.db).Order("symbol ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]readmodel.SymbolItem, 0, len(models))
	for _, model := range models {
		out = append(out, readmodel.SymbolItem{
			Symbol:      model.Symbol,
			AssetClass:  model.AssetClass,
			TickSize:    model.TickSize,
			StepSize:    model.StepSize,
			MinNotional: model.MinNotional,
			Status:      model.Status,
		})
	}
	return out, nil
}

func (r *MarketReadRepository) ListTickers(ctx context.Context) ([]readmodel.TickerItem, error) {
	if r.latestCache != nil {
		if cached, err := r.latestCache.ListTickers(ctx); err == nil && len(cached) > 0 {
			return cached, nil
		}
	}
	var symbols []SymbolModel
	if err := DB(ctx, r.db).Order("symbol ASC").Find(&symbols).Error; err != nil {
		return nil, err
	}
	if len(symbols) == 0 {
		return []readmodel.TickerItem{}, nil
	}
	symbolIDs := make([]uint64, 0, len(symbols))
	for _, symbol := range symbols {
		symbolIDs = append(symbolIDs, symbol.ID)
	}

	var markRows []MarkPriceSnapshotModel
	if err := DB(ctx, r.db).Raw(`
SELECT m1.*
FROM mark_price_snapshots m1
JOIN (
  SELECT symbol_id, MAX(id) AS max_id
  FROM mark_price_snapshots
  WHERE symbol_id IN ?
  GROUP BY symbol_id
) latest ON latest.max_id = m1.id
`, symbolIDs).Scan(&markRows).Error; err != nil {
		return nil, err
	}
	markBySymbol := make(map[uint64]MarkPriceSnapshotModel, len(markRows))
	for _, row := range markRows {
		markBySymbol[row.SymbolID] = row
	}

	var rawRows []MarketPriceSnapshotModel
	if err := DB(ctx, r.db).Raw(`
SELECT m1.*
FROM market_price_snapshots m1
JOIN (
  SELECT symbol_id, source_name, MAX(id) AS max_id
  FROM market_price_snapshots
  WHERE symbol_id IN ?
  GROUP BY symbol_id, source_name
) latest ON latest.max_id = m1.id
`, symbolIDs).Scan(&rawRows).Error; err != nil {
		return nil, err
	}
	rawBySymbol := make(map[uint64][]MarketPriceSnapshotModel)
	for _, row := range rawRows {
		rawBySymbol[row.SymbolID] = append(rawBySymbol[row.SymbolID], row)
	}

	out := make([]readmodel.TickerItem, 0, len(symbols))
	for _, symbol := range symbols {
		bestBid, bestAsk := deriveBestBidAsk(rawBySymbol[symbol.ID])
		mark, ok := markBySymbol[symbol.ID]
		if !ok {
			out = append(out, readmodel.TickerItem{
				Symbol:     symbol.Symbol,
				IndexPrice: "0",
				MarkPrice:  "0",
				BestBid:    bestBid,
				BestAsk:    bestAsk,
				Status:     symbol.Status,
				Stale:      true,
				TS:         "",
			})
			continue
		}
		out = append(out, readmodel.TickerItem{
			Symbol:     symbol.Symbol,
			IndexPrice: mark.IndexPrice,
			MarkPrice:  mark.MarkPrice,
			BestBid:    bestBid,
			BestAsk:    bestAsk,
			Status:     symbol.Status,
			Stale:      r.maxDataAge > 0 && time.Since(mark.CreatedAt) > r.maxDataAge,
			TS:         mark.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func (r *MarketSnapshotRepository) refreshLatestCache(ctx context.Context, snapshots []marketdomain.AggregatedPrice, runtimeStates []marketdomain.SymbolRuntimeState) {
	if r.latestCache == nil {
		return
	}

	symbolSet := make(map[uint64]struct{}, len(snapshots)+len(runtimeStates))
	for _, snapshot := range snapshots {
		symbolSet[snapshot.SymbolID] = struct{}{}
	}
	for _, state := range runtimeStates {
		symbolSet[state.SymbolID] = struct{}{}
	}
	if len(symbolSet) == 0 {
		return
	}

	symbolIDs := make([]uint64, 0, len(symbolSet))
	for symbolID := range symbolSet {
		symbolIDs = append(symbolIDs, symbolID)
	}

	var symbols []SymbolModel
	if err := r.db.Where("id IN ?", symbolIDs).Find(&symbols).Error; err != nil {
		return
	}
	if len(symbols) == 0 {
		return
	}
	symbolByID := make(map[uint64]SymbolModel, len(symbols))
	for _, symbol := range symbols {
		symbolByID[symbol.ID] = symbol
	}

	tierBySymbol, err := loadRiskTiersBySymbol(ctx, r.db.WithContext(ctx), symbolIDs)
	if err != nil {
		return
	}

	var markRows []MarkPriceSnapshotModel
	if err := r.db.Raw(`
SELECT m1.*
FROM mark_price_snapshots m1
JOIN (
  SELECT symbol_id, MAX(id) AS max_id
  FROM mark_price_snapshots
  WHERE symbol_id IN ?
  GROUP BY symbol_id
) latest ON latest.max_id = m1.id
`, symbolIDs).Scan(&markRows).Error; err != nil {
		return
	}
	markBySymbol := make(map[uint64]MarkPriceSnapshotModel, len(markRows))
	for _, row := range markRows {
		markBySymbol[row.SymbolID] = row
	}

	var rawRows []MarketPriceSnapshotModel
	if err := r.db.Raw(`
SELECT m1.*
FROM market_price_snapshots m1
JOIN (
  SELECT symbol_id, source_name, MAX(id) AS max_id
  FROM market_price_snapshots
  WHERE symbol_id IN ?
  GROUP BY symbol_id, source_name
) latest ON latest.max_id = m1.id
`, symbolIDs).Scan(&rawRows).Error; err != nil {
		return
	}
	rawBySymbol := make(map[uint64][]MarketPriceSnapshotModel)
	for _, row := range rawRows {
		rawBySymbol[row.SymbolID] = append(rawBySymbol[row.SymbolID], row)
	}

	cacheSnapshots := make([]marketcache.Snapshot, 0, len(symbols))
	for _, symbol := range symbols {
		mark, ok := markBySymbol[symbol.ID]
		if !ok {
			continue
		}
		tiers, ok := tierBySymbol[symbol.ID]
		if !ok || len(tiers) == 0 {
			continue
		}
		tier := tiers[0]
		bestBid, bestAsk := deriveBestBidAsk(rawBySymbol[symbol.ID])
		cacheSnapshots = append(cacheSnapshots, marketcache.Snapshot{
			SymbolID:              symbol.ID,
			Symbol:                symbol.Symbol,
			Status:                symbol.Status,
			ContractMultiplier:    symbol.ContractMultiplier,
			TickSize:              symbol.TickSize,
			StepSize:              symbol.StepSize,
			MinNotional:           symbol.MinNotional,
			InitialMarginRate:     tier.IMR,
			MaintenanceMarginRate: tier.MMR,
			IndexPrice:            mark.IndexPrice,
			MarkPrice:             mark.MarkPrice,
			BestBid:               bestBid,
			BestAsk:               bestAsk,
			TS:                    mark.CreatedAt,
		})
	}
	if len(cacheSnapshots) == 0 {
		return
	}
	_ = r.latestCache.StoreSnapshots(ctx, cacheSnapshots)
}

func deriveBestBidAsk(rows []MarketPriceSnapshotModel) (string, string) {
	if len(rows) == 0 {
		return "0", "0"
	}
	bestBid := rows[0].Bid
	bestAsk := rows[0].Ask
	for _, row := range rows[1:] {
		if decimalStringGreater(row.Bid, bestBid) {
			bestBid = row.Bid
		}
		if bestAsk == "0" || decimalStringLess(row.Ask, bestAsk) {
			bestAsk = row.Ask
		}
	}
	return bestBid, bestAsk
}

type TradingReadRepository struct {
	db *gorm.DB
}

func NewTradingReadRepository(db *gorm.DB) *TradingReadRepository {
	return &TradingReadRepository{db: db}
}

func (r *TradingReadRepository) ListOrders(ctx context.Context, userID uint64) ([]readmodel.OrderItem, error) {
	var rows []struct {
		OrderModel
		Symbol string `gorm:"column:symbol"`
	}
	if err := DB(ctx, r.db).
		Table("orders").
		Select("orders.*, symbols.symbol").
		Joins("JOIN symbols ON symbols.id = orders.symbol_id").
		Where("orders.user_id = ?", userID).
		Order("orders.created_at DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]readmodel.OrderItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, readmodel.OrderItem{
			OrderID:        row.OrderID,
			ClientOrderID:  row.ClientOrderID,
			Symbol:         row.Symbol,
			Side:           row.Side,
			PositionEffect: row.PositionEffect,
			Type:           row.Type,
			Qty:            row.Qty,
			FilledQty:      row.FilledQty,
			AvgFillPrice:   row.AvgFillPrice,
			Price:          row.Price,
			TriggerPrice:   row.TriggerPrice,
			ReduceOnly:     row.ReduceOnly,
			Status:         row.Status,
			RejectReason:   row.RejectReason,
			CreatedAt:      row.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func (r *TradingReadRepository) ListFills(ctx context.Context, userID uint64) ([]readmodel.FillItem, error) {
	var rows []struct {
		FillModel
		Symbol string `gorm:"column:symbol"`
	}
	if err := DB(ctx, r.db).
		Table("fills").
		Select("fills.*, symbols.symbol").
		Joins("JOIN symbols ON symbols.id = fills.symbol_id").
		Where("fills.user_id = ?", userID).
		Order("fills.created_at DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]readmodel.FillItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, readmodel.FillItem{
			FillID:    row.FillID,
			OrderID:   row.OrderID,
			Symbol:    row.Symbol,
			Side:      row.Side,
			Qty:       row.Qty,
			Price:     row.Price,
			FeeAmount: row.FeeAmount,
			CreatedAt: row.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func (r *TradingReadRepository) ListPositions(ctx context.Context, userID uint64) ([]readmodel.PositionItem, error) {
	var rows []struct {
		PositionModel
		Symbol string `gorm:"column:symbol"`
	}
	if err := DB(ctx, r.db).
		Table("positions").
		Select("positions.*, symbols.symbol").
		Joins("JOIN symbols ON symbols.id = positions.symbol_id").
		Where("positions.user_id = ?", userID).
		Order("positions.created_at DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]readmodel.PositionItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, readmodel.PositionItem{
			PositionID:        row.PositionID,
			Symbol:            row.Symbol,
			Side:              row.Side,
			Qty:               row.Qty,
			AvgEntryPrice:     row.AvgEntryPrice,
			MarkPrice:         row.MarkPrice,
			InitialMargin:     row.InitialMargin,
			MaintenanceMargin: row.MaintenanceMargin,
			RealizedPnL:       row.RealizedPnL,
			UnrealizedPnL:     row.UnrealizedPnL,
			FundingAccrual:    row.FundingAccrual,
			LiquidationPrice:  row.LiquidationPrice,
			Status:            row.Status,
		})
	}
	return out, nil
}

func (r *TradingReadRepository) ListFunding(ctx context.Context, userID uint64) ([]readmodel.FundingItem, error) {
	var rows []struct {
		FundingBatchItemModel
		Symbol         string    `gorm:"column:symbol"`
		NormalizedRate string    `gorm:"column:normalized_rate"`
		TimeWindowEnd  time.Time `gorm:"column:time_window_end"`
	}
	if err := DB(ctx, r.db).
		Table("funding_batch_items").
		Select("funding_batch_items.*, symbols.symbol, funding_batches.normalized_rate, funding_batches.time_window_end").
		Joins("JOIN funding_batches ON funding_batches.funding_batch_id = funding_batch_items.funding_batch_id").
		Joins("JOIN positions ON positions.position_id = funding_batch_items.position_id").
		Joins("JOIN symbols ON symbols.id = positions.symbol_id").
		Where("funding_batch_items.user_id = ? AND funding_batch_items.status = ?", userID, fundingdomain.ItemStatusApplied).
		Order("funding_batch_items.created_at DESC").
		Scan(&rows).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []readmodel.FundingItem{}, nil
		}
		return nil, err
	}
	out := make([]readmodel.FundingItem, 0, len(rows))
	for _, row := range rows {
		direction := "RECEIVE"
		if decimalStringLess(row.FundingFee, "0") {
			direction = "PAY"
		}
		out = append(out, readmodel.FundingItem{
			FundingID: row.FundingBatchID + ":" + row.PositionID,
			Symbol:    row.Symbol,
			Direction: direction,
			Rate:      row.NormalizedRate,
			Amount:    row.FundingFee,
			SettledAt: row.TimeWindowEnd.Format(time.RFC3339),
			BatchID:   row.FundingBatchID,
		})
	}
	return out, nil
}

func decimalStringGreater(a, b string) bool {
	return decimalStringCompare(a, b) > 0
}

func decimalStringLess(a, b string) bool {
	return decimalStringCompare(a, b) < 0
}

func decimalStringCompare(a, b string) int {
	da, errA := decimalx.NewFromString(a)
	db, errB := decimalx.NewFromString(b)
	if errA != nil || errB != nil {
		switch {
		case a > b:
			return 1
		case a < b:
			return -1
		default:
			return 0
		}
	}
	if da.String() == db.String() {
		return 0
	}
	if da.GreaterThan(db) {
		return 1
	}
	return -1
}
