package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
)

type fakeWithdrawApprover struct {
	inputSeen  bool
	reviewSeen bool
	refundSeen bool
}

func (f *fakeWithdrawApprover) ApproveWithdraw(_ context.Context, _ walletdomain.ApproveWithdrawInput) error {
	f.inputSeen = true
	return nil
}

func (f *fakeWithdrawApprover) ReturnWithdrawToReview(_ context.Context, _ walletdomain.ReturnWithdrawToReviewInput) error {
	f.reviewSeen = true
	return nil
}

func (f *fakeWithdrawApprover) RefundWithdraw(_ context.Context, _ walletdomain.RefundWithdrawInput) error {
	f.refundSeen = true
	return nil
}

type fakeAdminWithdrawalReader struct {
	scopeSeen string
}

type fakeAdminRiskMutator struct {
	userIDSeen   uint64
	operatorSeen string
	response     readmodel.AdminRiskRecalculationResult
}

type fakeAdminConfigManager struct {
	view         readmodel.RuntimeConfigView
	updateSeen   AdminRuntimeConfigUpdateInput
	getLimitSeen int
}

type fakeAdminFundingMutator struct {
	batchIDSeen  string
	operatorSeen string
	traceSeen    string
	reasonSeen   string
	result       readmodel.AdminFundingBatchItem
}

type fakeAdminLedgerMutator struct {
	inputSeen AdminInsuranceFundTopUpInput
}

type fakeAdminLiquidationMutator struct {
	retrySeen AdminLiquidationActionInput
	closeSeen AdminLiquidationCloseInput
}

func (f *fakeAdminRiskMutator) RecalculateAccount(_ context.Context, userID uint64, operatorID string) (readmodel.AdminRiskRecalculationResult, error) {
	f.userIDSeen = userID
	f.operatorSeen = operatorID
	if f.response.UserID == 0 {
		f.response.UserID = userID
	}
	return f.response, nil
}

func (f *fakeAdminConfigManager) GetRuntimeConfigView(_ context.Context, limit int) (readmodel.RuntimeConfigView, error) {
	f.getLimitSeen = limit
	return f.view, nil
}

func (f *fakeAdminConfigManager) UpdateRuntimeConfig(_ context.Context, input AdminRuntimeConfigUpdateInput) (readmodel.RuntimeConfigView, error) {
	f.updateSeen = input
	return f.view, nil
}

func (f *fakeAdminFundingMutator) ReverseFundingBatch(_ context.Context, fundingBatchID string, operatorID string, traceID string, reason string) (readmodel.AdminFundingBatchItem, error) {
	f.batchIDSeen = fundingBatchID
	f.operatorSeen = operatorID
	f.traceSeen = traceID
	f.reasonSeen = reason
	if f.result.FundingBatchID == "" {
		f.result.FundingBatchID = fundingBatchID
	}
	return f.result, nil
}

func (f *fakeAdminLedgerMutator) TopUpInsuranceFund(_ context.Context, input AdminInsuranceFundTopUpInput) (map[string]any, error) {
	f.inputSeen = input
	return map[string]any{"status": "COMMITTED"}, nil
}

func (f *fakeAdminLiquidationMutator) RetryPendingLiquidation(_ context.Context, input AdminLiquidationActionInput) (map[string]any, error) {
	f.retrySeen = input
	return map[string]any{"liquidation_id": input.LiquidationID, "status": "EXECUTED"}, nil
}

func (f *fakeAdminLiquidationMutator) ClosePendingLiquidation(_ context.Context, input AdminLiquidationCloseInput) (map[string]any, error) {
	f.closeSeen = input
	return map[string]any{"liquidation_id": input.LiquidationID, "status": "ABORTED"}, nil
}

func (f *fakeAdminWithdrawalReader) ListAdminWithdrawals(_ context.Context, _ int) ([]readmodel.AdminWithdrawReviewItem, error) {
	return nil, nil
}

func (f *fakeAdminWithdrawalReader) ListAdminLiquidations(_ context.Context, _ int) ([]readmodel.AdminLiquidationItem, error) {
	return []readmodel.AdminLiquidationItem{{LiquidationID: "liq_1", Status: "EXECUTED"}}, nil
}

