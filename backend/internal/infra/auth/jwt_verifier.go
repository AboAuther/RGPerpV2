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
	Type      string
}

type JWTVerifier struct {
	accessSecret []byte
}

func NewJWTVerifier(accessSecret string) *JWTVerifier {
	return &JWTVerifier{accessSecret: []byte(accessSecret)}
}

func (v *JWTVerifier) VerifyAccessToken(token string) (AccessClaims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AccessClaims{}, fmt.Errorf("%w: missing bearer token", errorsx.ErrUnauthorized)
	}

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method", errorsx.ErrUnauthorized)
		}
		return v.accessSecret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return AccessClaims{}, fmt.Errorf("%w: access token expired", errorsx.ErrExpired)
		}
		return AccessClaims{}, fmt.Errorf("%w: invalid access token", errorsx.ErrUnauthorized)
	}
	if !parsed.Valid {
		return AccessClaims{}, fmt.Errorf("%w: invalid access token", errorsx.ErrUnauthorized)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return AccessClaims{}, fmt.Errorf("%w: invalid access token claims", errorsx.ErrUnauthorized)
	}
	if fmt.Sprint(claims["type"]) != "access" {
		return AccessClaims{}, fmt.Errorf("%w: token type mismatch", errorsx.ErrUnauthorized)
	}
	return AccessClaims{
		UserID:    fmt.Sprint(claims["sub"]),
		Address:   fmt.Sprint(claims["address"]),
		SessionID: fmt.Sprint(claims["session_id"]),
		Type:      fmt.Sprint(claims["type"]),
	}, nil
}
