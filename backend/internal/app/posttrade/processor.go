package posttrade

import (
	"context"
	"strings"

	liquidationdomain "github.com/xiaobao/rgperp/backend/internal/domain/liquidation"
	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
)

type riskRecalculator interface {
	RecalculateAccountRisk(ctx context.Context, userID uint64, triggeredBy string) (riskdomain.Snapshot, *riskdomain.LiquidationTrigger, error)
	RefreshAccountRisk(ctx context.Context, userID uint64, triggeredBy string) (riskdomain.Snapshot, error)
}

type liquidationExecutor interface {
	Execute(ctx context.Context, input liquidationdomain.ExecuteInput) (liquidationdomain.Liquidation, error)
}

type Processor struct {
	risk        riskRecalculator
	liquidation liquidationExecutor
}

func NewProcessor(risk riskRecalculator, liquidation liquidationExecutor) *Processor {
	if risk == nil || liquidation == nil {
		return nil
	}
	return &Processor{risk: risk, liquidation: liquidation}
}

func (p *Processor) RecalculateAfterTrade(ctx context.Context, userID uint64, traceID string) error {
	_, _, err := p.recalculateAndMaybeLiquidate(ctx, userID, "trade_fill", traceID)
	return err
}

func (p *Processor) EvaluateAccount(ctx context.Context, userID uint64, triggeredBy string, traceID string) (riskdomain.Snapshot, *liquidationdomain.Liquidation, error) {
	return p.recalculateAndMaybeLiquidate(ctx, userID, triggeredBy, traceID)
}

func (p *Processor) RecalculateAccount(ctx context.Context, userID uint64, operatorID string) (readmodel.AdminRiskRecalculationResult, error) {
	snapshot, liquidation, err := p.recalculateAndMaybeLiquidate(ctx, userID, "admin", "admin:"+operatorID+":risk_recalc")
	if err != nil {
		return readmodel.AdminRiskRecalculationResult{}, err
	}
	result := readmodel.AdminRiskRecalculationResult{
		UserID:         userID,
		RiskSnapshotID: snapshot.ID,
		MarginRatio:    snapshot.MarginRatio,
		RiskLevel:      snapshot.RiskLevel,
		TriggeredBy:    snapshot.TriggeredBy,
	}
	if liquidation != nil {
		result.LiquidationID = &liquidation.ID
		result.LiquidationStatus = &liquidation.Status
	}
	return result, nil
}

func (p *Processor) recalculateAndMaybeLiquidate(ctx context.Context, userID uint64, triggeredBy string, traceID string) (riskdomain.Snapshot, *liquidationdomain.Liquidation, error) {
	if p == nil || p.risk == nil || p.liquidation == nil {
		return riskdomain.Snapshot{}, nil, nil
	}
	snapshot, trigger, err := p.risk.RecalculateAccountRisk(ctx, userID, triggeredBy)
	if err != nil {
		return riskdomain.Snapshot{}, nil, err
	}
	if trigger == nil {
		return snapshot, nil, nil
	}
	if strings.TrimSpace(traceID) == "" {
		traceID = triggeredBy + ":" + trigger.LiquidationID
	}
	liquidation, err := p.liquidation.Execute(ctx, liquidationdomain.ExecuteInput{
		LiquidationID:         trigger.LiquidationID,
		UserID:                userID,
		TriggerRiskSnapshotID: snapshot.ID,
		TraceID:               traceID,
	})
	if err != nil {
		return riskdomain.Snapshot{}, nil, err
	}
	if refreshed, err := p.refreshRiskSnapshotAfterLiquidation(ctx, userID, liquidation); err != nil {
		return riskdomain.Snapshot{}, nil, err
	} else if refreshed != nil {
		snapshot = *refreshed
	}
	return snapshot, &liquidation, nil
}

func (p *Processor) refreshRiskSnapshotAfterLiquidation(ctx context.Context, userID uint64, liquidation liquidationdomain.Liquidation) (*riskdomain.Snapshot, error) {
	var triggeredBy string
	switch liquidation.Status {
	case liquidationdomain.StatusExecuted:
		triggeredBy = "liquidation_executed"
	case liquidationdomain.StatusAborted:
		triggeredBy = "liquidation_aborted"
	default:
		return nil, nil
	}
	snapshot, err := p.risk.RefreshAccountRisk(ctx, userID, triggeredBy)
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}
