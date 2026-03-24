package db

import (
	"context"
	"errors"
	"strings"

	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	marketcache "github.com/xiaobao/rgperp/backend/internal/infra/marketcache"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OrderExecutionRepository struct {
	db          *gorm.DB
	latestCache *marketcache.Cache
	outbox      *OutboxRepository
}

func NewOrderExecutionRepository(db *gorm.DB, latestCache *marketcache.Cache) *OrderExecutionRepository {
	return &OrderExecutionRepository{db: db, latestCache: latestCache, outbox: NewOutboxRepository(db)}
}

func (r *OrderExecutionRepository) GetTradableSymbol(ctx context.Context, symbol string) (orderdomain.TradableSymbol, error) {
	if r.latestCache != nil {
		if cached, err := r.latestCache.GetTradableSymbol(ctx, symbol); err == nil {
			return cached, nil
		}
	}
	var symbolModel SymbolModel
	if err := DB(ctx, r.db).Where("symbol = ?", symbol).First(&symbolModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return orderdomain.TradableSymbol{}, errorsx.ErrNotFound
		}
		return orderdomain.TradableSymbol{}, err
	}

	tiersBySymbol, err := loadRiskTiersBySymbol(ctx, DB(ctx, r.db), []uint64{symbolModel.ID})
	if err != nil {
		return orderdomain.TradableSymbol{}, err
	}
	riskTiers := toOrderRiskTiers(tiersBySymbol[symbolModel.ID])
	if len(riskTiers) == 0 {
		return orderdomain.TradableSymbol{}, errorsx.ErrNotFound
	}

	var mark MarkPriceSnapshotModel
	if err := DB(ctx, r.db).Where("symbol_id = ?", symbolModel.ID).Order("id DESC").First(&mark).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return orderdomain.TradableSymbol{}, errorsx.ErrNotFound
		}
		return orderdomain.TradableSymbol{}, err
	}

	var sourceRows []MarketPriceSnapshotModel
	if err := DB(ctx, r.db).Raw(`
SELECT m1.*
FROM market_price_snapshots m1
JOIN (
  SELECT source_name, MAX(id) AS max_id
  FROM market_price_snapshots
  WHERE symbol_id = ?
  GROUP BY source_name
) latest ON latest.max_id = m1.id
WHERE m1.symbol_id = ?
`, symbolModel.ID, symbolModel.ID).Scan(&sourceRows).Error; err != nil {
		return orderdomain.TradableSymbol{}, err
	}
	bestBid, bestAsk := deriveBestBidAsk(sourceRows)

	return orderdomain.TradableSymbol{
		SymbolID:              symbolModel.ID,
		Symbol:                symbolModel.Symbol,
		ContractMultiplier:    symbolModel.ContractMultiplier,
		TickSize:              symbolModel.TickSize,
		StepSize:              symbolModel.StepSize,
		MinNotional:           symbolModel.MinNotional,
		Status:                symbolModel.Status,
		SessionPolicy:         symbolModel.SessionPolicy,
		IndexPrice:            mark.IndexPrice,
		MarkPrice:             mark.MarkPrice,
		BestBid:               bestBid,
		BestAsk:               bestAsk,
		InitialMarginRate:     riskTiers[0].InitialMarginRate,
		MaintenanceMarginRate: riskTiers[0].MaintenanceRate,
		RiskTiers:             riskTiers,
		SnapshotTS:            mark.CreatedAt,
	}, nil
}

func toOrderRiskTiers(models []RiskTierModel) []orderdomain.RiskTier {
	out := make([]orderdomain.RiskTier, 0, len(models))
	for _, model := range models {
		out = append(out, orderdomain.RiskTier{
			TierLevel:          model.TierLevel,
			MaxNotional:        model.MaxNotional,
			MaxLeverage:        model.MaxLeverage,
			InitialMarginRate:  model.IMR,
			MaintenanceRate:    model.MMR,
			LiquidationFeeRate: model.LiquidationFeeRate,
		})
	}
	return out
}

