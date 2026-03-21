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

type fakeWalletReader struct{}

func (fakeWalletReader) ListDepositAddresses(_ context.Context, _ uint64) ([]walletdomain.DepositAddress, error) {
	return []walletdomain.DepositAddress{{ChainID: 8453, Asset: "USDC", Address: "0xabc", Confirmations: 20}}, nil
}
func (fakeWalletReader) ListDeposits(_ context.Context, _ uint64) ([]readmodel.DepositItem, error) {
	return []readmodel.DepositItem{{DepositID: "dep_1", Status: "CREDITED"}}, nil
}
func (fakeWalletReader) ListWithdrawals(_ context.Context, _ uint64) ([]readmodel.WithdrawItem, error) {
	return []readmodel.WithdrawItem{{WithdrawID: "wd_1", Status: "HOLD"}}, nil
}

type fakeWalletMutator struct {
	withdrawInput walletdomain.RequestWithdrawInput
	faucetInput   walletdomain.GrantReviewFaucetInput
}

func (f *fakeWalletMutator) RequestWithdraw(_ context.Context, input walletdomain.RequestWithdrawInput) (walletdomain.WithdrawRequest, error) {
	f.withdrawInput = input
	return walletdomain.WithdrawRequest{WithdrawID: "wd_1", Status: "HOLD"}, nil
}
func (f *fakeWalletMutator) GrantReviewFaucet(_ context.Context, input walletdomain.GrantReviewFaucetInput) (walletdomain.DepositChainTx, error) {
	f.faucetInput = input
	return walletdomain.DepositChainTx{DepositID: "dep_1", Status: walletdomain.StatusCredited}, nil
}

func TestWalletHandler_CreateWithdrawal(t *testing.T) {
	mutator := &fakeWalletMutator{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		NewWalletHandler(fakeWalletReader{}, mutator, false),
		nil,
		nil,
	)

	body := []byte(`{"chain_id":8453,"asset":"USDC","amount":"100","to_address":"0x0000000000000000000000000000000000000001"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/wallet/withdrawals", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem_1")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if mutator.withdrawInput.UserID != 7 || mutator.withdrawInput.IdempotencyKey != "idem_1" {
		t.Fatalf("unexpected withdraw input: %+v", mutator.withdrawInput)
	}
}

func TestWalletHandler_RequestFaucet(t *testing.T) {
	mutator := &fakeWalletMutator{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0x0000000000000000000000000000000000000001"}},
		nil,
		nil,
		NewWalletHandler(fakeWalletReader{}, mutator, true),
		nil,
		nil,
	)

	body := []byte(`{"address":"0x0000000000000000000000000000000000000001","chain_id":8453}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/review/faucet", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if mutator.faucetInput.UserID != 7 || mutator.faucetInput.ChainID != 8453 {
		t.Fatalf("unexpected faucet input: %+v", mutator.faucetInput)
	}
}

func TestWalletHandler_RequestFaucet_RejectsWhenDisabled(t *testing.T) {
	mutator := &fakeWalletMutator{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0x0000000000000000000000000000000000000001"}},
		nil,
		nil,
		NewWalletHandler(fakeWalletReader{}, mutator, false),
		nil,
		nil,
	)

	body := []byte(`{"address":"0x0000000000000000000000000000000000000001","chain_id":8453}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/review/faucet", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.Code)
	}
	if mutator.faucetInput.UserID != 0 {
		t.Fatalf("expected faucet mutator not called")
	}
}
