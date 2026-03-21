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
	return []walletdomain.DepositAddress{{ChainID: 31337, Asset: "USDC", Address: "0x00000000000000000000000000000000000000ab", Confirmations: 1}}, nil
}
func (fakeWalletReader) ListDeposits(_ context.Context, _ uint64) ([]readmodel.DepositItem, error) {
	return []readmodel.DepositItem{{DepositID: "dep_1", Status: "CREDITED"}}, nil
}
func (fakeWalletReader) ListWithdrawals(_ context.Context, _ uint64) ([]readmodel.WithdrawItem, error) {
	return []readmodel.WithdrawItem{{WithdrawID: "wd_1", Status: "HOLD"}}, nil
}

type fakeWalletMutator struct {
	withdrawInput        walletdomain.RequestWithdrawInput
	generateAddressInput walletdomain.GenerateDepositAddressInput
}

func (f *fakeWalletMutator) RequestWithdraw(_ context.Context, input walletdomain.RequestWithdrawInput) (walletdomain.WithdrawRequest, error) {
	f.withdrawInput = input
	return walletdomain.WithdrawRequest{WithdrawID: "wd_1", Status: "HOLD"}, nil
}

func (f *fakeWalletMutator) GenerateDepositAddress(_ context.Context, input walletdomain.GenerateDepositAddressInput) (walletdomain.DepositAddress, error) {
	f.generateAddressInput = input
	return walletdomain.DepositAddress{
		UserID:        input.UserID,
		ChainID:       input.ChainID,
		Asset:         "USDC",
		Address:       "0x00000000000000000000000000000000000000ab",
		Confirmations: 1,
		Status:        "ACTIVE",
	}, nil
}

type fakeLocalChainSupport struct {
	address string
	chainID int64
}

func (f *fakeLocalChainSupport) GrantNativeToken(_ context.Context, address string, chainID int64) (string, error) {
	f.address = address
	f.chainID = chainID
	return "0xtx", nil
}

func TestWalletHandler_CreateWithdrawal(t *testing.T) {
	mutator := &fakeWalletMutator{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		NewWalletHandler(fakeWalletReader{}, mutator, nil),
		nil,
		nil,
		nil,
	)

	body := []byte(`{"chain_id":31337,"asset":"USDC","amount":"100","to_address":"0x0000000000000000000000000000000000000001"}`)
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

func TestWalletHandler_GenerateDepositAddress(t *testing.T) {
	mutator := &fakeWalletMutator{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0xabc"}},
		nil,
		nil,
		nil,
		NewWalletHandler(fakeWalletReader{}, mutator, nil),
		nil,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wallet/deposit-addresses/31337/generate", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if mutator.generateAddressInput.UserID != 7 || mutator.generateAddressInput.ChainID != 31337 {
		t.Fatalf("unexpected generate address input: %+v", mutator.generateAddressInput)
	}
}

func TestWalletHandler_RequestLocalNativeFaucet(t *testing.T) {
	tools := &fakeLocalChainSupport{}
	engine := NewEngine(
		fakeAccessVerifier{claims: AccessClaims{UserID: "7", Address: "0x0000000000000000000000000000000000000001"}},
		nil,
		nil,
		nil,
		NewWalletHandler(fakeWalletReader{}, &fakeWalletMutator{}, tools),
		nil,
		nil,
		nil,
	)

	body := []byte(`{"chain_id":31337}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/wallet/local-faucet/native", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace_1")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if tools.address != "0x0000000000000000000000000000000000000001" || tools.chainID != 31337 {
		t.Fatalf("unexpected faucet input: %+v", tools)
	}
}
