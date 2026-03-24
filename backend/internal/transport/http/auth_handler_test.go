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
	challengeResp    authdomain.IssueChallengeOutput
	challengeErr     error
	loginResp        authdomain.LoginResult
	loginErr         error
	refreshResp      authdomain.LoginResult
	refreshErr       error
	logoutErr        error
	lastChallengeReq authdomain.IssueChallengeInput
	lastLoginInput   authdomain.LoginInput
	lastRefreshInput authdomain.RefreshInput
	lastLogoutInput  authdomain.LogoutInput
}

func (f *fakeAuthUseCase) IssueChallenge(_ context.Context, input authdomain.IssueChallengeInput) (authdomain.IssueChallengeOutput, error) {
	f.lastChallengeReq = input
	return f.challengeResp, f.challengeErr
}

func (f *fakeAuthUseCase) Login(_ context.Context, input authdomain.LoginInput) (authdomain.LoginResult, error) {
	f.lastLoginInput = input
	return f.loginResp, f.loginErr
}

func (f *fakeAuthUseCase) Refresh(_ context.Context, input authdomain.RefreshInput) (authdomain.LoginResult, error) {
	f.lastRefreshInput = input
	return f.refreshResp, f.refreshErr
}

func (f *fakeAuthUseCase) Logout(_ context.Context, input authdomain.LogoutInput) error {
	f.lastLogoutInput = input
	return f.logoutErr
}

func TestChallengeHandler_Success(t *testing.T) {
	fakeUC := &fakeAuthUseCase{
		challengeResp: authdomain.IssueChallengeOutput{
			Nonce:     "challenge_1",
			Message:   "RGPerp Login",
			Domain:    "localhost",
			ChainID:   8453,
			ExpiresAt: time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
		},
	}
	engine := NewEngine(nil, nil, NewAuthHandler(fakeUC, nil, nil), nil, nil, nil, nil, nil, nil)

	body, _ := json.Marshal(map[string]any{
		"address":  "0x0000000000000000000000000000000000000001",
		"chain_id": 8453,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/challenge", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fakeUC.lastChallengeReq.ChainID != 8453 {
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
	engine := NewEngine(nil, nil, NewAuthHandler(fakeUC, nil, nil), nil, nil, nil, nil, nil, nil)

	body, _ := json.Marshal(map[string]any{
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
	engine := NewEngine(nil, nil, NewAuthHandler(fakeUC, nil, nil), nil, nil, nil, nil, nil, nil)

	body, _ := json.Marshal(map[string]any{
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

func TestRefreshHandler_VerifiesRefreshTokenAndCallsUseCase(t *testing.T) {
	fakeUC := &fakeAuthUseCase{
		refreshResp: authdomain.LoginResult{
			User: authdomain.User{
				ID:         42,
				EVMAddress: "0x0000000000000000000000000000000000000001",
				Status:     "ACTIVE",
			},
			AccessToken:  "access_next",
			RefreshToken: "refresh_next",
			ExpiresAt:    time.Date(2026, 3, 21, 13, 0, 0, 0, time.UTC),
		},
	}
	engine := NewEngine(
		nil,
		nil,
		NewAuthHandler(fakeUC, fakeAccessVerifier{claims: AccessClaims{UserID: "42", Address: "0x0000000000000000000000000000000000000001", SessionID: "session_1", JTI: "refresh_1"}}, nil),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	body, _ := json.Marshal(map[string]any{"refresh_token": "refresh-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fakeUC.lastRefreshInput.RefreshJTI != "refresh_1" || fakeUC.lastRefreshInput.UserID != 42 {
		t.Fatalf("unexpected refresh input: %+v", fakeUC.lastRefreshInput)
	}
}

func TestLogoutHandler_UsesAccessJTI(t *testing.T) {
	fakeUC := &fakeAuthUseCase{}
	engine := NewEngine(
		nil,
		nil,
		NewAuthHandler(fakeUC, fakeAccessVerifier{claims: AccessClaims{UserID: "42", Address: "0xabc", JTI: "access_1"}}, nil),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fakeUC.lastLogoutInput.AccessJTI != "access_1" {
		t.Fatalf("unexpected logout input: %+v", fakeUC.lastLogoutInput)
	}
}
