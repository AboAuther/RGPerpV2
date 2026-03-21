package authinfra

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
)

func TestJWTIssuer_IssueAccessToken(t *testing.T) {
	issuer := NewJWTIssuer("access-secret", "refresh-secret")
	tokenString, err := issuer.IssueAccessToken(context.Background(), authdomain.User{
		ID:         42,
		EVMAddress: "0x0000000000000000000000000000000000000001",
	}, authdomain.Session{
		ID:         "session_1",
		AccessJTI:  "access_1",
		RefreshJTI: "refresh_1",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte("access-secret"), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("expected valid token, got err=%v", err)
	}
}
