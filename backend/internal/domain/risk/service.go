package risk

import (
	"context"
	"fmt"
	"strings"
	"time"

	hedgedomain "github.com/xiaobao/rgperp/backend/internal/domain/hedge"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

const (
	liquidationModeFull     = "FULL"
	liquidationModeIsolated = "ISOLATED"
)

type ServiceConfig struct {
	RiskBufferRatio             string
	HedgeEnabled                bool
	SoftThresholdRatio          string
	HardThresholdRatio          string
	MarkPriceStaleSec           int
	ForceReduceOnlyOnStalePrice bool
	TakerFeeRate                string
}

type Service struct {
	cfg     ServiceConfig
	clock   Clock
	idgen   IDGenerator
	txm     TxManager
	repo    Repository
	outbox  OutboxPublisher
	runtime RuntimeConfigProvider
}

func NewService(cfg ServiceConfig, clock Clock, idgen IDGenerator, txm TxManager, repo Repository, outbox OutboxPublisher) (*Service, error) {
	if cfg.RiskBufferRatio == "" || cfg.SoftThresholdRatio == "" || cfg.HardThresholdRatio == "" || cfg.TakerFeeRate == "" {
		return nil, fmt.Errorf("%w: risk config is incomplete", errorsx.ErrInvalidArgument)
	}
	if clock == nil || idgen == nil || txm == nil || repo == nil || outbox == nil {
		return nil, fmt.Errorf("%w: missing risk dependency", errorsx.ErrInvalidArgument)
	}
	return &Service{
		cfg:    cfg,
		clock:  clock,
		idgen:  idgen,
		txm:    txm,
		repo:   repo,
		outbox: outbox,
	}, nil
}

func (s *Service) SetRuntimeConfigProvider(provider RuntimeConfigProvider) {
	s.runtime = provider
}

func (s *Service) PreviewAccountRisk(ctx context.Context, userID uint64) (Snapshot, error) {
	if userID == 0 {
		return Snapshot{}, fmt.Errorf("%w: user id is required", errorsx.ErrInvalidArgument)
	}
	state, err := s.repo.GetAccountStateForUpdate(ctx, userID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := s.validateMarkPrices(state, s.currentConfig()); err != nil {
		return Snapshot{}, err
	}
	return s.buildSnapshot(state, "preview"), nil
}

func (s *Service) RecalculateAccountRisk(ctx context.Context, userID uint64, triggeredBy string) (Snapshot, *LiquidationTrigger, error) {
	return s.recalculateAccountRisk(ctx, userID, triggeredBy, true)
}

func (s *Service) RefreshAccountRisk(ctx context.Context, userID uint64, triggeredBy string) (Snapshot, error) {
	snapshot, _, err := s.recalculateAccountRisk(ctx, userID, triggeredBy, false)
	return snapshot, err
}

func (s *Service) recalculateAccountRisk(ctx context.Context, userID uint64, triggeredBy string, allowLiquidationTrigger bool) (Snapshot, *LiquidationTrigger, error) {
	if userID == 0 {
		return Snapshot{}, nil, fmt.Errorf("%w: user id is required", errorsx.ErrInvalidArgument)
	}
	triggeredBy = strings.TrimSpace(triggeredBy)
	if triggeredBy == "" {
		triggeredBy = "manual"
	}

	var snapshot Snapshot
	var trigger *LiquidationTrigger
	now := s.clock.Now().UTC()
	err := s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		previous, prevErr := s.repo.GetLatestRiskSnapshot(txCtx, userID)
		if prevErr != nil && prevErr != errorsx.ErrNotFound {
			return prevErr
		}

		state, err := s.repo.GetAccountStateForUpdate(txCtx, userID)
		if err != nil {
			return err
		}
		if err := s.validateMarkPrices(state, s.currentConfig()); err != nil {
			return err
		}

		built := s.buildSnapshot(state, triggeredBy)
		built.CreatedAt = now
		saved, err := s.repo.CreateRiskSnapshot(txCtx, built)
		if err != nil {
			return err
		}
		snapshot = saved

		riskLevelChanged := prevErr == nil && previous.RiskLevel != snapshot.RiskLevel
		if riskLevelChanged {
			if err := s.outbox.Publish(txCtx, DomainEvent{
				EventID:       s.idgen.NewID("evt"),
				AggregateType: "risk_snapshot",
				AggregateID:   fmt.Sprintf("%d", snapshot.ID),
				EventType:     "risk.snapshot.updated",
				Payload: map[string]any{
					"risk_snapshot_id":    snapshot.ID,
					"user_id":             snapshot.UserID,
					"equity":              snapshot.Equity,
					"available_balance":   snapshot.AvailableBalance,
					"maintenance_margin":  snapshot.MaintenanceMargin,
					"margin_ratio":        snapshot.MarginRatio,
					"risk_level":          snapshot.RiskLevel,
					"previous_risk_level": previous.RiskLevel,
					"triggered_by":        snapshot.TriggeredBy,
				},
				CreatedAt: now,
			}); err != nil {
				return err
			}
		}

		if !allowLiquidationTrigger {
			return nil
		}

		shouldTriggerFull := snapshot.RiskLevel == RiskLevelLiquidating && (prevErr == errorsx.ErrNotFound || previous.RiskLevel != RiskLevelLiquidating)
		if shouldTriggerFull {
			trigger = &LiquidationTrigger{
				LiquidationID:     s.idgen.NewID("liq"),
				UserID:            userID,
				Mode:              liquidationModeFull,
				MarginRatio:       snapshot.MarginRatio,
				Equity:            snapshot.Equity,
				MaintenanceMargin: snapshot.MaintenanceMargin,
				TriggeredAt:       now,
				Status:            "TRIGGERED",
				SnapshotID:        snapshot.ID,
			}
			return s.publishLiquidationTriggered(txCtx, trigger)
		}

		position := findTriggeredIsolatedPosition(state)
		if position == nil {
			return nil
		}
		claimed, err := s.repo.MarkPositionLiquidating(txCtx, position.PositionID, now)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		trigger = &LiquidationTrigger{
			LiquidationID:     s.idgen.NewID("liq"),
			UserID:            userID,
			Mode:              liquidationModeIsolated,
			PositionID:        position.PositionID,
			Symbol:            position.Symbol,
			MarginRatio:       snapshot.MarginRatio,
			Equity:            snapshot.Equity,
			MaintenanceMargin: snapshot.MaintenanceMargin,
			TriggeredAt:       now,
			Status:            "TRIGGERED",
			SnapshotID:        snapshot.ID,
		}
		return s.publishLiquidationTriggered(txCtx, trigger)
	})
	if err != nil {
		return Snapshot{}, nil, err
	}
	return snapshot, trigger, nil
}