func (r *OrderExecutionRepository) GetByUserClientOrderID(ctx context.Context, userID uint64, clientOrderID string) (orderdomain.Order, error) {
	var row OrderModel
	if err := DB(ctx, r.db).
		Where("user_id = ? AND client_order_id = ?", userID, clientOrderID).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return orderdomain.Order{}, errorsx.ErrNotFound
		}
		return orderdomain.Order{}, err
	}
	return toOrderDomain(row, ""), nil
}

func (r *OrderExecutionRepository) GetByUserOrderIDForUpdate(ctx context.Context, userID uint64, orderID string) (orderdomain.Order, error) {
	var row OrderModel
	if err := DB(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND order_id = ?", userID, orderID).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return orderdomain.Order{}, errorsx.ErrNotFound
		}
		return orderdomain.Order{}, err
	}
	return toOrderDomain(row, ""), nil
}

func (r *OrderExecutionRepository) ListRestingOpenLimitOrders(ctx context.Context, limit int) ([]orderdomain.Order, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []struct {
		OrderModel
		Symbol string `gorm:"column:symbol"`
	}
	if err := DB(ctx, r.db).
		Table("orders").
		Select("orders.*, symbols.symbol").
		Joins("JOIN symbols ON symbols.id = orders.symbol_id").
		Where("orders.status = ? AND orders.type = ? AND orders.position_effect = ?", orderdomain.OrderStatusResting, orderdomain.OrderTypeLimit, orderdomain.PositionEffectOpen).
		Order("orders.created_at ASC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]orderdomain.Order, 0, len(rows))
	for _, row := range rows {
		out = append(out, toOrderDomain(row.OrderModel, row.Symbol))
	}
	return out, nil
}

func (r *OrderExecutionRepository) ListTriggerWaitingOrders(ctx context.Context, limit int) ([]orderdomain.Order, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []struct {
		OrderModel
		Symbol string `gorm:"column:symbol"`
	}
	if err := DB(ctx, r.db).
		Table("orders").
		Select("orders.*, symbols.symbol").
		Joins("JOIN symbols ON symbols.id = orders.symbol_id").
		Where("orders.status = ? AND orders.type IN ?", orderdomain.OrderStatusTriggerWait, []string{orderdomain.OrderTypeStopMarket, orderdomain.OrderTypeTakeProfitMarket}).
		Order("orders.created_at ASC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]orderdomain.Order, 0, len(rows))
	for _, row := range rows {
		out = append(out, toOrderDomain(row.OrderModel, row.Symbol))
	}
	return out, nil
}

func (r *OrderExecutionRepository) LockSymbolForUpdate(ctx context.Context, symbolID uint64) error {
	var symbol SymbolModel
	if err := DB(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", symbolID).
		Take(&symbol).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errorsx.ErrNotFound
		}
		return err
	}
	return nil
}

func (r *OrderExecutionRepository) GetLatestRiskLevelForUpdate(ctx context.Context, userID uint64) (string, error) {
	var row RiskSnapshotModel
	if err := DB(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Order("id DESC").
		Where("user_id = ?", userID).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errorsx.ErrNotFound
		}
		return "", err
	}
	return row.RiskLevel, nil
}

func (r *OrderExecutionRepository) GetSymbolExposureForUpdate(ctx context.Context, symbolID uint64) (orderdomain.SymbolExposure, error) {
	var row struct {
		LongQty  string `gorm:"column:long_qty"`
		ShortQty string `gorm:"column:short_qty"`
	}
	if err := DB(ctx, r.db).
		Table("positions").
		Select(`
COALESCE(SUM(CASE WHEN side = ? THEN CAST(qty AS DECIMAL(38,18)) ELSE 0 END), 0) AS long_qty,
COALESCE(SUM(CASE WHEN side = ? THEN CAST(qty AS DECIMAL(38,18)) ELSE 0 END), 0) AS short_qty
`, orderdomain.PositionSideLong, orderdomain.PositionSideShort).
		Where("symbol_id = ? AND status = ?", symbolID, orderdomain.PositionStatusOpen).
		Scan(&row).Error; err != nil {
		return orderdomain.SymbolExposure{}, err
	}
	return orderdomain.SymbolExposure{
		SymbolID: symbolID,
		LongQty:  strings.TrimSpace(row.LongQty),
		ShortQty: strings.TrimSpace(row.ShortQty),
	}, nil
}

