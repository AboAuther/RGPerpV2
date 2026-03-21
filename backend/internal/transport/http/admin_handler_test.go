package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
)

type fakeWithdrawApprover struct {
	inputSeen bool
}

func (f *fakeWithdrawApprover) ApproveWithdraw(_ context.Context, _ walletdomain.ApproveWithdrawInput) error {
	f.inputSeen = true
	return nil
}

func TestAdminHandler_ApproveWithdrawal(t *testing.T) {
	approver := &fakeWithdrawApprover{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		nil,
		NewAdminHandler(approver, []string{"0xabc"}),
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