func (s *Service) EvaluateAllHedges(ctx context.Context) ([]HedgeDecision, error) {
	symbols, err := s.repo.ListActiveSymbols(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]HedgeDecision, 0, len(symbols))
	for _, symbol := range symbols {
		decision, err := s.EvaluateHedgeIntent(ctx, symbol.SymbolID)
		if err != nil {
			return nil, err
		}
		if decision != nil {
			out = append(out, *decision)
		}
	}
	return out, nil
}

func (s *Service) publishLiquidationTriggered(ctx context.Context, trigger *LiquidationTrigger) error {
	payload := map[string]any{
		"liquidation_id":     trigger.LiquidationID,
		"user_id":            trigger.UserID,
		"mode":               trigger.Mode,
		"margin_ratio":       trigger.MarginRatio,
		"equity":             trigger.Equity,
		"maintenance_margin": trigger.MaintenanceMargin,
		"trigger_price_ts":   trigger.TriggeredAt,
		"status":             trigger.Status,
		"risk_snapshot_id":   trigger.SnapshotID,
	}
	if strings.TrimSpace(trigger.PositionID) != "" {
		payload["position_id"] = trigger.PositionID
	}
	if strings.TrimSpace(trigger.Symbol) != "" {
		payload["symbol"] = trigger.Symbol
	}
	return s.outbox.Publish(ctx, DomainEvent{
		EventID:       s.idgen.NewID("evt"),
		AggregateType: "risk_liquidation",
		AggregateID:   trigger.LiquidationID,
		EventType:     "risk.liquidation.triggered",
		Payload:       payload,
		CreatedAt:     trigger.TriggeredAt,
	})
}

func findTriggeredIsolatedPosition(state AccountState) *PositionExposure {
	for idx := range state.Positions {
		position := &state.Positions[idx]
		if position.MarginMode != orderdomain.MarginModeIsolated {
			continue
		}
		if !isolatedPositionShouldLiquidate(*position) {
			continue
		}
		return position
	}
	return nil
}

func isolatedPositionShouldLiquidate(position PositionExposure) bool {
	liqPrice := strings.TrimSpace(position.LiquidationPrice)
	if liqPrice == "" || liqPrice == "0" {
		return false
	}
	mark := decimalx.MustFromString(position.MarkPrice)
	threshold := decimalx.MustFromString(liqPrice)
	if position.Side == orderdomain.PositionSideShort {
		return mark.GreaterThanOrEqual(threshold)
	}
	return mark.LessThanOrEqual(threshold)
}

