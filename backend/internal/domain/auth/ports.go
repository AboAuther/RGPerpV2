package auth

import "context"

type NonceRepository interface {
	Create(ctx context.Context, nonce Nonce) error
	GetByValue(ctx context.Context, nonceValue string) (Nonce, error)
	MarkUsed(ctx context.Context, nonceID string) error
}

type UserRepository interface {
	GetByAddress(ctx context.Context, address string) (User, error)
	GetByID(ctx context.Context, userID uint64) (User, error)
	Create(ctx context.Context, user User) (User, error)
}

type SessionRepository interface {
	Create(ctx context.Context, session Session) error
	GetActiveByAccessJTI(ctx context.Context, accessJTI string) (Session, error)
	GetActiveByRefreshJTI(ctx context.Context, refreshJTI string) (Session, error)
	Rotate(ctx context.Context, previousRefreshJTI string, session Session) error
	RevokeByAccessJTI(ctx context.Context, accessJTI string) error
}

type SignatureVerifier interface {
	VerifyLogin(ctx context.Context, address string, chainID int64, domain string, nonce string, signature string) error
}

type TokenIssuer interface {
	IssueAccessToken(ctx context.Context, user User, session Session) (string, error)
	IssueRefreshToken(ctx context.Context, user User, session Session) (string, error)
}

type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type UserBootstrapper interface {
	EnsureUserBootstrap(ctx context.Context, user User) error
}
