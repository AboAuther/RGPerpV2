package httptransport

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
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

type fakeMissingTransferResolver struct{}

func (fakeMissingTransferResolver) ResolveUserIDByAddress(_ context.Context, _ string) (uint64, error) {
	return 0, errorsx.ErrNotFound
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

func TestAccountHandler_TransferRejectsUnregisteredReceiver(t *testing.T) {
	transferUC := &fakeTransferUseCase{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		NewAccountHandler(fakeAccountReader{}, transferUC, fakeMissingTransferResolver{}),
		nil,
		nil,
		nil,
		nil,
	)

	body := []byte(`{"to_address":"0x0000000000000000000000000000000000000999","amount":"10","asset":"USDC"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/transfer", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace_2")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
	if transferUC.req.ToUserID != 0 {
		t.Fatalf("transfer use case should not be called on unregistered receiver: %+v", transferUC.req)
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte("收款地址未注册")) {
		t.Fatalf("expected unregistered receiver message, got %s", resp.Body.String())
	}
}
