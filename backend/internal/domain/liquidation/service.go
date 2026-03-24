package liquidation

import (
	"context"
	"fmt"
	"slices"
	"strings"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type ServiceConfig struct {
	Asset            string
	PenaltyRate      string
	ExtraSlippageBps int
}

type Service struct {
	cfg      ServiceConfig
	clock    Clock
	idgen    IDGenerator
	txm      TxManager
	repo     Repository
	accounts AccountResolver
	ledger   LedgerPoster
	risk     RiskPreviewer
	outbox   OutboxPublisher
	runtime  RuntimeConfigProvider
}

const (
	abortReasonNoOpenPositions           = "no_open_positions"
	abortReasonAfterRelease              = "liquidation_aborted_after_release"
	liquidationPriceSourceMarkPrice      = "MARK_PRICE"
	liquidationPriceSourceExecutionPrice = "EXECUTION_PRICE"
)

type liquidationExecution struct {
	position       Position
	executionPrice decimalx.Decimal
	realizedPnL    decimalx.Decimal
	penalty        decimalx.Decimal
}

func NewService(cfg ServiceConfig, clock Clock, idgen IDGenerator, txm TxManager, repo Repository, accounts AccountResolver, ledger LedgerPoster, risk RiskPreviewer, outbox OutboxPublisher) (*Service, error) {
	if cfg.Asset == "" || cfg.PenaltyRate == "" || cfg.ExtraSlippageBps < 0 {
		return nil, fmt.Errorf("%w: invalid liquidation config", errorsx.ErrInvalidArgument)
	}
	if clock == nil || idgen == nil || txm == nil || repo == nil || accounts == nil || ledger == nil || risk == nil || outbox == nil {
		return nil, fmt.Errorf("%w: missing liquidation dependency", errorsx.ErrInvalidArgument)
	}
	return &Service{
		cfg:      cfg,
		clock:    clock,
		idgen:    idgen,
		txm:      txm,
		repo:     repo,
		accounts: accounts,
		ledger:   ledger,
		risk:     risk,
		outbox:   outbox,
	}, nil
}

func (s *Service) SetRuntimeConfigProvider(provider RuntimeConfigProvider) {
	s.runtime = provider
}

func (s *Service) Execute(ctx context.Context, input ExecuteInput) (Liquidation, error) {
	input.Mode = strings.ToUpper(strings.TrimSpace(input.Mode))
	if input.Mode == "" {
		input.Mode = ModeFull
	}
	if strings.TrimSpace(input.LiquidationID) == "" || input.UserID == 0 || input.TriggerRiskSnapshotID == 0 {
		return Liquidation{}, fmt.Errorf("%w: invalid liquidation input", errorsx.ErrInvalidArgument)
	}
	if input.Mode != ModeFull && input.Mode != ModeIsolated {
		return Liquidation{}, fmt.Errorf("%w: invalid liquidation mode", errorsx.ErrInvalidArgument)
	}
	if input.Mode == ModeIsolated && strings.TrimSpace(input.PositionID) == "" {
		return Liquidation{}, fmt.Errorf("%w: invalid liquidation input", errorsx.ErrInvalidArgument)
	}
	cfg := s.currentConfig("")

	var liquidation Liquidation
	now := s.clock.Now().UTC()
	err := s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, err := s.repo.GetLiquidationByIDForUpdate(txCtx, input.LiquidationID)
		if err == nil {
			liquidation = existing
			if existing.Status == StatusExecuted || existing.Status == StatusAborted {
				return nil
			}
		} else if err != errorsx.ErrNotFound {
			return err
		} else {
			liquidation = Liquidation{
				ID:                    input.LiquidationID,
				UserID:                input.UserID,
				Mode:                  input.Mode,
				Status:                StatusExecuting,
				TriggerRiskSnapshotID: input.TriggerRiskSnapshotID,
				PenaltyAmount:         "0",
				InsuranceFundUsed:     "0",
				BankruptAmount:        "0",
				ConfigSnapshot:        buildConfigSnapshot(cfg),
				CreatedAt:             now,
				UpdatedAt:             now,
			}
			if err := s.repo.CreateLiquidation(txCtx, liquidation); err != nil {
				return err
			}
		}

		positions, err := s.loadTargetPositions(txCtx, input)
		if err != nil {
			return err
		}
		if len(positions) == 0 {
			liquidation.Status = StatusAborted
			liquidation.AbortReason = stringPtr(abortReasonNoOpenPositions)
			liquidation.ConfigSnapshot = buildConfigSnapshot(cfg)
			liquidation.PrePositionsSnapshot = []PositionSnapshot{}
			liquidation.PostPositionsSnapshot = []PositionSnapshot{}
			liquidation.ReleasedOrders = []ReleasedOrderSnapshot{}
			liquidation.PriceSnapshot = []PriceSnapshot{}
			liquidation.UpdatedAt = now
			if err := s.repo.UpdateLiquidation(txCtx, liquidation); err != nil {
				return err
			}
			return s.outbox.Publish(txCtx, DomainEvent{
				EventID:       s.idgen.NewID("evt"),
				AggregateType: "liquidation",
				AggregateID:   liquidation.ID,
				EventType:     "risk.liquidation.aborted",
				Payload: map[string]any{
					"liquidation_id": liquidation.ID,
					"user_id":        liquidation.UserID,
					"status":         liquidation.Status,
					"abort_reason":   abortReasonNoOpenPositions,
				},
				CreatedAt: now,
			})
		}
		if err := s.lockSymbolsForUpdate(txCtx, positions); err != nil {
			return err
		}

		resolvedAccounts, err := s.accounts.ResolveLiquidationAccounts(txCtx, input.UserID, s.cfg.Asset)
		if err != nil {
			return err
		}
		preCoverageBalances, err := s.repo.GetCoverageBalancesForUpdate(txCtx, input.UserID, s.cfg.Asset)
		if err != nil {
			return err
		}
		prePreview, err := s.risk.PreviewAccountRisk(txCtx, input.UserID)
		if err != nil {
			return err
		}

		orders := make([]RiskIncreaseOrder, 0)
		releasedOrderMargin := decimalx.MustFromString("0")
		orderIDs := make([]string, 0)
		if input.Mode == ModeFull {
			orders, err = s.repo.ListRiskIncreaseOrdersForUpdate(txCtx, input.UserID)
			if err != nil {
				return err
			}
			orderIDs = make([]string, 0, len(orders))
			for _, order := range orders {
				orderIDs = append(orderIDs, order.OrderID)
				releasedOrderMargin = releasedOrderMargin.Add(decimalx.MustFromString(order.FrozenMargin))
			}
		}
		liquidation.ConfigSnapshot = buildConfigSnapshot(cfg)
		liquidation.PreAccountSnapshot = buildAccountSnapshot(prePreview, preCoverageBalances)
		liquidation.PrePositionsSnapshot = buildPositionSnapshots(positions, nil)
		liquidation.ReleasedOrders = buildReleasedOrderSnapshots(orders)
		liquidation.PriceSnapshot = buildPriceSnapshots(positions, nil, cfg.ExtraSlippageBps)
		if len(orderIDs) > 0 {
			if err := s.repo.CancelOrders(txCtx, orderIDs, now); err != nil {
				return err
			}
		}
		if releasedOrderMargin.GreaterThan(decimalx.MustFromString("0")) {
			if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
				LedgerTx: ledgerdomain.LedgerTx{
					ID:             s.idgen.NewID("ldg"),
					EventID:        s.idgen.NewID("evt"),
					BizType:        "liquidation.release",
					BizRefID:       liquidation.ID,
					Asset:          s.cfg.Asset,
					IdempotencyKey: liquidation.ID + ":release",
					OperatorType:   "SYSTEM",
					OperatorID:     "liquidator",
					TraceID:        input.TraceID,
					Status:         "COMMITTED",
					CreatedAt:      now,
				},
				Entries: []ledgerdomain.LedgerEntry{
					{AccountID: resolvedAccounts.UserOrderMarginAccountID, UserID: uint64Ptr(input.UserID), Asset: s.cfg.Asset, Amount: releasedOrderMargin.Neg().String(), EntryType: "LIQUIDATION_ORDER_RELEASE"},
					{AccountID: resolvedAccounts.UserWalletAccountID, UserID: uint64Ptr(input.UserID), Asset: s.cfg.Asset, Amount: releasedOrderMargin.String(), EntryType: "LIQUIDATION_ORDER_RELEASE"},
				},
			}); err != nil {
				return err
			}
		}

		coverageBalances, err := s.repo.GetCoverageBalancesForUpdate(txCtx, input.UserID, s.cfg.Asset)
		if err != nil {
			return err
		}
		preview, err := s.risk.PreviewAccountRisk(txCtx, input.UserID)
		if err != nil {
			return err
		}
		liquidation.PostAccountSnapshot = buildAccountSnapshot(preview, coverageBalances)
		if input.Mode == ModeFull && preview.RiskLevel != riskdomain.RiskLevelLiquidating {
			liquidation.Status = StatusAborted
			liquidation.AbortReason = stringPtr(abortReasonAfterRelease)
			liquidation.PostPositionsSnapshot = buildPositionSnapshots(positions, nil)
			liquidation.UpdatedAt = now
			if err := s.repo.UpdateLiquidation(txCtx, liquidation); err != nil {
				return err
			}
			return s.outbox.Publish(txCtx, DomainEvent{
				EventID:       s.idgen.NewID("evt"),
				AggregateType: "liquidation",
				AggregateID:   liquidation.ID,
				EventType:     "risk.liquidation.aborted",
				Payload: map[string]any{
					"liquidation_id":       liquidation.ID,
					"user_id":              liquidation.UserID,
					"status":               liquidation.Status,
					"abort_reason":         abortReasonAfterRelease,
					"released_order_count": len(liquidation.ReleasedOrders),
					"released_margin":      releasedOrderMargin.String(),
				},
				CreatedAt: now,
			})
		}

		executions := make([]liquidationExecution, 0, len(positions))
		totalInitial := decimalx.MustFromString("0")
		totalPenalty := decimalx.MustFromString("0")
		totalRealizedPnL := decimalx.MustFromString("0")
		for _, position := range positions {
			positionCfg := s.currentConfig(position.Symbol)
			executionPrice := s.worsenedExecutionPrice(position.Side, position.MarkPrice)
			qty := decimalx.MustFromString(position.Qty)
			entryPrice := decimalx.MustFromString(position.AvgEntryPrice)
			initialMargin := decimalx.MustFromString(position.InitialMargin)
			fundingAccrual := decimalx.MustFromString(defaultDecimalString(position.FundingAccrual))
			multiplier := decimalx.MustFromString(position.ContractMultiplier)
			realizedPnL := realizedPnL(position.Side, qty, executionPrice, entryPrice, multiplier)
			liquidatedNotional := qty.Mul(executionPrice).Mul(multiplier)
			penalty := liquidatedNotional.Mul(decimalx.MustFromString(positionCfg.PenaltyRate))

			executions = append(executions, liquidationExecution{
				position:       position,
				executionPrice: executionPrice,
				realizedPnL:    realizedPnL,
				penalty:        penalty,
			})
			totalInitial = totalInitial.Add(initialMargin.Add(fundingAccrual))
			totalPenalty = totalPenalty.Add(penalty)
			totalRealizedPnL = totalRealizedPnL.Add(realizedPnL)
		}

		settlementNet := totalInitial.Add(totalRealizedPnL).Sub(totalPenalty)
		walletBalance := decimalx.MustFromString(coverageBalances.UserWalletBalance)
		insuranceBalance := decimalx.MustFromString(coverageBalances.InsuranceFundBalance)
		walletDebit := decimalx.MustFromString("0")
		walletCredit := decimalx.MustFromString("0")
		insuranceDebit := decimalx.MustFromString("0")
		bankruptAmount := decimalx.MustFromString("0")
		if settlementNet.GreaterThan(decimalx.MustFromString("0")) {
			walletCredit = settlementNet
		} else if settlementNet.LessThan(decimalx.MustFromString("0")) {
			deficit := settlementNet.Abs()
			remainingAfterWallet := deficit
			if input.Mode == ModeFull {
				walletDebit = minDecimal(walletBalance, deficit)
				remainingAfterWallet = deficit.Sub(walletDebit)
			}
			insuranceDebit = minDecimal(insuranceBalance, remainingAfterWallet)
			bankruptAmount = remainingAfterWallet.Sub(insuranceDebit)
		}
		liquidation.PriceSnapshot = buildPriceSnapshots(positions, executions, cfg.ExtraSlippageBps)
		liquidation.SettlementSnapshot = buildSettlementSnapshot(
			releasedOrderMargin.String(),
			totalInitial.String(),
			totalRealizedPnL.String(),
			totalPenalty.String(),
			walletDebit.String(),
			walletCredit.String(),
			insuranceDebit.String(),
			bankruptAmount.String(),
		)

		if bankruptAmount.GreaterThan(decimalx.MustFromString("0")) {
			liquidation.Status = StatusPendingManual
			liquidation.InsuranceFundUsed = insuranceDebit.String()
			liquidation.BankruptAmount = bankruptAmount.String()
			liquidation.PostPositionsSnapshot = buildPositionSnapshots(positions, nil)
			liquidation.UpdatedAt = now
			if err := s.repo.UpdateLiquidation(txCtx, liquidation); err != nil {
				return err
			}
			return s.outbox.Publish(txCtx, DomainEvent{
				EventID:       s.idgen.NewID("evt"),
				AggregateType: "liquidation",
				AggregateID:   liquidation.ID,
				EventType:     "risk.liquidation.manual_required",
				Payload: map[string]any{
					"liquidation_id":          liquidation.ID,
					"user_id":                 liquidation.UserID,
					"required_wallet_debit":   walletDebit.String(),
					"required_insurance_fund": insuranceDebit.String(),
					"bankrupt_amount":         bankruptAmount.String(),
					"released_margin":         releasedOrderMargin.String(),
					"settlement_snapshot":     liquidation.SettlementSnapshot,
					"status":                  liquidation.Status,
				},
				CreatedAt: now,
			})
		}

		settlementLedgerTxID := s.idgen.NewID("ldg")
		settlementEntries := make([]ledgerdomain.LedgerEntry, 0, 5)
		settlementEntries = appendNonZeroEntry(settlementEntries, ledgerdomain.LedgerEntry{
			AccountID: resolvedAccounts.UserPositionMarginAccountID,
			UserID:    uint64Ptr(input.UserID),
			Asset:     s.cfg.Asset,
			Amount:    totalInitial.Neg().String(),
			EntryType: "LIQUIDATION_MARGIN_RELEASE",
		})
		settlementEntries = appendNonZeroEntry(settlementEntries, ledgerdomain.LedgerEntry{
			AccountID: resolvedAccounts.PenaltyAccountID,
			Asset:     s.cfg.Asset,
			Amount:    totalPenalty.String(),
			EntryType: "LIQUIDATION_PENALTY",
		})
		settlementEntries = appendNonZeroEntry(settlementEntries, ledgerdomain.LedgerEntry{
			AccountID: resolvedAccounts.SystemPoolAccountID,
			Asset:     s.cfg.Asset,
			Amount:    totalRealizedPnL.Neg().String(),
			EntryType: "LIQUIDATION_SETTLEMENT",
		})
		if walletCredit.GreaterThan(decimalx.MustFromString("0")) {
			settlementEntries = append(settlementEntries, ledgerdomain.LedgerEntry{
				AccountID: resolvedAccounts.UserWalletAccountID,
				UserID:    uint64Ptr(input.UserID),
				Asset:     s.cfg.Asset,
				Amount:    walletCredit.String(),
				EntryType: "LIQUIDATION_REFUND",
			})
		}
		if walletDebit.GreaterThan(decimalx.MustFromString("0")) {
			settlementEntries = append(settlementEntries, ledgerdomain.LedgerEntry{
				AccountID: resolvedAccounts.UserWalletAccountID,
				UserID:    uint64Ptr(input.UserID),
				Asset:     s.cfg.Asset,
				Amount:    walletDebit.Neg().String(),
				EntryType: "LIQUIDATION_WALLET_COVER",
			})
		}
		if insuranceDebit.GreaterThan(decimalx.MustFromString("0")) {
			settlementEntries = append(settlementEntries, ledgerdomain.LedgerEntry{
				AccountID: resolvedAccounts.InsuranceFundAccountID,
				Asset:     s.cfg.Asset,
				Amount:    insuranceDebit.Neg().String(),
				EntryType: "LIQUIDATION_INSURANCE",
			})
		}
		if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             settlementLedgerTxID,
				EventID:        s.idgen.NewID("evt"),
				BizType:        "liquidation.execute",
				BizRefID:       liquidation.ID,
				Asset:          s.cfg.Asset,
				IdempotencyKey: fmt.Sprintf("liquidation:%s:settlement", liquidation.ID),
				OperatorType:   "SYSTEM",
				OperatorID:     "liquidator",
				TraceID:        input.TraceID,
				Status:         "COMMITTED",
				CreatedAt:      now,
			},
			Entries: settlementEntries,
		}); err != nil {
			return err
		}

		for _, execution := range executions {
			position := execution.position
			qty := decimalx.MustFromString(position.Qty)
			orderID := s.idgen.NewID("ord")
			order := OrderRecord{
				OrderID:        orderID,
				ClientOrderID:  "liq:" + liquidation.ID + ":" + position.PositionID,
				UserID:         input.UserID,
				SymbolID:       position.SymbolID,
				Side:           oppositeSide(position.Side),
				MarginMode:     position.MarginMode,
				PositionEffect: "CLOSE",
				Type:           "MARKET",
				TimeInForce:    "IOC",
				Qty:            position.Qty,
				FilledQty:      position.Qty,
				AvgFillPrice:   execution.executionPrice.String(),
				ReduceOnly:     true,
				Status:         "FILLED",
				FrozenMargin:   "0",
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := s.repo.CreateOrder(txCtx, order); err != nil {
				return err
			}
			if err := s.repo.CreateFill(txCtx, FillRecord{
				FillID:     s.idgen.NewID("fill"),
				OrderID:    orderID,
				UserID:     input.UserID,
				SymbolID:   position.SymbolID,
				Side:       order.Side,
				Qty:        position.Qty,
				Price:      execution.executionPrice.String(),
				FeeAmount:  execution.penalty.String(),
				LedgerTxID: settlementLedgerTxID,
				CreatedAt:  now,
			}); err != nil {
				return err
			}

			position.Qty = "0"
			position.MarkPrice = execution.executionPrice.String()
			position.Notional = "0"
			position.InitialMargin = "0"
			position.MaintenanceMargin = "0"
			position.RealizedPnL = decimalx.MustFromString(position.RealizedPnL).Add(execution.realizedPnL).String()
			position.UnrealizedPnL = "0"
			position.FundingAccrual = "0"
			position.LiquidationPrice = "0"
			position.BankruptcyPrice = "0"
			position.Status = "CLOSED"
			position.UpdatedAt = now
			if err := s.repo.UpsertPosition(txCtx, position); err != nil {
				return err
			}

			if err := s.repo.CreateItem(txCtx, Item{
				LiquidationID:  liquidation.ID,
				PositionID:     position.PositionID,
				LiquidatedQty:  qty.String(),
				ExecutionPrice: execution.executionPrice.String(),
				LedgerTxID:     settlementLedgerTxID,
				CreatedAt:      now,
			}); err != nil {
				return err
			}
		}

		finalCoverageBalances, err := s.repo.GetCoverageBalancesForUpdate(txCtx, input.UserID, s.cfg.Asset)
		if err != nil {
			return err
		}
		finalPreview, err := s.risk.PreviewAccountRisk(txCtx, input.UserID)
		if err != nil {
			return err
		}

		liquidation.Status = StatusExecuted
		liquidation.PenaltyAmount = totalPenalty.String()
		liquidation.InsuranceFundUsed = insuranceDebit.String()
		liquidation.BankruptAmount = bankruptAmount.String()
		liquidation.PostAccountSnapshot = buildAccountSnapshot(finalPreview, finalCoverageBalances)
		liquidation.PostPositionsSnapshot = buildPositionSnapshots(positions, executions)
		liquidation.UpdatedAt = now
		if err := s.repo.UpdateLiquidation(txCtx, liquidation); err != nil {
			return err
		}

		payload := map[string]any{
			"liquidation_id":      liquidation.ID,
			"user_id":             liquidation.UserID,
			"liquidated_qty":      aggregateQty(positions),
			"penalty_amount":      liquidation.PenaltyAmount,
			"insurance_fund_used": liquidation.InsuranceFundUsed,
			"released_margin":     releasedOrderMargin.String(),
			"settlement_snapshot": liquidation.SettlementSnapshot,
			"status":              liquidation.Status,
		}
		if len(positions) == 1 {
			payload["symbol"] = positions[0].Symbol
			payload["execution_price"] = s.worsenedExecutionPrice(positions[0].Side, positions[0].MarkPrice).String()
		}
		return s.outbox.Publish(txCtx, DomainEvent{
			EventID:       s.idgen.NewID("evt"),
			AggregateType: "liquidation",
			AggregateID:   liquidation.ID,
			EventType:     "risk.liquidation.executed",
			Payload:       payload,
			CreatedAt:     now,
		})
	})
	if err != nil {
		return Liquidation{}, err
	}
	return liquidation, nil
}

