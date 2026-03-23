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
	inputSeen bool
}

func (f *fakeWithdrawApprover) ApproveWithdraw(_ context.Context, _ walletdomain.ApproveWithdrawInput) error {
	f.inputSeen = true
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

func (f *fakeAdminWithdrawalReader) ListAdminWithdrawals(_ context.Context, _ int) ([]readmodel.AdminWithdrawReviewItem, error) {
	return nil, nil
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

func TestAdminHandler_RecalculateAccountRisk(t *testing.T) {
	reader := &fakeAdminWithdrawalReader{}
	riskMutator := &fakeAdminRiskMutator{
		response: readmodel.AdminRiskRecalculationResult{
			UserID:         7,
			RiskSnapshotID: 101,
			MarginRatio:    "0.9",
			RiskLevel:      "LIQUIDATING",
			TriggeredBy:    "admin",
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

	body := strings.NewReader(`{"reason":"enable kill switch","global":{"read_only":true,"reduce_only":true},"risk":{"net_exposure_hard_limit":"250000"}}`)
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
}