func (r *OrderExecutionRepository) CreateOrder(ctx context.Context, order orderdomain.Order) error {
	return DB(ctx, r.db).Create(&OrderModel{
		OrderID:             order.OrderID,
		ClientOrderID:       order.ClientOrderID,
		UserID:              order.UserID,
		SymbolID:            order.SymbolID,
		Side:                order.Side,
		PositionEffect:      order.PositionEffect,
		Type:                order.Type,
		TimeInForce:         order.TimeInForce,
		Price:               order.Price,
		TriggerPrice:        order.TriggerPrice,
		Qty:                 order.Qty,
		FilledQty:           order.FilledQty,
		AvgFillPrice:        order.AvgFillPrice,
		Leverage:            order.Leverage,
		MarginMode:          order.MarginMode,
		ReduceOnly:          order.ReduceOnly,
		MaxSlippageBps:      order.MaxSlippageBps,
		Status:              order.Status,
		RejectReason:        order.RejectReason,
		FrozenInitialMargin: order.FrozenInitialMargin,
		FrozenFee:           order.FrozenFee,
		FrozenMargin:        order.FrozenMargin,
		CreatedAt:           order.CreatedAt,
		UpdatedAt:           order.UpdatedAt,
	}).Error
}

func (r *OrderExecutionRepository) UpdateOrder(ctx context.Context, order orderdomain.Order) error {
	return DB(ctx, r.db).Model(&OrderModel{}).
		Where("order_id = ?", order.OrderID).
		Updates(map[string]any{
			"filled_qty":            order.FilledQty,
			"avg_fill_price":        order.AvgFillPrice,
			"leverage":              order.Leverage,
			"status":                order.Status,
			"reject_reason":         order.RejectReason,
			"frozen_initial_margin": order.FrozenInitialMargin,
			"frozen_fee":            order.FrozenFee,
			"frozen_margin":         order.FrozenMargin,
			"updated_at":            order.UpdatedAt,
		}).Error
}

func (r *OrderExecutionRepository) CreateFill(ctx context.Context, fill orderdomain.Fill) error {
	return DB(ctx, r.db).Create(&FillModel{
		FillID:     fill.FillID,
		OrderID:    fill.OrderID,
		UserID:     fill.UserID,
		SymbolID:   fill.SymbolID,
		Side:       fill.Side,
		Qty:        fill.Qty,
		Price:      fill.Price,
		FeeAmount:  fill.FeeAmount,
		LedgerTxID: fill.LedgerTxID,
		CreatedAt:  fill.CreatedAt,
	}).Error
}

func (r *OrderExecutionRepository) CreateEvent(ctx context.Context, event orderdomain.Event) error {
	return r.outbox.Create(ctx, OutboxMessage{
		EventID:       event.EventID,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		EventType:     event.EventType,
		Payload:       event.Payload,
		Status:        "PENDING",
		CreatedAt:     event.CreatedAt,
	})
}

func (r *OrderExecutionRepository) GetPositionForUpdate(ctx context.Context, userID uint64, symbolID uint64, side string, marginMode string) (orderdomain.Position, error) {
	var row PositionModel
	if err := DB(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND symbol_id = ? AND side = ? AND margin_mode = ?", userID, symbolID, side, marginModeOrDefault(marginMode)).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return orderdomain.Position{}, errorsx.ErrNotFound
		}
		return orderdomain.Position{}, err
	}
	return toPositionDomain(row), nil
}