func (f *fakeAdminWithdrawalReader) ListFundingBatches(_ context.Context, _ int) ([]readmodel.AdminFundingBatchItem, error) {
	return []readmodel.AdminFundingBatchItem{{FundingBatchID: "fb_1", Status: "APPLIED"}}, nil
}

func (f *fakeAdminWithdrawalReader) ListAdminHedgeIntents(_ context.Context, _ int) ([]readmodel.AdminHedgeIntentItem, error) {
	return []readmodel.AdminHedgeIntentItem{{HedgeIntentID: "hint_1", Symbol: "BTC-PERP", Status: "COMPLETED"}}, nil
}

func (f *fakeAdminWithdrawalReader) ListLatestSystemHedgeSnapshots(_ context.Context, _ int) ([]readmodel.SystemHedgeSnapshotItem, error) {
	return []readmodel.SystemHedgeSnapshotItem{{Symbol: "BTC-PERP", HedgeHealthy: true, CreatedAt: "2026-03-24T00:00:00Z"}}, nil
}

func (f *fakeAdminWithdrawalReader) GetLedgerOverview(_ context.Context, scope string) (readmodel.LedgerOverview, error) {
	f.scopeSeen = scope
	return readmodel.LedgerOverview{}, nil
}

func (f *fakeAdminWithdrawalReader) GetLatestLedgerAuditReport(_ context.Context, scope string) (readmodel.LedgerAuditReport, error) {
	f.scopeSeen = scope
	return readmodel.LedgerAuditReport{
		AuditReportID: "audit_1",
		ScopeAsset:    "USDC",
		Status:        "PASS",
		ExecutedBy:    "0xabc",
	}, nil
}

func (f *fakeAdminWithdrawalReader) RunLedgerAudit(_ context.Context, _, _ string) (readmodel.LedgerAuditReport, error) {
	return readmodel.LedgerAuditReport{}, nil
}

func (f *fakeAdminWithdrawalReader) GetRiskMonitorDashboard(_ context.Context) (readmodel.RiskMonitorDashboard, error) {
	return readmodel.RiskMonitorDashboard{}, nil
}

func TestAdminHandler_ApproveWithdrawal(t *testing.T) {
	approver := &fakeWithdrawApprover{}
	reader := &fakeAdminWithdrawalReader{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewAdminHandler(approver, reader, []string{"0xabc"}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/withdrawals/wd_1/approve", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if !approver.inputSeen {
		t.Fatal("expected approve use case to be invoked")
	}
}

func TestAdminHandler_LedgerOverviewPassesAssetScope(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ledger/overview?asset=USDC", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if reader.scopeSeen != "USDC" {
		t.Fatalf("expected scope USDC, got %q", reader.scopeSeen)
	}
}

func TestAdminHandler_ExportLatestLedgerAuditCSV(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ledger/audits/latest/export?asset=USDC&format=csv", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if got := resp.Header().Get("Content-Type"); got == "" || !strings.HasPrefix(got, "text/csv") {
		t.Fatalf("expected csv response, got %q", got)
	}
	if reader.scopeSeen != "USDC" {
		t.Fatalf("expected scope USDC, got %q", reader.scopeSeen)
	}
}

func TestAdminHandler_RiskMonitorDashboard(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/risk/net-exposures", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}

func TestAdminHandler_ListLiquidations(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/liquidations", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "liq_1") {
		t.Fatalf("expected liquidation payload, got %s", resp.Body.String())
	}
}

func TestAdminHandler_ListHedgeIntents(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/hedges/intents", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "hint_1") {
		t.Fatalf("expected hedge intent payload, got %s", resp.Body.String())
	}
}

func TestAdminHandler_ListHedgeSnapshots(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/hedges/snapshots", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "BTC-PERP") {
		t.Fatalf("expected hedge snapshot payload, got %s", resp.Body.String())
	}
}

func TestAdminHandler_RetryLiquidation(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	mutator := &fakeAdminLiquidationMutator{}
	adminHandler := NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"})
	adminHandler.SetLiquidationMutator(mutator)
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		adminHandler,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/liquidations/liq_9/retry", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_retry")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if mutator.retrySeen.LiquidationID != "liq_9" || mutator.retrySeen.OperatorID != "0xabc" || mutator.retrySeen.TraceID != "trace_retry" {
		t.Fatalf("unexpected retry input: %+v", mutator.retrySeen)
	}
}

