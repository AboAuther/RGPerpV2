package main

import (
	"context"

	adminopsapp "github.com/xiaobao/rgperp/backend/internal/app/adminops"
	httptransport "github.com/xiaobao/rgperp/backend/internal/transport/http"
)

type adminLedgerManager struct {
	service *adminopsapp.Service
}

func (m adminLedgerManager) TopUpInsuranceFund(ctx context.Context, input httptransport.AdminInsuranceFundTopUpInput) (map[string]any, error) {
	result, err := m.service.TopUpInsuranceFund(ctx, adminopsapp.InsuranceFundTopUpInput{
		OperatorID:     input.OperatorID,
		TraceID:        input.TraceID,
		IdempotencyKey: input.IdempotencyKey,
		Reason:         input.Reason,
		Asset:          input.Asset,
		Amount:         input.Amount,
		SourceAccount:  input.SourceAccount,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"topup_id":       result.TopUpID,
		"asset":          result.Asset,
		"amount":         result.Amount,
		"source_account": result.SourceAccount,
		"status":         result.Status,
	}, nil
}

type adminLiquidationManager struct {
	service *adminopsapp.Service
}

func (m adminLiquidationManager) RetryPendingLiquidation(ctx context.Context, input httptransport.AdminLiquidationActionInput) (map[string]any, error) {
	result, err := m.service.RetryPendingLiquidation(ctx, adminopsapp.LiquidationActionInput{
		LiquidationID: input.LiquidationID,
		OperatorID:    input.OperatorID,
		TraceID:       input.TraceID,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"liquidation_id": result.LiquidationID,
		"status":         result.Status,
		"abort_reason":   result.AbortReason,
	}, nil
}

func (m adminLiquidationManager) ClosePendingLiquidation(ctx context.Context, input httptransport.AdminLiquidationCloseInput) (map[string]any, error) {
	result, err := m.service.ClosePendingLiquidation(ctx, adminopsapp.LiquidationActionInput{
		LiquidationID: input.LiquidationID,
		OperatorID:    input.OperatorID,
		TraceID:       input.TraceID,
		Reason:        input.Reason,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"liquidation_id": result.LiquidationID,
		"status":         result.Status,
		"abort_reason":   result.AbortReason,
	}, nil
}
