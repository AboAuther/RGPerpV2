package authinfra

import (
	"context"
	"time"

	"github.com/golang-jwt/jwt/v5"
	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
)

type JWTIssuer struct {
	accessSecret  []byte
	refreshSecret []byte
}

func NewJWTIssuer(accessSecret string, refreshSecret string) *JWTIssuer {
	return &JWTIssuer{
		accessSecret:  []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
	}
}

func (j *JWTIssuer) IssueAccessToken(_ context.Context, user authdomain.User, session authdomain.Session) (string, error) {
	return j.sign(user, session, j.accessSecret, "access")
}

func (j *JWTIssuer) IssueRefreshToken(_ context.Context, user authdomain.User, session authdomain.Session) (string, error) {
	return j.sign(user, session, j.refreshSecret, "refresh")
}

func (j *JWTIssuer) sign(user authdomain.User, session authdomain.Session, secret []byte, tokenType string) (string, error) {
	claims := jwt.MapClaims{
		"sub":        user.ID,
		"address":    user.EVMAddress,
		"session_id": session.ID,
		"jti":        map[string]string{"access": session.AccessJTI, "refresh": session.RefreshJTI}[tokenType],
		"type":       tokenType,
		"exp":        session.ExpiresAt.Unix(),
		"iat":        time.Now().UTC().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
