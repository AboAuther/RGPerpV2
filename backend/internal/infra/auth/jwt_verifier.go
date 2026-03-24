package authinfra

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type AccessClaims struct {
	UserID    string
	Address   string
	SessionID string
	JTI       string
	Type      string
}

type JWTVerifier struct {
	accessSecret  []byte
	refreshSecret []byte
}

func NewJWTVerifier(accessSecret string, refreshSecret string) *JWTVerifier {
	return &JWTVerifier{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
	}
}

func (v *JWTVerifier) VerifyAccessToken(token string) (AccessClaims, error) {
	return v.verifyToken(token, v.accessSecret, "access")
}

func (v *JWTVerifier) VerifyRefreshToken(token string) (AccessClaims, error) {
	return v.verifyToken(token, v.refreshSecret, "refresh")
}

func (v *JWTVerifier) verifyToken(token string, secret []byte, expectedType string) (AccessClaims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AccessClaims{}, fmt.Errorf("%w: missing bearer token", errorsx.ErrUnauthorized)
	}

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method", errorsx.ErrUnauthorized)
		}
		return secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return AccessClaims{}, fmt.Errorf("%w: %s token expired", errorsx.ErrExpired, expectedType)
		}
		return AccessClaims{}, fmt.Errorf("%w: invalid %s token", errorsx.ErrUnauthorized, expectedType)
	}
	if !parsed.Valid {
		return AccessClaims{}, fmt.Errorf("%w: invalid %s token", errorsx.ErrUnauthorized, expectedType)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return AccessClaims{}, fmt.Errorf("%w: invalid %s token claims", errorsx.ErrUnauthorized, expectedType)
	}
	if fmt.Sprint(claims["type"]) != expectedType {
		return AccessClaims{}, fmt.Errorf("%w: token type mismatch", errorsx.ErrUnauthorized)
	}
	return AccessClaims{
		UserID:    fmt.Sprint(claims["sub"]),
		Address:   fmt.Sprint(claims["address"]),
		SessionID: fmt.Sprint(claims["session_id"]),
		JTI:       fmt.Sprint(claims["jti"]),
		Type:      fmt.Sprint(claims["type"]),
	}, nil
}
