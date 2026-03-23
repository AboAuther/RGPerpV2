package httptransport

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
)

type fakeAccessVerifier struct {
	claims AccessClaims
	err    error
}

func (f fakeAccessVerifier) VerifyAccessToken(_ string) (AccessClaims, error) {
	return f.claims, f.err
}

type fakeAccountReader struct{}

func (fakeAccountReader) GetSummary(_ context.Context, _ uint64) (readmodel.AccountSummary, error) {
	return readmodel.AccountSummary{Equity: "100", AvailableBalance: "50"}, nil
}
func (fakeAccountReader) ListBalances(_ context.Context, _ uint64) ([]readmodel.BalanceItem, error) {
	return []readmodel.BalanceItem{{AccountCode: "USER_WALLET", Asset: "USDC", Balance: "50"}}, nil
}
func (fakeAccountReader) GetRisk(_ context.Context, _ uint64) (readmodel.RiskSnapshot, error) {
	return readmodel.RiskSnapshot{AccountStatus: "ACTIVE", RiskState: "SAFE", Notes: []string{"ok"}}, nil
}
func (fakeAccountReader) ListFunding(_ context.Context, _ uint64) ([]readmodel.FundingItem, error) {
	return []readmodel.FundingItem{{FundingID: "fund_1", Symbol: "BTC-PERP", Direction: "PAY", Amount: "-1"}}, nil
}
func (fakeAccountReader) ListTransfers(_ context.Context, _ uint64) ([]readmodel.TransferItem, error) {
	return []readmodel.TransferItem{{TransferID: "trf_1", Asset: "USDC", Amount: "10", Status: "COMMITTED"}}, nil
}

type fakeTransferUseCase struct {
	req walletdomain.TransferRequest
}

func (f *fakeTransferUseCase) Transfer(_ context.Context, req walletdomain.TransferRequest) error {
	f.req = req
	return nil
}

type fakeTransferResolver struct{}

func (fakeTransferResolver) ResolveUserIDByAddress(_ context.Context, _ string) (uint64, error) {
	return 88, nil
}

func TestAccountHandler_GetSummary(t *testing.T) {
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		NewAccountHandler(fakeAccountReader{}, &fakeTransferUseCase{}, fakeTransferResolver{}),
		nil,
		nil,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/summary", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}

func TestAccountHandler_Transfer(t *testing.T) {
	transferUC := &fakeTransferUseCase{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		NewAccountHandler(fakeAccountReader{}, transferUC, fakeTransferResolver{}),
		nil,
		nil,
		nil,
		nil,
	)

	body := []byte(`{"to_address":"0x0000000000000000000000000000000000000002","amount":"10","asset":"USDC"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/transfer", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace_1")
	req.Header.Set("Idempotency-Key", "idem_transfer_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if transferUC.req.ToUserID != 88 || transferUC.req.FromUserID != 7 {
		t.Fatalf("unexpected transfer request: %+v", transferUC.req)
	}
	if transferUC.req.IdempotencyKey != "idem_transfer_1" {
		t.Fatalf("unexpected idempotency key: %+v", transferUC.req)
	}
}
