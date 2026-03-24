package db

import (
	"context"
	"fmt"

	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func loadRiskAccountState(ctx context.Context, tx *gorm.DB, userID uint64, forUpdate bool) (riskdomain.AccountState, error) {
	accountQuery := tx.Table("accounts").
		Select("accounts.account_code, COALESCE(account_balance_snapshots.balance, '0') AS balance").
		Joins("LEFT JOIN account_balance_snapshots ON account_balance_snapshots.account_id = accounts.id AND account_balance_snapshots.asset = accounts.asset").
		Where("accounts.user_id = ?", userID).
		Order("accounts.id ASC")
	if forUpdate {
		accountQuery = accountQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var accountRows []struct {
		AccountCode string `gorm:"column:account_code"`
		Balance     string `gorm:"column:balance"`
	}
	if err := accountQuery.Scan(&accountRows).Error; err != nil {
		return riskdomain.AccountState{}, err
	}

	positionQuery := tx.Table("positions").
		Select("positions.*, symbols.symbol, symbols.contract_multiplier").
		Joins("JOIN symbols ON symbols.id = positions.symbol_id").
		Where("positions.user_id = ? AND positions.status = ?", userID, orderdomain.PositionStatusOpen).
		Order("positions.id ASC")
	if forUpdate {
		positionQuery = positionQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var positionRows []struct {
		PositionModel
		Symbol             string `gorm:"column:symbol"`
		ContractMultiplier string `gorm:"column:contract_multiplier"`
	}
	if err := positionQuery.Scan(&positionRows).Error; err != nil {
		return riskdomain.AccountState{}, err
	}

	pendingOrderQuery := tx.Table("orders").
		Select("orders.order_id, orders.symbol_id, symbols.symbol, orders.side, orders.type, orders.qty, orders.price, orders.trigger_price, orders.frozen_initial_margin, orders.frozen_fee, orders.frozen_margin, symbols.contract_multiplier").
		Joins("JOIN symbols ON symbols.id = orders.symbol_id").
		Where("orders.user_id = ? AND orders.position_effect = ? AND orders.status IN ?", userID, orderdomain.PositionEffectOpen, []string{orderdomain.OrderStatusResting, orderdomain.OrderStatusTriggerWait}).
		Order("orders.id ASC")
	if forUpdate {
		pendingOrderQuery = pendingOrderQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var pendingOrderRows []struct {
		OrderID             string  `gorm:"column:order_id"`
		SymbolID            uint64  `gorm:"column:symbol_id"`
		Symbol              string  `gorm:"column:symbol"`
		Side                string  `gorm:"column:side"`
		Type                string  `gorm:"column:type"`
		Qty                 string  `gorm:"column:qty"`
		Price               *string `gorm:"column:price"`
		TriggerPrice        *string `gorm:"column:trigger_price"`
		FrozenInitialMargin string  `gorm:"column:frozen_initial_margin"`
		FrozenFee           string  `gorm:"column:frozen_fee"`
		FrozenMargin        string  `gorm:"column:frozen_margin"`
		ContractMultiplier  string  `gorm:"column:contract_multiplier"`
	}
	if err := pendingOrderQuery.Scan(&pendingOrderRows).Error; err != nil {
		return riskdomain.AccountState{}, err
	}

	state := riskdomain.AccountState{UserID: userID}
	for _, row := range accountRows {
		switch row.AccountCode {
		case "USER_WALLET":
			state.WalletBalance = row.Balance
		case "USER_ORDER_MARGIN":
			state.OrderMarginBalance = row.Balance
		case "USER_POSITION_MARGIN":
			state.PositionMarginBalance = row.Balance
		case "USER_WITHDRAW_HOLD":
			state.WithdrawHoldBalance = row.Balance
		}
	}

	symbolIDs := make([]uint64, 0, len(positionRows)+len(pendingOrderRows))
	seenSymbolIDs := make(map[uint64]struct{}, len(positionRows)+len(pendingOrderRows))
	for _, row := range positionRows {
		if _, ok := seenSymbolIDs[row.SymbolID]; ok {
			continue
		}
		seenSymbolIDs[row.SymbolID] = struct{}{}
		symbolIDs = append(symbolIDs, row.SymbolID)
	}
	for _, row := range pendingOrderRows {
		if _, ok := seenSymbolIDs[row.SymbolID]; ok {
			continue
		}
		seenSymbolIDs[row.SymbolID] = struct{}{}
		symbolIDs = append(symbolIDs, row.SymbolID)
	}

	tiersBySymbol, err := loadRiskTiersBySymbol(ctx, tx, symbolIDs)
	if err != nil {
		return riskdomain.AccountState{}, err
	}
	marksBySymbol, err := loadLatestMarkPrices(ctx, tx, symbolIDs)
	if err != nil {
		return riskdomain.AccountState{}, err
	}

	state.Positions = make([]riskdomain.PositionExposure, 0, len(positionRows))
	for _, row := range positionRows {
		mark, ok := marksBySymbol[row.SymbolID]
		if !ok {
			return riskdomain.AccountState{}, fmt.Errorf("%w: latest mark price missing for symbol %s", errorsx.ErrNotFound, row.Symbol)
		}
		notionalDecimal := decimalx.MustFromString(row.Qty).Mul(decimalx.MustFromString(mark.MarkPrice)).Mul(decimalx.MustFromString(row.ContractMultiplier))
		tiers, ok := tiersBySymbol[row.SymbolID]
		if !ok || len(tiers) == 0 {
			return riskdomain.AccountState{}, fmt.Errorf("%w: risk tier missing for symbol %s", errorsx.ErrNotFound, row.Symbol)
		}
		tier, err := selectRiskTierByNotional(tiers, notionalDecimal)
		if err != nil {
			return riskdomain.AccountState{}, err
		}
		notional, maintenanceMargin, unrealizedPnL := livePositionMetrics(row.Side, row.Qty, row.AvgEntryPrice, mark.MarkPrice, row.ContractMultiplier, tier.MMR)
		state.Positions = append(state.Positions, riskdomain.PositionExposure{
			PositionID:         row.PositionID,
			SymbolID:           row.SymbolID,
			Symbol:             row.Symbol,
			Side:               row.Side,
			MarginMode:         marginModeOrDefault(row.MarginMode),
			Qty:                row.Qty,
			AvgEntryPrice:      row.AvgEntryPrice,
			MarkPrice:          mark.MarkPrice,
			Notional:           notional,
			InitialMargin:      row.InitialMargin,
			MaintenanceMargin:  maintenanceMargin,
			UnrealizedPnL:      unrealizedPnL,
			FundingAccrual:     row.FundingAccrual,
			LiquidationPrice:   row.LiquidationPrice,
			LiquidationFeeRate: tier.LiquidationFeeRate,
			ContractMultiplier: row.ContractMultiplier,
			MarkPriceUpdatedAt: mark.CreatedAt,
		})
	}

	state.PendingOrders = make([]riskdomain.PendingOrderExposure, 0, len(pendingOrderRows))
	for _, row := range pendingOrderRows {
		mark, ok := marksBySymbol[row.SymbolID]
		if !ok {
			mark = latestMarkPriceSnapshot{}
		}
		state.PendingOrders = append(state.PendingOrders, riskdomain.PendingOrderExposure{
			OrderID:             row.OrderID,
			SymbolID:            row.SymbolID,
			Symbol:              row.Symbol,
			Side:                row.Side,
			Type:                row.Type,
			Qty:                 row.Qty,
			Price:               row.Price,
			TriggerPrice:        row.TriggerPrice,
			FrozenInitialMargin: row.FrozenInitialMargin,
			FrozenFee:           row.FrozenFee,
			FrozenMargin:        row.FrozenMargin,
			MarkPrice:           mark.MarkPrice,
			ContractMultiplier:  row.ContractMultiplier,
			MarkPriceUpdatedAt:  mark.CreatedAt,
		})
	}

	return state, nil
}
