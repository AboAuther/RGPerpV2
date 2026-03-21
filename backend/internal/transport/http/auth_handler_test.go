package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type fakeAuthUseCase struct {
	issueNonceResp authdomain.IssueNonceOutput
	issueNonceErr  error
	loginResp      authdomain.LoginResult
	loginErr       error
	lastNonceInput authdomain.IssueNonceInput
	lastLoginInput authdomain.LoginInput
}

func (f *fakeAuthUseCase) IssueNonce(_ context.Context, input authdomain.IssueNonceInput) (authdomain.IssueNonceOutput, error) {
	f.lastNonceInput = input
	return f.issueNonceResp, f.issueNonceErr
}

func (f *fakeAuthUseCase) Login(_ context.Context, input authdomain.LoginInput) (authdomain.LoginResult, error) {
	f.lastLoginInput = input
	return f.loginResp, f.loginErr
}

func TestIssueNonceHandler_Success(t *testing.T) {
	fakeUC := &fakeAuthUseCase{
		issueNonceResp: authdomain.IssueNonceOutput{
			Nonce:     "challenge_1",
			Domain:    "localhost",
			ChainID:   8453,
			ExpiresAt: time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
		},
	}
	engine := NewEngine(nil, NewAuthHandler(fakeUC), nil, nil, nil, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"address":  "0x0000000000000000000000000000000000000001",
		"chain_id": 8453,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/nonce", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Trace-Id", "trace_1")
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fakeUC.lastNonceInput.ChainID != 8453 {
		t.Fatalf("expected chain id propagated")
	}
}

func TestLoginHandler_UsesFallbackIPAndUA(t *testing.T) {
	fakeUC := &fakeAuthUseCase{
		loginResp: authdomain.LoginResult{
			User: authdomain.User{
				ID:         42,
				EVMAddress: "0x0000000000000000000000000000000000000001",
				Status:     "ACTIVE",
			},
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
		},
	}
	engine := NewEngine(nil, NewAuthHandler(fakeUC), nil, nil, nil, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"address":            "0x0000000000000000000000000000000000000001",
		"chain_id":           8453,
		"nonce":              "challenge_1",
		"signature":          "0xsig",
		"device_fingerprint": "device",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ut")
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fakeUC.lastLoginInput.UserAgent != "ut" {
		t.Fatalf("expected user agent fallback")
	}
}

func TestLoginHandler_MapsDomainErrors(t *testing.T) {
	fakeUC := &fakeAuthUseCase{loginErr: errorsx.ErrUnauthorized}
	engine := NewEngine(nil, NewAuthHandler(fakeUC), nil, nil, nil, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"address":   "0x0000000000000000000000000000000000000001",
		"chain_id":  8453,
		"nonce":     "challenge_1",
		"signature": "0xsig",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
