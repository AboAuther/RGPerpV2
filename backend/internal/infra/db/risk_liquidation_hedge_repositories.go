package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	hedgedomain "github.com/xiaobao/rgperp/backend/internal/domain/hedge"
	liquidationdomain "github.com/xiaobao/rgperp/backend/internal/domain/liquidation"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RiskRepository struct {
	db *gorm.DB
}

func NewRiskRepository(db *gorm.DB) *RiskRepository {
	return &RiskRepository{db: db}
}

type latestMarkPriceSnapshot struct {
	SymbolID  uint64    `gorm:"column:symbol_id"`
	MarkPrice string    `gorm:"column:mark_price"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func loadLatestMarkPrices(ctx context.Context, tx *gorm.DB, symbolIDs []uint64) (map[uint64]latestMarkPriceSnapshot, error) {
	if len(symbolIDs) == 0 {
		return map[uint64]latestMarkPriceSnapshot{}, nil
	}

	var rows []latestMarkPriceSnapshot
	if err := tx.Raw(`
SELECT m1.symbol_id, m1.mark_price, m1.created_at
FROM mark_price_snapshots m1
JOIN (
  SELECT symbol_id, MAX(id) AS max_id
  FROM mark_price_snapshots
  WHERE symbol_id IN ?
  GROUP BY symbol_id
) latest ON latest.max_id = m1.id
`, symbolIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make(map[uint64]latestMarkPriceSnapshot, len(rows))
	for _, row := range rows {
		out[row.SymbolID] = row
	}
	return out, nil
}

func livePositionMetrics(side string, qty string, avgEntryPrice string, markPrice string, contractMultiplier string, initialMarginRate string, maintenanceMarginRate string) (notional string, initialMargin string, maintenanceMargin string, unrealizedPnL string) {
	qtyDecimal := decimalx.MustFromString(qty)
	entryPriceDecimal := decimalx.MustFromString(avgEntryPrice)
	markPriceDecimal := decimalx.MustFromString(markPrice)
	multiplierDecimal := decimalx.MustFromString(contractMultiplier)
	imrDecimal := decimalx.MustFromString(initialMarginRate)
	mmrDecimal := decimalx.MustFromString(maintenanceMarginRate)

	notionalDecimal := qtyDecimal.Mul(markPriceDecimal).Mul(multiplierDecimal)
	pnlSign := decimalx.MustFromString("1")
	if side == orderdomain.PositionSideShort {
		pnlSign = decimalx.MustFromString("-1")
	}
	unrealizedPnLDecimal := pnlSign.Mul(qtyDecimal).Mul(markPriceDecimal.Sub(entryPriceDecimal)).Mul(multiplierDecimal)
	initialMarginDecimal := notionalDecimal.Mul(imrDecimal)
	maintenanceMarginDecimal := notionalDecimal.Mul(mmrDecimal)

	return notionalDecimal.String(), initialMarginDecimal.String(), maintenanceMarginDecimal.String(), unrealizedPnLDecimal.String()
}

func (r *RiskRepository) GetAccountStateForUpdate(ctx context.Context, userID uint64) (riskdomain.AccountState, error) {
	return loadRiskAccountState(ctx, DB(ctx, r.db), userID, true)
}

func (r *RiskRepository) GetLatestRiskSnapshot(ctx context.Context, userID uint64) (riskdomain.Snapshot, error) {
	var model RiskSnapshotModel
	if err := DB(ctx, r.db).
		Order("id DESC").
		Where("user_id = ?", userID).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return riskdomain.Snapshot{}, errorsx.ErrNotFound
		}
		return riskdomain.Snapshot{}, err
	}
	return toRiskSnapshotDomain(model), nil
}

func (r *RiskRepository) CreateRiskSnapshot(ctx context.Context, snapshot riskdomain.Snapshot) (riskdomain.Snapshot, error) {
	model := RiskSnapshotModel{
		UserID:            snapshot.UserID,
		Equity:            snapshot.Equity,
		AvailableBalance:  snapshot.AvailableBalance,
		MaintenanceMargin: snapshot.MaintenanceMargin,
		MarginRatio:       snapshot.MarginRatio,
		RiskLevel:         snapshot.RiskLevel,
		TriggeredBy:       snapshot.TriggeredBy,
		CreatedAt:         snapshot.CreatedAt,
	}
	if err := DB(ctx, r.db).Create(&model).Error; err != nil {
		return riskdomain.Snapshot{}, err
	}
	return toRiskSnapshotDomain(model), nil
}

func (r *RiskRepository) ListUsersWithOpenPositions(ctx context.Context) ([]uint64, error) {
	var userIDs []uint64
	if err := DB(ctx, r.db).
		Table("positions").
		Distinct("user_id").
		Where("status = ?", orderdomain.PositionStatusOpen).
		Order("user_id ASC").
		Pluck("user_id", &userIDs).Error; err != nil {
		return nil, err
	}
	return userIDs, nil
}

func (r *RiskRepository) ListActiveSymbols(ctx context.Context) ([]riskdomain.HedgeState, error) {
	var rows []struct {
		ID     uint64 `gorm:"column:id"`
		Symbol string `gorm:"column:symbol"`
	}
	if err := DB(ctx, r.db).
		Table("symbols").
		Select("id, symbol").
		Where("status IN ?", []string{"TRADING", "REDUCE_ONLY"}).
		Order("id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]riskdomain.HedgeState, 0, len(rows))
	for _, row := range rows {
		out = append(out, riskdomain.HedgeState{
			SymbolID: row.ID,
			Symbol:   row.Symbol,
		})
	}
	return out, nil
}

func (r *RiskRepository) GetHedgeStateForUpdate(ctx context.Context, symbolID uint64) (riskdomain.HedgeState, error) {
	tx := DB(ctx, r.db)
	var symbol SymbolModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", symbolID).
		First(&symbol).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return riskdomain.HedgeState{}, errorsx.ErrNotFound
		}
		return riskdomain.HedgeState{}, err
	}

	var positionRows []struct {
		Side string `gorm:"column:side"`
		Qty  string `gorm:"column:qty"`
	}
	if err := tx.Table("positions").
		Select("side, qty").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("symbol_id = ? AND status = ?", symbolID, orderdomain.PositionStatusOpen).
		Scan(&positionRows).Error; err != nil {
		return riskdomain.HedgeState{}, err
	}

	var hedgeRows []HedgePositionModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("symbol = ?", symbol.Symbol).
		Find(&hedgeRows).Error; err != nil {
		return riskdomain.HedgeState{}, err
	}

	state := riskdomain.HedgeState{
		SymbolID:         symbol.ID,
		Symbol:           symbol.Symbol,
		InternalLongQty:  "0",
		InternalShortQty: "0",
		ManagedLongQty:   "0",
		ManagedShortQty:  "0",
	}
	for _, row := range positionRows {
		if row.Side == orderdomain.PositionSideLong {
			state.InternalLongQty = decimalAdd(state.InternalLongQty, row.Qty)
			continue
		}
		state.InternalShortQty = decimalAdd(state.InternalShortQty, row.Qty)
	}
	for _, row := range hedgeRows {
		if row.Side == hedgedomain.PositionSideLong {
			state.ManagedLongQty = decimalAdd(state.ManagedLongQty, row.Qty)
			continue
		}
		state.ManagedShortQty = decimalAdd(state.ManagedShortQty, row.Qty)
	}
	return state, nil
}

func (r *RiskRepository) GetLatestOpenHedgeIntentForUpdate(ctx context.Context, symbolID uint64) (hedgedomain.Intent, error) {
	var model HedgeIntentModel
	if err := DB(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Order("id DESC").
		Where("symbol_id = ? AND status IN ?", symbolID, []string{hedgedomain.IntentStatusPending, hedgedomain.IntentStatusExecuting}).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return hedgedomain.Intent{}, errorsx.ErrNotFound
		}
		return hedgedomain.Intent{}, err
	}
	return toHedgeIntentDomain(model, ""), nil
}

func (r *RiskRepository) CreateHedgeIntent(ctx context.Context, intent hedgedomain.Intent) error {
	return DB(ctx, r.db).Create(&HedgeIntentModel{
		HedgeIntentID:      intent.ID,
		SymbolID:           intent.SymbolID,
		Side:               intent.Side,
		TargetQty:          intent.TargetQty,
		CurrentNetExposure: intent.CurrentNetExposure,
		Status:             intent.Status,
		CreatedAt:          intent.CreatedAt,
		UpdatedAt:          intent.UpdatedAt,
	}).Error
}

type HedgeRepository struct {
	db *gorm.DB
}

func NewHedgeRepository(db *gorm.DB) *HedgeRepository {
	return &HedgeRepository{db: db}
}

func (r *HedgeRepository) GetIntentForUpdate(ctx context.Context, intentID string) (hedgedomain.Intent, error) {
	var row struct {
		HedgeIntentModel
		Symbol string `gorm:"column:symbol"`
	}
	if err := DB(ctx, r.db).
		Table("hedge_intents").
		Select("hedge_intents.*, symbols.symbol").
		Joins("JOIN symbols ON symbols.id = hedge_intents.symbol_id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("hedge_intents.hedge_intent_id = ?", intentID).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return hedgedomain.Intent{}, errorsx.ErrNotFound
		}
		return hedgedomain.Intent{}, err
	}
	return toHedgeIntentDomain(row.HedgeIntentModel, row.Symbol), nil
}

func (r *HedgeRepository) GetLatestOrderByIntentForUpdate(ctx context.Context, intentID string) (hedgedomain.Order, error) {
	var model HedgeOrderModel
	if err := DB(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Order("id DESC").
		Where("hedge_intent_id = ?", intentID).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return hedgedomain.Order{}, errorsx.ErrNotFound
		}
		return hedgedomain.Order{}, err
	}
	return toHedgeOrderDomain(model), nil
}

func (r *HedgeRepository) CreateOrder(ctx context.Context, order hedgedomain.Order) error {
	return DB(ctx, r.db).Create(&HedgeOrderModel{
		HedgeOrderID:  order.ID,
		HedgeIntentID: order.IntentID,
		Venue:         order.Venue,
		VenueOrderID:  order.VenueOrderID,
		Symbol:        order.Symbol,
		Side:          order.Side,
		Qty:           order.Qty,
		Price:         order.Price,
		Status:        order.Status,
		ErrorCode:     order.ErrorCode,
		CreatedAt:     order.CreatedAt,
		UpdatedAt:     order.UpdatedAt,
	}).Error
}

func (r *HedgeRepository) UpdateOrder(ctx context.Context, order hedgedomain.Order) error {
	return DB(ctx, r.db).Model(&HedgeOrderModel{}).
		Where("hedge_order_id = ?", order.ID).
		Updates(map[string]any{
			"venue_order_id": order.VenueOrderID,
			"price":          order.Price,
			"status":         order.Status,
			"error_code":     order.ErrorCode,
			"updated_at":     order.UpdatedAt,
		}).Error
}

func (r *HedgeRepository) UpdateIntent(ctx context.Context, intent hedgedomain.Intent) error {
	return DB(ctx, r.db).Model(&HedgeIntentModel{}).
		Where("hedge_intent_id = ?", intent.ID).
		Updates(map[string]any{
			"status":     intent.Status,
			"updated_at": intent.UpdatedAt,
		}).Error
}

func (r *HedgeRepository) CreateFill(ctx context.Context, fill hedgedomain.Fill) error {
	return DB(ctx, r.db).Create(&HedgeFillModel{
		HedgeFillID:  fill.ID,
		HedgeOrderID: fill.OrderID,
		VenueFillID:  fill.VenueFillID,
		Qty:          fill.Qty,
		Price:        fill.Price,
		Fee:          fill.Fee,
		CreatedAt:    fill.CreatedAt,
	}).Error
}

func (r *HedgeRepository) GetPositionForUpdate(ctx context.Context, symbol string, side string) (hedgedomain.Position, error) {
	var model HedgePositionModel
	if err := DB(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("symbol = ? AND side = ?", symbol, side).
		Take(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return hedgedomain.Position{}, errorsx.ErrNotFound
		}
		return hedgedomain.Position{}, err
	}
	return toHedgePositionDomain(model), nil
}

func (r *HedgeRepository) UpsertPosition(ctx context.Context, position hedgedomain.Position) error {
	model := HedgePositionModel{
		Symbol:        position.Symbol,
		Side:          position.Side,
		Qty:           position.Qty,
		AvgEntryPrice: position.AvgEntryPrice,
		RealizedPnL:   position.RealizedPnL,
		UnrealizedPnL: position.UnrealizedPnL,
		UpdatedAt:     position.UpdatedAt,
	}
	return DB(ctx, r.db).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "symbol"}, {Name: "side"}},
		DoUpdates: clause.Assignments(map[string]any{
			"qty":             model.Qty,
			"avg_entry_price": model.AvgEntryPrice,
			"realized_pnl":    model.RealizedPnL,
			"unrealized_pnl":  model.UnrealizedPnL,
			"updated_at":      model.UpdatedAt,
		}),
	}).Create(&model).Error
}

type LiquidationRepository struct {
	db *gorm.DB
}

func NewLiquidationRepository(db *gorm.DB) *LiquidationRepository {
	return &LiquidationRepository{db: db}
}

func (r *LiquidationRepository) GetLiquidationByIDForUpdate(ctx context.Context, liquidationID string) (liquidationdomain.Liquidation, error) {
	var model LiquidationModel
	if err := DB(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("liquidation_id = ?", liquidationID).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return liquidationdomain.Liquidation{}, errorsx.ErrNotFound
		}
		return liquidationdomain.Liquidation{}, err
	}
	return toLiquidationDomain(model), nil
}

func (r *LiquidationRepository) CreateLiquidation(ctx context.Context, liquidation liquidationdomain.Liquidation) error {
	model, err := newLiquidationModel(liquidation)
	if err != nil {
		return err
	}
	return DB(ctx, r.db).Create(model).Error
}

func (r *LiquidationRepository) UpdateLiquidation(ctx context.Context, liquidation liquidationdomain.Liquidation) error {
	model, err := newLiquidationModel(liquidation)
	if err != nil {
		return err
	}
	return DB(ctx, r.db).Model(&LiquidationModel{}).
		Where("liquidation_id = ?", liquidation.ID).
		Updates(map[string]any{
			"status":                       liquidation.Status,
			"penalty_amount":               liquidation.PenaltyAmount,
			"insurance_fund_used":          liquidation.InsuranceFundUsed,
			"bankrupt_amount":              liquidation.BankruptAmount,
			"abort_reason":                 model.AbortReason,
			"pre_account_snapshot_json":    model.PreAccountJSON,
			"post_account_snapshot_json":   model.PostAccountJSON,
			"pre_positions_snapshot_json":  model.PrePositionsJSON,
			"post_positions_snapshot_json": model.PostPositionsJSON,
			"released_orders_json":         model.ReleasedOrdersJSON,
			"price_snapshot_json":          model.PriceSnapshotJSON,
			"config_snapshot_json":         model.ConfigSnapshotJSON,
			"settlement_snapshot_json":     model.SettlementJSON,
			"updated_at":                   liquidation.UpdatedAt,
		}).Error
}

func (r *LiquidationRepository) CreateItem(ctx context.Context, item liquidationdomain.Item) error {
	return DB(ctx, r.db).Create(&LiquidationItemModel{
		LiquidationID:  item.LiquidationID,
		PositionID:     item.PositionID,
		LiquidatedQty:  item.LiquidatedQty,
		ExecutionPrice: item.ExecutionPrice,
		LedgerTxID:     item.LedgerTxID,
		CreatedAt:      item.CreatedAt,
	}).Error
}

func (r *LiquidationRepository) ListOpenPositionsForUpdate(ctx context.Context, userID uint64) ([]liquidationdomain.Position, error) {
	tx := DB(ctx, r.db)
	var rows []struct {
		PositionModel
		Symbol             string `gorm:"column:symbol"`
		ContractMultiplier string `gorm:"column:contract_multiplier"`
	}
	if err := tx.
		Table("positions").
		Select("positions.*, symbols.symbol, symbols.contract_multiplier").
		Joins("JOIN symbols ON symbols.id = positions.symbol_id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("positions.user_id = ? AND positions.status = ?", userID, orderdomain.PositionStatusOpen).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	symbolIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		symbolIDs = append(symbolIDs, row.SymbolID)
	}
	marksBySymbol, err := loadLatestMarkPrices(ctx, tx, symbolIDs)
	if err != nil {
		return nil, err
	}
	out := make([]liquidationdomain.Position, 0, len(rows))
	for _, row := range rows {
		mark, ok := marksBySymbol[row.SymbolID]
		if !ok {
			return nil, fmt.Errorf("%w: latest mark price missing for symbol %s", errorsx.ErrNotFound, row.Symbol)
		}
		out = append(out, liquidationdomain.Position{
			PositionID:         row.PositionID,
			UserID:             row.UserID,
			SymbolID:           row.SymbolID,
			Symbol:             row.Symbol,
			Side:               row.Side,
			Qty:                row.Qty,
			AvgEntryPrice:      row.AvgEntryPrice,
			MarkPrice:          mark.MarkPrice,
			Notional:           row.Notional,
			InitialMargin:      row.InitialMargin,
			MaintenanceMargin:  row.MaintenanceMargin,
			RealizedPnL:        row.RealizedPnL,
			UnrealizedPnL:      row.UnrealizedPnL,
			FundingAccrual:     row.FundingAccrual,
			LiquidationPrice:   row.LiquidationPrice,
			BankruptcyPrice:    row.BankruptcyPrice,
			ContractMultiplier: row.ContractMultiplier,
			Status:             row.Status,
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		})
	}
	return out, nil
}

func (r *LiquidationRepository) GetCoverageBalancesForUpdate(ctx context.Context, userID uint64, asset string) (liquidationdomain.CoverageBalances, error) {
	tx := DB(ctx, r.db)

	var rows []struct {
		AccountCode string `gorm:"column:account_code"`
		Balance     string `gorm:"column:balance"`
	}
	if err := tx.Table("accounts").
		Select("accounts.account_code, COALESCE(account_balance_snapshots.balance, '0') AS balance").
		Joins("LEFT JOIN account_balance_snapshots ON account_balance_snapshots.account_id = accounts.id AND account_balance_snapshots.asset = accounts.asset").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("(accounts.user_id = ? AND accounts.account_code = ? AND accounts.asset = ?) OR (accounts.user_id IS NULL AND accounts.account_code = ? AND accounts.asset = ?)", userID, "USER_WALLET", asset, "INSURANCE_FUND", asset).
		Scan(&rows).Error; err != nil {
		return liquidationdomain.CoverageBalances{}, err
	}

	balances := liquidationdomain.CoverageBalances{
		UserWalletBalance:    "0",
		InsuranceFundBalance: "0",
	}
	for _, row := range rows {
		switch row.AccountCode {
		case "USER_WALLET":
			balances.UserWalletBalance = row.Balance
		case "INSURANCE_FUND":
			balances.InsuranceFundBalance = row.Balance
		}
	}
	return balances, nil
}

func (r *LiquidationRepository) ListRiskIncreaseOrdersForUpdate(ctx context.Context, userID uint64) ([]liquidationdomain.RiskIncreaseOrder, error) {
	var rows []struct {
		OrderModel
		Symbol string `gorm:"column:symbol"`
	}
	if err := DB(ctx, r.db).
		Table("orders").
		Select("orders.*, symbols.symbol").
		Joins("JOIN symbols ON symbols.id = orders.symbol_id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("orders.user_id = ? AND orders.status = ? AND orders.position_effect = ?", userID, orderdomain.OrderStatusResting, orderdomain.PositionEffectOpen).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]liquidationdomain.RiskIncreaseOrder, 0, len(rows))
	for _, row := range rows {
		out = append(out, liquidationdomain.RiskIncreaseOrder{
			OrderID:       row.OrderID,
			ClientOrderID: row.ClientOrderID,
			UserID:        row.UserID,
			SymbolID:      row.SymbolID,
			Symbol:        row.Symbol,
			Side:          row.Side,
			Qty:           row.Qty,
			FrozenMargin:  row.FrozenMargin,
			Status:        row.Status,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
		})
	}
	return out, nil
}

func (r *LiquidationRepository) CancelOrders(ctx context.Context, orderIDs []string, updatedAt time.Time) error {
	if len(orderIDs) == 0 {
		return nil
	}
	return DB(ctx, r.db).Model(&OrderModel{}).
		Where("order_id IN ?", orderIDs).
		Updates(map[string]any{
			"status":        orderdomain.OrderStatusCanceled,
			"frozen_margin": "0",
			"updated_at":    updatedAt,
		}).Error
}

func (r *LiquidationRepository) CreateOrder(ctx context.Context, order liquidationdomain.OrderRecord) error {
	return DB(ctx, r.db).Create(&OrderModel{
		OrderID:        order.OrderID,
		ClientOrderID:  order.ClientOrderID,
		UserID:         order.UserID,
		SymbolID:       order.SymbolID,
		Side:           order.Side,
		PositionEffect: order.PositionEffect,
		Type:           order.Type,
		TimeInForce:    order.TimeInForce,
		Qty:            order.Qty,
		FilledQty:      order.FilledQty,
		AvgFillPrice:   order.AvgFillPrice,
		ReduceOnly:     order.ReduceOnly,
		MaxSlippageBps: order.MaxSlippageBps,
		Status:         order.Status,
		FrozenMargin:   order.FrozenMargin,
		CreatedAt:      order.CreatedAt,
		UpdatedAt:      order.UpdatedAt,
	}).Error
}

func (r *LiquidationRepository) CreateFill(ctx context.Context, fill liquidationdomain.FillRecord) error {
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

func (r *LiquidationRepository) UpsertPosition(ctx context.Context, position liquidationdomain.Position) error {
	model := PositionModel{
		PositionID:        position.PositionID,
		UserID:            position.UserID,
		SymbolID:          position.SymbolID,
		Side:              position.Side,
		Qty:               position.Qty,
		AvgEntryPrice:     position.AvgEntryPrice,
		MarkPrice:         position.MarkPrice,
		Notional:          position.Notional,
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
	return DB(ctx, r.db).Model(&PositionModel{}).
		Where("position_id = ?", position.PositionID).
		Updates(map[string]any{
			"qty":                model.Qty,
			"mark_price":         model.MarkPrice,
			"notional":           model.Notional,
			"initial_margin":     model.InitialMargin,
			"maintenance_margin": model.MaintenanceMargin,
			"realized_pnl":       model.RealizedPnL,
			"unrealized_pnl":     model.UnrealizedPnL,
			"funding_accrual":    model.FundingAccrual,
			"liquidation_price":  model.LiquidationPrice,
			"bankruptcy_price":   model.BankruptcyPrice,
			"status":             model.Status,
			"updated_at":         model.UpdatedAt,
		}).Error
}

type RiskOutboxPublisher struct{ outbox *OutboxRepository }

func NewRiskOutboxPublisher(db *gorm.DB) *RiskOutboxPublisher {
	return &RiskOutboxPublisher{outbox: NewOutboxRepository(db)}
}

func (p *RiskOutboxPublisher) Publish(ctx context.Context, event riskdomain.DomainEvent) error {
	return p.outbox.Create(ctx, OutboxMessage{
		EventID:       event.EventID,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		EventType:     event.EventType,
		Payload:       event.Payload,
		CreatedAt:     event.CreatedAt,
	})
}

type HedgeOutboxPublisher struct{ outbox *OutboxRepository }

func NewHedgeOutboxPublisher(db *gorm.DB) *HedgeOutboxPublisher {
	return &HedgeOutboxPublisher{outbox: NewOutboxRepository(db)}
}

func (p *HedgeOutboxPublisher) Publish(ctx context.Context, event hedgedomain.DomainEvent) error {
	return p.outbox.Create(ctx, OutboxMessage{
		EventID:       event.EventID,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		EventType:     event.EventType,
		Payload:       event.Payload,
		CreatedAt:     event.CreatedAt,
	})
}

type LiquidationOutboxPublisher struct{ outbox *OutboxRepository }

func NewLiquidationOutboxPublisher(db *gorm.DB) *LiquidationOutboxPublisher {
	return &LiquidationOutboxPublisher{outbox: NewOutboxRepository(db)}
}

func (p *LiquidationOutboxPublisher) Publish(ctx context.Context, event liquidationdomain.DomainEvent) error {
	return p.outbox.Create(ctx, OutboxMessage{
		EventID:       event.EventID,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		EventType:     event.EventType,
		Payload:       event.Payload,
		CreatedAt:     event.CreatedAt,
	})
}

func decimalAdd(a string, b string) string {
	return decimalx.MustFromString(a).Add(decimalx.MustFromString(b)).String()
}

func toRiskSnapshotDomain(model RiskSnapshotModel) riskdomain.Snapshot {
	return riskdomain.Snapshot{
		ID:                model.ID,
		UserID:            model.UserID,
		Equity:            model.Equity,
		AvailableBalance:  model.AvailableBalance,
		MaintenanceMargin: model.MaintenanceMargin,
		MarginRatio:       model.MarginRatio,
		RiskLevel:         model.RiskLevel,
		TriggeredBy:       model.TriggeredBy,
		CreatedAt:         model.CreatedAt,
	}
}

func toHedgeIntentDomain(model HedgeIntentModel, symbol string) hedgedomain.Intent {
	return hedgedomain.Intent{
		ID:                 model.HedgeIntentID,
		SymbolID:           model.SymbolID,
		Symbol:             symbol,
		Side:               model.Side,
		TargetQty:          model.TargetQty,
		CurrentNetExposure: model.CurrentNetExposure,
		Status:             model.Status,
		CreatedAt:          model.CreatedAt,
		UpdatedAt:          model.UpdatedAt,
	}
}

func toHedgeOrderDomain(model HedgeOrderModel) hedgedomain.Order {
	return hedgedomain.Order{
		ID:           model.HedgeOrderID,
		IntentID:     model.HedgeIntentID,
		Venue:        model.Venue,
		VenueOrderID: model.VenueOrderID,
		Symbol:       model.Symbol,
		Side:         model.Side,
		Qty:          model.Qty,
		Price:        model.Price,
		Status:       model.Status,
		ErrorCode:    model.ErrorCode,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}

func toHedgePositionDomain(model HedgePositionModel) hedgedomain.Position {
	return hedgedomain.Position{
		Symbol:        model.Symbol,
		Side:          model.Side,
		Qty:           model.Qty,
		AvgEntryPrice: model.AvgEntryPrice,
		RealizedPnL:   model.RealizedPnL,
		UnrealizedPnL: model.UnrealizedPnL,
		UpdatedAt:     model.UpdatedAt,
	}
}

func toLiquidationDomain(model LiquidationModel) liquidationdomain.Liquidation {
	preAccount, _ := unmarshalOptionalJSON[liquidationdomain.AccountSnapshot](model.PreAccountJSON)
	postAccount, _ := unmarshalOptionalJSON[liquidationdomain.AccountSnapshot](model.PostAccountJSON)
	prePositions, _ := unmarshalSliceJSON[liquidationdomain.PositionSnapshot](model.PrePositionsJSON)
	postPositions, _ := unmarshalSliceJSON[liquidationdomain.PositionSnapshot](model.PostPositionsJSON)
	releasedOrders, _ := unmarshalSliceJSON[liquidationdomain.ReleasedOrderSnapshot](model.ReleasedOrdersJSON)
	priceSnapshot, _ := unmarshalSliceJSON[liquidationdomain.PriceSnapshot](model.PriceSnapshotJSON)
	configSnapshot, _ := unmarshalOptionalJSON[liquidationdomain.ConfigSnapshot](model.ConfigSnapshotJSON)
	settlementSnapshot, _ := unmarshalOptionalJSON[liquidationdomain.SettlementSnapshot](model.SettlementJSON)
	return liquidationdomain.Liquidation{
		ID:                    model.LiquidationID,
		UserID:                model.UserID,
		SymbolID:              model.SymbolID,
		Mode:                  model.Mode,
		Status:                model.Status,
		TriggerRiskSnapshotID: model.TriggerRiskSnapshotID,
		PenaltyAmount:         model.PenaltyAmount,
		InsuranceFundUsed:     model.InsuranceFundUsed,
		BankruptAmount:        model.BankruptAmount,
		AbortReason:           model.AbortReason,
		PreAccountSnapshot:    preAccount,
		PostAccountSnapshot:   postAccount,
		PrePositionsSnapshot:  prePositions,
		PostPositionsSnapshot: postPositions,
		ReleasedOrders:        releasedOrders,
		PriceSnapshot:         priceSnapshot,
		ConfigSnapshot:        configSnapshot,
		SettlementSnapshot:    settlementSnapshot,
		CreatedAt:             model.CreatedAt,
		UpdatedAt:             model.UpdatedAt,
	}
}

func newLiquidationModel(liquidation liquidationdomain.Liquidation) (*LiquidationModel, error) {
	preAccountJSON, err := marshalOptionalJSON(liquidation.PreAccountSnapshot)
	if err != nil {
		return nil, err
	}
	postAccountJSON, err := marshalOptionalJSON(liquidation.PostAccountSnapshot)
	if err != nil {
		return nil, err
	}
	prePositionsJSON, err := marshalOptionalJSON(liquidation.PrePositionsSnapshot)
	if err != nil {
		return nil, err
	}
	postPositionsJSON, err := marshalOptionalJSON(liquidation.PostPositionsSnapshot)
	if err != nil {
		return nil, err
	}
	releasedOrdersJSON, err := marshalOptionalJSON(liquidation.ReleasedOrders)
	if err != nil {
		return nil, err
	}
	priceSnapshotJSON, err := marshalOptionalJSON(liquidation.PriceSnapshot)
	if err != nil {
		return nil, err
	}
	configSnapshotJSON, err := marshalOptionalJSON(liquidation.ConfigSnapshot)
	if err != nil {
		return nil, err
	}
	settlementJSON, err := marshalOptionalJSON(liquidation.SettlementSnapshot)
	if err != nil {
		return nil, err
	}
	return &LiquidationModel{
		LiquidationID:         liquidation.ID,
		UserID:                liquidation.UserID,
		SymbolID:              liquidation.SymbolID,
		Mode:                  liquidation.Mode,
		Status:                liquidation.Status,
		TriggerRiskSnapshotID: liquidation.TriggerRiskSnapshotID,
		PenaltyAmount:         liquidation.PenaltyAmount,
		InsuranceFundUsed:     liquidation.InsuranceFundUsed,
		BankruptAmount:        liquidation.BankruptAmount,
		AbortReason:           liquidation.AbortReason,
		PreAccountJSON:        preAccountJSON,
		PostAccountJSON:       postAccountJSON,
		PrePositionsJSON:      prePositionsJSON,
		PostPositionsJSON:     postPositionsJSON,
		ReleasedOrdersJSON:    releasedOrdersJSON,
		PriceSnapshotJSON:     priceSnapshotJSON,
		ConfigSnapshotJSON:    configSnapshotJSON,
		SettlementJSON:        settlementJSON,
		CreatedAt:             liquidation.CreatedAt,
		UpdatedAt:             liquidation.UpdatedAt,
	}, nil
}

func marshalOptionalJSON(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	return &text, nil
}

func unmarshalOptionalJSON[T any](raw *string) (*T, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" || strings.EqualFold(strings.TrimSpace(*raw), "null") {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal([]byte(*raw), &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func unmarshalSliceJSON[T any](raw *string) ([]T, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" || strings.EqualFold(strings.TrimSpace(*raw), "null") {
		return nil, nil
	}
	var value []T
	if err := json.Unmarshal([]byte(*raw), &value); err != nil {
		return nil, err
	}
	return value, nil
}