func TestAdminHandler_CloseLiquidation(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	mutator := &fakeAdminLiquidationMutator{}
	adminHandler := NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"})
	adminHandler.SetLiquidationMutator(mutator)
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		adminHandler,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/liquidations/liq_9/close", strings.NewReader(`{"reason":"position already closed"}`))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_close")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if mutator.closeSeen.LiquidationID != "liq_9" || mutator.closeSeen.Reason != "position already closed" {
		t.Fatalf("unexpected close input: %+v", mutator.closeSeen)
	}
}

func TestAdminHandler_ReturnWithdrawalToReview(t *testing.T) {
	approver := &fakeWithdrawApprover{}
	reader := &fakeAdminWithdrawalReader{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewAdminHandler(approver, reader, []string{"0xabc"}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/withdrawals/wd_1/review", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if !approver.reviewSeen {
		t.Fatal("expected return-to-review use case to be invoked")
	}
}

func TestAdminHandler_RefundWithdrawal(t *testing.T) {
	approver := &fakeWithdrawApprover{}
	reader := &fakeAdminWithdrawalReader{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewAdminHandler(approver, reader, []string{"0xabc"}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/withdrawals/wd_1/refund", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	req.Header.Set("Idempotency-Key", "idem_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if !approver.refundSeen {
		t.Fatal("expected refund use case to be invoked")
	}
}

func TestAdminHandler_TopUpInsuranceFund(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	mutator := &fakeAdminLedgerMutator{}
	adminHandler := NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"})
	adminHandler.SetLedgerMutator(mutator)
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		adminHandler,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/ledger/insurance-fund/topups", strings.NewReader(`{"asset":"USDC","amount":"25","source_account":"SYSTEM_POOL","reason":"test top-up"}`))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_topup")
	req.Header.Set("Idempotency-Key", "idem_topup")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if mutator.inputSeen.Asset != "USDC" || mutator.inputSeen.Amount != "25" || mutator.inputSeen.SourceAccount != "SYSTEM_POOL" || mutator.inputSeen.Reason != "test top-up" {
		t.Fatalf("unexpected top-up input: %+v", mutator.inputSeen)
	}
	if mutator.inputSeen.IdempotencyKey != "idem_topup" || mutator.inputSeen.TraceID != "trace_topup" || mutator.inputSeen.OperatorID != "0xabc" {
		t.Fatalf("unexpected top-up metadata: %+v", mutator.inputSeen)
	}
}

func TestAdminHandler_RecalculateAccountRisk(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	riskMutator := &fakeAdminRiskMutator{
		response: readmodel.AdminRiskRecalculationResult{
			UserID:      7,
			TriggeredBy: "admin",
		},
	}
	adminHandler := NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"})
	adminHandler.SetRiskMutator(riskMutator)
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		adminHandler,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/risk/accounts/7/recalculate", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if riskMutator.userIDSeen != 7 {
		t.Fatalf("expected user_id=7, got %d", riskMutator.userIDSeen)
	}
	if riskMutator.operatorSeen != "0xabc" {
		t.Fatalf("expected operator 0xabc, got %q", riskMutator.operatorSeen)
	}
}

func TestAdminHandler_GetRuntimeConfig(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	configManager := &fakeAdminConfigManager{
		view: readmodel.RuntimeConfigView{
			GeneratedAt: "2026-03-23T00:00:00Z",
			Snapshot: readmodel.RuntimeConfigSnapshotView{
				ReadOnly:                   true,
				ReduceOnly:                 true,
				RiskNetExposureHardLimit:   "100000",
				RiskMaxExposureSlippageBps: 80,
			},
		},
	}
	adminHandler := NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"})
	adminHandler.SetConfigManager(configManager)
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		adminHandler,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/configs/runtime?limit=15", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if configManager.getLimitSeen != 15 {
		t.Fatalf("expected limit 15, got %d", configManager.getLimitSeen)
	}
}