func (s *Service) EvaluateHedgeIntent(ctx context.Context, symbolID uint64) (*HedgeDecision, error) {
	cfg := s.currentConfig()
	if !cfg.HedgeEnabled {
		return nil, nil
	}
	if symbolID == 0 {
		return nil, fmt.Errorf("%w: symbol id is required", errorsx.ErrInvalidArgument)
	}

	var decision *HedgeDecision
	now := s.clock.Now().UTC()
	err := s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		state, err := s.repo.GetHedgeStateForUpdate(txCtx, symbolID)
		if err != nil {
			return err
		}

		internalNet := decimalx.MustFromString(state.InternalLongQty).Sub(decimalx.MustFromString(state.InternalShortQty))
		targetSigned := internalNet.Neg()
		managedSigned := decimalx.MustFromString(state.ManagedLongQty).Sub(decimalx.MustFromString(state.ManagedShortQty))
		drift := targetSigned.Sub(managedSigned)
		if drift.IsZero() {
			return nil
		}

		basis := targetSigned.Abs()
		if basis.IsZero() {
			basis = drift.Abs()
		}
		if basis.IsZero() {
			return nil
		}
		deltaRatio := drift.Abs().Div(basis)
		if deltaRatio.LessThan(decimalx.MustFromString(cfg.HardThresholdRatio)) {
			return nil
		}

		existing, err := s.repo.GetLatestOpenHedgeIntentForUpdate(txCtx, symbolID)
		if err != nil && err != errorsx.ErrNotFound {
			return err
		}
		if err == nil && existing.Side == hedgeSideFromSigned(drift) && existing.TargetQty == drift.Abs().String() {
			decision = &HedgeDecision{Intent: existing, Drift: drift.String()}
			return nil
		}

		intent := hedgedomain.Intent{
			ID:                 s.idgen.NewID("hint"),
			SymbolID:           state.SymbolID,
			Symbol:             state.Symbol,
			Side:               hedgeSideFromSigned(drift),
			TargetQty:          drift.Abs().String(),
			CurrentNetExposure: internalNet.String(),
			Status:             hedgedomain.IntentStatusPending,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := s.repo.CreateHedgeIntent(txCtx, intent); err != nil {
			return err
		}

		decision = &HedgeDecision{
			Intent: intent,
			Drift:  drift.String(),
		}
		return s.outbox.Publish(txCtx, DomainEvent{
			EventID:       s.idgen.NewID("evt"),
			AggregateType: "hedge_intent",
			AggregateID:   intent.ID,
			EventType:     "hedge.requested",
			Payload: map[string]any{
				"hedge_intent_id":      intent.ID,
				"symbol":               intent.Symbol,
				"side":                 intent.Side,
				"target_qty":           intent.TargetQty,
				"current_net_exposure": intent.CurrentNetExposure,
				"soft_threshold_ratio": cfg.SoftThresholdRatio,
				"hard_threshold_ratio": cfg.HardThresholdRatio,
				"status":               intent.Status,
			},
			CreatedAt: now,
		})
	})
	if err != nil {
		return nil, err
	}
	return decision, nil
}

func (s *Service) buildSnapshot(state AccountState, triggeredBy string) Snapshot {
	metrics := ComputeAccountMetrics(state, s.currentConfig())
	return Snapshot{
		UserID:            state.UserID,
		Equity:            metrics.Equity,
		AvailableBalance:  metrics.AvailableBalance,
		MaintenanceMargin: metrics.MaintenanceMargin,
		MarginRatio:       metrics.MarginRatio,
		RiskLevel:         metrics.RiskLevel,
		TriggeredBy:       triggeredBy,
	}
}

func (s *Service) currentConfig() ServiceConfig {
	if s.runtime == nil {
		return s.cfg
	}
	cfg := s.runtime.CurrentRiskRuntimeConfig()
	if cfg.RiskBufferRatio == "" || cfg.SoftThresholdRatio == "" || cfg.HardThresholdRatio == "" || cfg.TakerFeeRate == "" {
		return s.cfg
	}
	return cfg
}

