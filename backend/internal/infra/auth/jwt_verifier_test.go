package authinfra

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTVerifier_VerifyAccessToken(t *testing.T) {
	secret := "access-secret"
	refreshSecret := "refresh-secret"
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":        "42",
		"address":    "0xabc",
		"session_id": "session_1",
		"type":       "access",
		"exp":        time.Now().UTC().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	claims, err := NewJWTVerifier(secret, refreshSecret).VerifyAccessToken(signed)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if claims.UserID != "42" || claims.Address != "0xabc" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}