func TestAdminHandler_UpdateRuntimeConfig(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	configManager := &fakeAdminConfigManager{
		view: readmodel.RuntimeConfigView{
			Snapshot: readmodel.RuntimeConfigSnapshotView{
				ReadOnly: true,
			},
		},
	}
	adminHandler := NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"})
	adminHandler.SetConfigManager(configManager)
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		adminHandler,
	)

	body := strings.NewReader(`{"reason":"enable kill switch","global":{"read_only":true,"reduce_only":true},"risk":{"net_exposure_hard_limit":"250000"},"funding":{"interval_sec":7200,"min_valid_source_count":2}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/configs/runtime", body)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace_2")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if configManager.updateSeen.OperatorID != "0xabc" {
		t.Fatalf("expected operator 0xabc, got %q", configManager.updateSeen.OperatorID)
	}
	if configManager.updateSeen.Reason != "enable kill switch" {
		t.Fatalf("unexpected reason %q", configManager.updateSeen.Reason)
	}
	if _, ok := configManager.updateSeen.Values["system.read_only"]; !ok {
		t.Fatalf("expected read_only patch")
	}
	if _, ok := configManager.updateSeen.Values["risk.net_exposure_hard_limit"]; !ok {
		t.Fatalf("expected net exposure patch")
	}
	if _, ok := configManager.updateSeen.Values["funding.interval_sec"]; !ok {
		t.Fatalf("expected funding interval patch")
	}
	if _, ok := configManager.updateSeen.Values["funding.min_valid_source_count"]; !ok {
		t.Fatalf("expected funding valid source patch")
	}
}

func TestAdminHandler_UpdateRuntimeConfigWithPairOverrides(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	configManager := &fakeAdminConfigManager{}
	adminHandler := NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"})
	adminHandler.SetConfigManager(configManager)
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		adminHandler,
	)

	body := strings.NewReader(`{"reason":"pair tuning","pairs":{"BTC-USDC":{"market":{"max_leverage":"25","session_policy":"US_EQUITY_REGULAR","taker_fee_rate":"0.0008","default_max_slippage_bps":60},"risk":{"liquidation_penalty_rate":"0.015","maintenance_margin_uplift_ratio":"0.10"},"funding":{"interval_sec":1800}}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/configs/runtime", body)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace_4")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	pairValues, ok := configManager.updateSeen.PairValues["BTC-USDC"]
	if !ok {
		t.Fatalf("expected BTC-USDC pair patch")
	}
	if _, ok := pairValues["market.max_leverage"]; !ok {
		t.Fatalf("expected pair max leverage patch")
	}
	if _, ok := pairValues["market.session_policy"]; !ok {
		t.Fatalf("expected pair session policy patch")
	}
	if _, ok := pairValues["market.taker_fee_rate"]; !ok {
		t.Fatalf("expected pair taker fee patch")
	}
	if _, ok := pairValues["market.default_max_slippage_bps"]; !ok {
		t.Fatalf("expected pair slippage patch")
	}
	if _, ok := pairValues["risk.liquidation_penalty_rate"]; !ok {
		t.Fatalf("expected pair liquidation penalty patch")
	}
	if _, ok := pairValues["risk.maintenance_margin_uplift_ratio"]; !ok {
		t.Fatalf("expected pair maintenance uplift patch")
	}
	if _, ok := pairValues["funding.interval_sec"]; !ok {
		t.Fatalf("expected pair funding interval patch")
	}
}

func TestAdminHandler_ReverseFundingBatch(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	fundingMutator := &fakeAdminFundingMutator{
		result: readmodel.AdminFundingBatchItem{
			FundingBatchID: "fb_1",
			Status:         "REVERSED",
		},
	}
	adminHandler := NewAdminHandler(&fakeWithdrawApprover{}, reader, []string{"0xabc"})
	adminHandler.SetFundingMutator(fundingMutator)
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		adminHandler,
	)

	body := strings.NewReader(`{"reason":"manual correction"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/funding-batches/fb_1/reverse", body)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace_3")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if fundingMutator.batchIDSeen != "fb_1" {
		t.Fatalf("expected batch id fb_1, got %q", fundingMutator.batchIDSeen)
	}
	if fundingMutator.operatorSeen != "0xabc" {
		t.Fatalf("expected operator 0xabc, got %q", fundingMutator.operatorSeen)
	}
	if fundingMutator.reasonSeen != "manual correction" {
		t.Fatalf("expected reason manual correction, got %q", fundingMutator.reasonSeen)
	}
}