func ComputeAccountMetrics(state AccountState, cfg ServiceConfig) AccountMetrics {
	wallet := decimalx.MustFromString(state.WalletBalance)
	orderMargin := decimalx.MustFromString(state.OrderMarginBalance)
	positionMargin := decimalx.MustFromString(state.PositionMarginBalance)
	withdrawHold := decimalx.MustFromString(state.WithdrawHoldBalance)

	collateral := wallet.Add(orderMargin).Add(positionMargin).Add(withdrawHold)
	totalInitial := decimalx.MustFromString("0")
	totalMaintenance := decimalx.MustFromString("0")
	totalUnrealized := decimalx.MustFromString("0")
	liquidationFeeBuffer := decimalx.MustFromString("0")
	totalNotional := decimalx.MustFromString("0")
	for _, position := range state.Positions {
		totalInitial = totalInitial.Add(decimalx.MustFromString(position.InitialMargin))
		totalMaintenance = totalMaintenance.Add(decimalx.MustFromString(position.MaintenanceMargin))
		totalUnrealized = totalUnrealized.Add(decimalx.MustFromString(position.UnrealizedPnL))
		notional := decimalx.MustFromString(position.Notional)
		totalNotional = totalNotional.Add(notional)
		liquidationFeeBuffer = liquidationFeeBuffer.Add(notional.Mul(decimalx.MustFromString(position.LiquidationFeeRate)))
	}

	pendingOrderInitialMargin := decimalx.MustFromString("0")
	pendingTradingFee := decimalx.MustFromString("0")
	for _, order := range state.PendingOrders {
		frozenInitialMargin, frozenFee := pendingOrderHoldComponents(order)
		pendingOrderInitialMargin = pendingOrderInitialMargin.Add(frozenInitialMargin)
		pendingTradingFee = pendingTradingFee.Add(frozenFee)
	}

	equity := collateral.Add(totalUnrealized).Sub(pendingTradingFee)
	riskBuffer := totalNotional.Mul(decimalx.MustFromString(cfg.RiskBufferRatio))
	available := equity.Sub(totalInitial).Sub(pendingOrderInitialMargin).Sub(withdrawHold).Sub(riskBuffer)
	denominator := totalMaintenance.Add(liquidationFeeBuffer)

	marginRatio := decimalx.MustFromString("0")
	riskLevel := RiskLevelSafe
	if available.LessThan(decimalx.MustFromString("0")) {
		riskLevel = RiskLevelNoNewRisk
	}
	if !denominator.IsZero() {
		marginRatio = equity.Div(denominator)
		if marginRatio.LessThanOrEqual(decimalx.MustFromString("1")) {
			riskLevel = RiskLevelLiquidating
		}
	}

	return AccountMetrics{
		Equity:               equity.String(),
		AvailableBalance:     available.String(),
		InitialMargin:        totalInitial.Add(pendingOrderInitialMargin).String(),
		MaintenanceMargin:    totalMaintenance.String(),
		UnrealizedPnL:        totalUnrealized.String(),
		MarginRatio:          marginRatio.String(),
		RiskLevel:            riskLevel,
		PendingTradingFee:    pendingTradingFee.String(),
		LiquidationFeeBuffer: liquidationFeeBuffer.String(),
		TotalNotional:        totalNotional.String(),
	}
}

func pendingOrderHoldComponents(order PendingOrderExposure) (decimalx.Decimal, decimalx.Decimal) {
	frozenInitialMargin := decimalx.MustFromString(defaultDecimal(order.FrozenInitialMargin))
	frozenFee := decimalx.MustFromString(defaultDecimal(order.FrozenFee))
	return frozenInitialMargin, frozenFee
}

func defaultDecimal(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return value
}

func (s *Service) validateMarkPrices(state AccountState, cfg ServiceConfig) error {
	if len(state.Positions) == 0 {
		return nil
	}
	now := s.clock.Now().UTC()
	staleThreshold := time.Duration(cfg.MarkPriceStaleSec) * time.Second
	for _, position := range state.Positions {
		if !decimalx.MustFromString(position.MarkPrice).GreaterThan(decimalx.MustFromString("0")) {
			return fmt.Errorf("%w: invalid mark price for symbol %s", errorsx.ErrConflict, position.Symbol)
		}
		if staleThreshold > 0 && (position.MarkPriceUpdatedAt.IsZero() || now.Sub(position.MarkPriceUpdatedAt) > staleThreshold) && cfg.ForceReduceOnlyOnStalePrice {
			return fmt.Errorf("%w: stale mark price for symbol %s", errorsx.ErrConflict, position.Symbol)
		}
	}
	return nil
}

func hedgeSideFromSigned(value decimalx.Decimal) string {
	if value.GreaterThan(decimalx.MustFromString("0")) {
		return hedgedomain.OrderSideBuy
	}
	return hedgedomain.OrderSideSell
}