func buildAccountSnapshot(snapshot riskdomain.Snapshot, coverage CoverageBalances) *AccountSnapshot {
	return &AccountSnapshot{
		Equity:            snapshot.Equity,
		AvailableBalance:  snapshot.AvailableBalance,
		MaintenanceMargin: snapshot.MaintenanceMargin,
		MarginRatio:       snapshot.MarginRatio,
		RiskLevel:         snapshot.RiskLevel,
		WalletBalance:     coverage.UserWalletBalance,
		InsuranceBalance:  coverage.InsuranceFundBalance,
	}
}

func buildPositionSnapshots(positions []Position, executions []liquidationExecution) []PositionSnapshot {
	executionByPosition := make(map[string]liquidationExecution, len(executions))
	for _, execution := range executions {
		executionByPosition[execution.position.PositionID] = execution
	}
	snapshots := make([]PositionSnapshot, 0, len(positions))
	for _, position := range positions {
		snapshot := PositionSnapshot{
			PositionID:        position.PositionID,
			SymbolID:          position.SymbolID,
			Symbol:            position.Symbol,
			Side:              position.Side,
			MarginMode:        position.MarginMode,
			Qty:               position.Qty,
			AvgEntryPrice:     position.AvgEntryPrice,
			MarkPrice:         position.MarkPrice,
			Notional:          position.Notional,
			InitialMargin:     position.InitialMargin,
			MaintenanceMargin: position.MaintenanceMargin,
			UnrealizedPnL:     position.UnrealizedPnL,
			FundingAccrual:    position.FundingAccrual,
			LiquidationPrice:  position.LiquidationPrice,
			BankruptcyPrice:   position.BankruptcyPrice,
			Status:            position.Status,
		}
		if execution, ok := executionByPosition[position.PositionID]; ok {
			snapshot.ExecutionPrice = execution.executionPrice.String()
			snapshot.RealizedPnL = execution.realizedPnL.String()
			snapshot.Penalty = execution.penalty.String()
			snapshot.Qty = "0"
			snapshot.Notional = "0"
			snapshot.InitialMargin = "0"
			snapshot.MaintenanceMargin = "0"
			snapshot.UnrealizedPnL = "0"
			snapshot.LiquidationPrice = "0"
			snapshot.BankruptcyPrice = "0"
			snapshot.Status = "CLOSED"
			snapshot.MarkPrice = execution.executionPrice.String()
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (s *Service) loadTargetPositions(ctx context.Context, input ExecuteInput) ([]Position, error) {
	if input.Mode == ModeIsolated {
		position, err := s.repo.GetPositionForLiquidationByID(ctx, input.UserID, input.PositionID)
		if err != nil {
			if err == errorsx.ErrNotFound {
				return []Position{}, nil
			}
			return nil, err
		}
		return []Position{position}, nil
	}
	return s.repo.ListOpenPositionsForUpdate(ctx, input.UserID)
}

func (s *Service) lockSymbolsForUpdate(ctx context.Context, positions []Position) error {
	if len(positions) == 0 {
		return nil
	}
	unique := make(map[uint64]struct{}, len(positions))
	symbolIDs := make([]uint64, 0, len(positions))
	for _, position := range positions {
		if position.SymbolID == 0 {
			continue
		}
		if _, ok := unique[position.SymbolID]; ok {
			continue
		}
		unique[position.SymbolID] = struct{}{}
		symbolIDs = append(symbolIDs, position.SymbolID)
	}
	slices.Sort(symbolIDs)
	for _, symbolID := range symbolIDs {
		if err := s.repo.LockSymbolForUpdate(ctx, symbolID); err != nil {
			return err
		}
	}
	return nil
}

func buildReleasedOrderSnapshots(orders []RiskIncreaseOrder) []ReleasedOrderSnapshot {
	snapshots := make([]ReleasedOrderSnapshot, 0, len(orders))
	for _, order := range orders {
		snapshots = append(snapshots, ReleasedOrderSnapshot{
			OrderID:       order.OrderID,
			ClientOrderID: order.ClientOrderID,
			SymbolID:      order.SymbolID,
			Symbol:        order.Symbol,
			Side:          order.Side,
			Qty:           order.Qty,
			FrozenMargin:  order.FrozenMargin,
			Status:        order.Status,
		})
	}
	return snapshots
}

func buildPriceSnapshots(positions []Position, executions []liquidationExecution, extraSlippageBps int) []PriceSnapshot {
	executionByPosition := make(map[string]liquidationExecution, len(executions))
	for _, execution := range executions {
		executionByPosition[execution.position.PositionID] = execution
	}
	snapshots := make([]PriceSnapshot, 0, len(positions))
	for _, position := range positions {
		snapshot := PriceSnapshot{
			PositionID:      position.PositionID,
			SymbolID:        position.SymbolID,
			Symbol:          position.Symbol,
			MarkPrice:       position.MarkPrice,
			PriceSource:     liquidationPriceSourceMarkPrice,
			SlippageApplied: fmt.Sprintf("%d", extraSlippageBps),
		}
		if execution, ok := executionByPosition[position.PositionID]; ok {
			snapshot.ExecutionPrice = execution.executionPrice.String()
			snapshot.PriceSource = liquidationPriceSourceExecutionPrice
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func buildConfigSnapshot(cfg ServiceConfig) *ConfigSnapshot {
	return &ConfigSnapshot{
		Asset:            cfg.Asset,
		PenaltyRate:      cfg.PenaltyRate,
		ExtraSlippageBps: cfg.ExtraSlippageBps,
	}
}

func buildSettlementSnapshot(
	releasedOrderMargin string,
	totalInitial string,
	totalRealizedPnL string,
	totalPenalty string,
	walletDebit string,
	walletCredit string,
	insuranceDebit string,
	bankruptAmount string,
) *SettlementSnapshot {
	return &SettlementSnapshot{
		ReleasedOrderMargin: releasedOrderMargin,
		TotalInitialMargin:  totalInitial,
		TotalRealizedPnL:    totalRealizedPnL,
		TotalPenalty:        totalPenalty,
		WalletDebit:         walletDebit,
		WalletCredit:        walletCredit,
		InsuranceDebit:      insuranceDebit,
		BankruptAmount:      bankruptAmount,
	}
}

func (s *Service) currentConfig(symbol string) ServiceConfig {
	if s.runtime == nil {
		return s.cfg
	}
	cfg := s.runtime.CurrentLiquidationRuntimeConfig(symbol)
	if cfg.Asset == "" || cfg.PenaltyRate == "" {
		return s.cfg
	}
	return cfg
}

func appendNonZeroEntry(entries []ledgerdomain.LedgerEntry, entry ledgerdomain.LedgerEntry) []ledgerdomain.LedgerEntry {
	if decimalx.MustFromString(entry.Amount).IsZero() {
		return entries
	}
	return append(entries, entry)
}

func minDecimal(left decimalx.Decimal, right decimalx.Decimal) decimalx.Decimal {
	if left.LessThan(right) {
		return left
	}
	return right
}

func (s *Service) worsenedExecutionPrice(positionSide string, markPrice string) decimalx.Decimal {
	cfg := s.currentConfig("")
	mark := decimalx.MustFromString(markPrice)
	factor := decimalx.MustFromString(fmt.Sprintf("%d", cfg.ExtraSlippageBps)).Div(decimalx.MustFromString("10000"))
	if strings.ToUpper(positionSide) == "LONG" {
		return mark.Mul(decimalx.MustFromString("1").Sub(factor))
	}
	return mark.Mul(decimalx.MustFromString("1").Add(factor))
}

func realizedPnL(side string, qty decimalx.Decimal, executionPrice decimalx.Decimal, entryPrice decimalx.Decimal, multiplier decimalx.Decimal) decimalx.Decimal {
	sign := decimalx.MustFromString("1")
	if strings.ToUpper(side) == "SHORT" {
		sign = decimalx.MustFromString("-1")
	}
	return sign.Mul(qty).Mul(executionPrice.Sub(entryPrice)).Mul(multiplier)
}

func aggregateQty(items []Position) string {
	total := decimalx.MustFromString("0")
	for _, item := range items {
		total = total.Add(decimalx.MustFromString(item.Qty))
	}
	return total.String()
}

func oppositeSide(positionSide string) string {
	if strings.ToUpper(positionSide) == "LONG" {
		return "SELL"
	}
	return "BUY"
}

func defaultDecimalString(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return value
}

func uint64Ptr(v uint64) *uint64 { return &v }

func stringPtr(v string) *string { return &v }