func (r *OrderExecutionRepository) UpsertPosition(ctx context.Context, position orderdomain.Position) error {
	model := PositionModel{
		PositionID:        position.PositionID,
		UserID:            position.UserID,
		SymbolID:          position.SymbolID,
		Side:              position.Side,
		MarginMode:        marginModeOrDefault(position.MarginMode),
		Qty:               position.Qty,
		AvgEntryPrice:     position.AvgEntryPrice,
		MarkPrice:         position.MarkPrice,
		Notional:          position.Notional,
		Leverage:          position.Leverage,
		InitialMargin:     position.InitialMargin,
		MaintenanceMargin: position.MaintenanceMargin,
		RealizedPnL:       position.RealizedPnL,
		UnrealizedPnL:     position.UnrealizedPnL,
		FundingAccrual:    position.FundingAccrual,
		LiquidationPrice:  position.LiquidationPrice,
		BankruptcyPrice:   position.BankruptcyPrice,
		Status:            position.Status,
		CreatedAt:         position.CreatedAt,
		UpdatedAt:         position.UpdatedAt,
	}
	return DB(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "symbol_id"}, {Name: "side"}, {Name: "margin_mode"}},
		DoUpdates: clause.Assignments(map[string]any{
			"qty":                model.Qty,
			"avg_entry_price":    model.AvgEntryPrice,
			"mark_price":         model.MarkPrice,
			"notional":           model.Notional,
			"leverage":           model.Leverage,
			"initial_margin":     model.InitialMargin,
			"maintenance_margin": model.MaintenanceMargin,
			"realized_pnl":       model.RealizedPnL,
			"unrealized_pnl":     model.UnrealizedPnL,
			"funding_accrual":    model.FundingAccrual,
			"liquidation_price":  model.LiquidationPrice,
			"bankruptcy_price":   model.BankruptcyPrice,
			"status":             model.Status,
			"updated_at":         model.UpdatedAt,
		}),
	}).Create(&model).Error
}

func marginModeOrDefault(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case orderdomain.MarginModeIsolated:
		return orderdomain.MarginModeIsolated
	default:
		return orderdomain.MarginModeCross
	}
}

func toOrderDomain(model OrderModel, symbol string) orderdomain.Order {
	return orderdomain.Order{
		OrderID:             model.OrderID,
		ClientOrderID:       model.ClientOrderID,
		UserID:              model.UserID,
		SymbolID:            model.SymbolID,
		Symbol:              symbol,
		Side:                model.Side,
		PositionEffect:      model.PositionEffect,
		Type:                model.Type,
		TimeInForce:         model.TimeInForce,
		Price:               model.Price,
		TriggerPrice:        model.TriggerPrice,
		Qty:                 model.Qty,
		FilledQty:           model.FilledQty,
		AvgFillPrice:        model.AvgFillPrice,
		Leverage:            model.Leverage,
		MarginMode:          marginModeOrDefault(model.MarginMode),
		ReduceOnly:          model.ReduceOnly,
		MaxSlippageBps:      model.MaxSlippageBps,
		Status:              model.Status,
		RejectReason:        model.RejectReason,
		FrozenInitialMargin: model.FrozenInitialMargin,
		FrozenFee:           model.FrozenFee,
		FrozenMargin:        model.FrozenMargin,
		CreatedAt:           model.CreatedAt,
		UpdatedAt:           model.UpdatedAt,
	}
}

func toPositionDomain(model PositionModel) orderdomain.Position {
	return orderdomain.Position{
		PositionID:        model.PositionID,
		UserID:            model.UserID,
		SymbolID:          model.SymbolID,
		Side:              model.Side,
		Qty:               model.Qty,
		AvgEntryPrice:     model.AvgEntryPrice,
		MarkPrice:         model.MarkPrice,
		Notional:          model.Notional,
		Leverage:          model.Leverage,
		MarginMode:        marginModeOrDefault(model.MarginMode),
		InitialMargin:     model.InitialMargin,
		MaintenanceMargin: model.MaintenanceMargin,
		RealizedPnL:       model.RealizedPnL,
		UnrealizedPnL:     model.UnrealizedPnL,
		FundingAccrual:    model.FundingAccrual,
		LiquidationPrice:  model.LiquidationPrice,
		BankruptcyPrice:   model.BankruptcyPrice,
		Status:            model.Status,
		CreatedAt:         model.CreatedAt,
		UpdatedAt:         model.UpdatedAt,
	}
}
