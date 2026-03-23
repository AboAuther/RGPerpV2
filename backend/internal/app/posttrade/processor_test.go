package posttrade

import (
	"context"
	"testing"

	liquidationdomain "github.com/xiaobao/rgperp/backend/internal/domain/liquidation"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
)

type fakeRiskRecalculator struct {
	recalculateSnapshot riskdomain.Snapshot
	recalculateTrigger  *riskdomain.LiquidationTrigger
	recalculateErr      error
	refreshSnapshot     riskdomain.Snapshot
	refreshErr          error
	recalculateCalls    []string
	refreshCalls        []string
}

func (f *fakeRiskRecalculator) RecalculateAccountRisk(_ context.Context, _ uint64, triggeredBy string) (riskdomain.Snapshot, *riskdomain.LiquidationTrigger, error) {
	f.recalculateCalls = append(f.recalculateCalls, triggeredBy)
	return f.recalculateSnapshot, f.recalculateTrigger, f.recalculateErr
}

func (f *fakeRiskRecalculator) RefreshAccountRisk(_ context.Context, _ uint64, triggeredBy string) (riskdomain.Snapshot, error) {
	f.refreshCalls = append(f.refreshCalls, triggeredBy)
	return f.refreshSnapshot, f.refreshErr
}

type fakeLiquidationExecutor struct {
	response liquidationdomain.Liquidation
	err      error
	inputs   []liquidationdomain.ExecuteInput
}

func (f *fakeLiquidationExecutor) Execute(_ context.Context, input liquidationdomain.ExecuteInput) (liquidationdomain.Liquidation, error) {
	f.inputs = append(f.inputs, input)
	return f.response, f.err
}

func TestProcessorRecalculateAccountRefreshesRiskAfterExecutedLiquidation(t *testing.T) {
	risk := &fakeRiskRecalculator{
		recalculateSnapshot: riskdomain.Snapshot{
			ID:          10,
			UserID:      7,
			MarginRatio: "0.5",
			RiskLevel:   riskdomain.RiskLevelLiquidating,
			TriggeredBy: "admin",
		},
		recalculateTrigger: &riskdomain.LiquidationTrigger{
			LiquidationID: "liq_001",
			SnapshotID:    10,
		},
		refreshSnapshot: riskdomain.Snapshot{
			ID:          11,
			UserID:      7,
			MarginRatio: "0",
			RiskLevel:   riskdomain.RiskLevelSafe,
			TriggeredBy: "liquidation_executed",
		},
	}
	liquidation := &fakeLiquidationExecutor{
		response: liquidationdomain.Liquidation{
			ID:     "liq_001",
			UserID: 7,
			Status: liquidationdomain.StatusExecuted,
		},
	}
	processor := NewProcessor(risk, liquidation)

	result, err := processor.RecalculateAccount(context.Background(), 7, "admin-wallet")
	if err != nil {
		t.Fatalf("RecalculateAccount error = %v", err)
	}
	if result.UserID != 7 || result.RiskSnapshotID != 11 || result.MarginRatio != "0" || result.RiskLevel != riskdomain.RiskLevelSafe || result.TriggeredBy != "liquidation_executed" {
		t.Fatalf("unexpected result snapshot: %+v", result)
	}
	if result.LiquidationID == nil || *result.LiquidationID != "liq_001" {
		t.Fatalf("expected liquidation id liq_001, got %+v", result.LiquidationID)
	}
	if result.LiquidationStatus == nil || *result.LiquidationStatus != liquidationdomain.StatusExecuted {
		t.Fatalf("expected liquidation status EXECUTED, got %+v", result.LiquidationStatus)
	}
	if len(risk.refreshCalls) != 1 || risk.refreshCalls[0] != "liquidation_executed" {
		t.Fatalf("expected executed liquidation refresh, got %+v", risk.refreshCalls)
	}
	if len(liquidation.inputs) != 1 || liquidation.inputs[0].TriggerRiskSnapshotID != 10 {
		t.Fatalf("expected liquidation execute with snapshot 10, got %+v", liquidation.inputs)
	}
}

func TestProcessorEvaluateAccountSkipsRefreshForPendingManualLiquidation(t *testing.T) {
	risk := &fakeRiskRecalculator{
		recalculateSnapshot: riskdomain.Snapshot{
			ID:          21,
			UserID:      9,
			MarginRatio: "0.4",
			RiskLevel:   riskdomain.RiskLevelLiquidating,
			TriggeredBy: "admin",
		},
		recalculateTrigger: &riskdomain.LiquidationTrigger{
			LiquidationID: "liq_002",
			SnapshotID:    21,
		},
	}
	liquidation := &fakeLiquidationExecutor{
		response: liquidationdomain.Liquidation{
			ID:     "liq_002",
			UserID: 9,
			Status: liquidationdomain.StatusPendingManual,
		},
	}
	processor := NewProcessor(risk, liquidation)

	snapshot, gotLiquidation, err := processor.EvaluateAccount(context.Background(), 9, "admin", "trace-2")
	if err != nil {
		t.Fatalf("EvaluateAccount error = %v", err)
	}
	if snapshot.ID != 21 || snapshot.RiskLevel != riskdomain.RiskLevelLiquidating {
		t.Fatalf("expected original liquidating snapshot, got %+v", snapshot)
	}
	if gotLiquidation == nil || gotLiquidation.Status != liquidationdomain.StatusPendingManual {
		t.Fatalf("expected pending manual liquidation, got %+v", gotLiquidation)
	}
	if len(risk.refreshCalls) != 0 {
		t.Fatalf("expected no refresh for pending manual, got %+v", risk.refreshCalls)
	}
}

func TestProcessorEvaluateAccountRefreshesRiskAfterAbortedLiquidation(t *testing.T) {
	risk := &fakeRiskRecalculator{
		recalculateSnapshot: riskdomain.Snapshot{
			ID:          31,
			UserID:      12,
			MarginRatio: "0.7",
			RiskLevel:   riskdomain.RiskLevelLiquidating,
			TriggeredBy: "mark_price",
		},
		recalculateTrigger: &riskdomain.LiquidationTrigger{
			LiquidationID: "liq_003",
			SnapshotID:    31,
		},
		refreshSnapshot: riskdomain.Snapshot{
			ID:          32,
			UserID:      12,
			MarginRatio: "1.8",
			RiskLevel:   riskdomain.RiskLevelSafe,
			TriggeredBy: "liquidation_aborted",
		},
	}
	liquidation := &fakeLiquidationExecutor{
		response: liquidationdomain.Liquidation{
			ID:     "liq_003",
			UserID: 12,
			Status: liquidationdomain.StatusAborted,
		},
	}
	processor := NewProcessor(risk, liquidation)

	snapshot, gotLiquidation, err := processor.EvaluateAccount(context.Background(), 12, "mark_price", "trace-3")
	if err != nil {
		t.Fatalf("EvaluateAccount error = %v", err)
	}
	if snapshot.ID != 32 || snapshot.TriggeredBy != "liquidation_aborted" {
		t.Fatalf("expected aborted-refresh snapshot, got %+v", snapshot)
	}
	if gotLiquidation == nil || gotLiquidation.Status != liquidationdomain.StatusAborted {
		t.Fatalf("expected aborted liquidation, got %+v", gotLiquidation)
	}
	if len(risk.refreshCalls) != 1 || risk.refreshCalls[0] != "liquidation_aborted" {
		t.Fatalf("expected aborted liquidation refresh, got %+v", risk.refreshCalls)
	}
}
