package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/authx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/clockx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
)

type ServiceConfig struct {
	Domain           string
	NonceTTL         time.Duration
	AccessTTL        time.Duration
	RefreshTTL       time.Duration
	DefaultUserState string
}

type Service struct {
	cfg       ServiceConfig
	clock     clockx.Clock
	idgen     idgen.Generator
	nonces    NonceRepository
	users     UserRepository
	sessions  SessionRepository
	verifier  SignatureVerifier
	tokens    TokenIssuer
	txManager TxManager
	bootstrap UserBootstrapper
}

func NewService(
	cfg ServiceConfig,
	clock clockx.Clock,
	idGenerator idgen.Generator,
	nonceRepo NonceRepository,
	userRepo UserRepository,
	sessionRepo SessionRepository,
	verifier SignatureVerifier,
	tokenIssuer TokenIssuer,
	txManager TxManager,
	bootstrap UserBootstrapper,
) *Service {
	return &Service{
		cfg:       cfg,
		clock:     clock,
		idgen:     idGenerator,
		nonces:    nonceRepo,
		users:     userRepo,
		sessions:  sessionRepo,
		verifier:  verifier,
		tokens:    tokenIssuer,
		txManager: txManager,
		bootstrap: bootstrap,
	}
}

func (s *Service) IssueNonce(ctx context.Context, input IssueNonceInput) (IssueNonceOutput, error) {
	address, err := authx.NormalizeEVMAddress(input.Address)
	if err != nil {
		return IssueNonceOutput{}, err
	}
	if input.ChainID <= 0 {
		return IssueNonceOutput{}, fmt.Errorf("%w: chain id must be positive", errorsx.ErrInvalidArgument)
	}

	now := s.clock.Now()
	nonce := Nonce{
		ID:        s.idgen.NewID("nonce"),
		Address:   address,
		ChainID:   input.ChainID,
		Domain:    s.cfg.Domain,
		Value:     s.idgen.NewID("challenge"),
		CreatedAt: now,
		ExpiresAt: now.Add(s.cfg.NonceTTL),
	}

	if err := s.nonces.Create(ctx, nonce); err != nil {
		return IssueNonceOutput{}, err
	}

	return IssueNonceOutput{
		Nonce:     nonce.Value,
		Domain:    nonce.Domain,
		ChainID:   nonce.ChainID,
		ExpiresAt: nonce.ExpiresAt,
	}, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	address, err := authx.NormalizeEVMAddress(input.Address)
	if err != nil {
		return LoginResult{}, err
	}
	if input.ChainID <= 0 || input.Nonce == "" || input.Signature == "" {
		return LoginResult{}, fmt.Errorf("%w: incomplete login request", errorsx.ErrInvalidArgument)
	}

	nonce, err := s.nonces.GetByValue(ctx, input.Nonce)
	if err != nil {
		return LoginResult{}, err
	}
	if nonce.Address != address || nonce.ChainID != input.ChainID || nonce.Domain != s.cfg.Domain {
		return LoginResult{}, fmt.Errorf("%w: nonce binding mismatch", errorsx.ErrUnauthorized)
	}

	now := s.clock.Now()
	if nonce.UsedAt != nil {
		return LoginResult{}, fmt.Errorf("%w: nonce already used", errorsx.ErrConflict)
	}
	if !now.Before(nonce.ExpiresAt) {
		return LoginResult{}, fmt.Errorf("%w: nonce expired", errorsx.ErrExpired)
	}
	if err := s.verifier.VerifyLogin(ctx, address, input.ChainID, nonce.Domain, nonce.Value, input.Signature); err != nil {
		return LoginResult{}, err
	}

	var user User
	session := Session{
		ID:                s.idgen.NewID("session"),
		AccessJTI:         s.idgen.NewID("access"),
		RefreshJTI:        s.idgen.NewID("refresh"),
		DeviceFingerprint: input.DeviceFingerprint,
		IP:                input.IP,
		UserAgent:         input.UserAgent,
		AccessExpiresAt:   now.Add(s.cfg.AccessTTL),
		RefreshExpiresAt:  now.Add(s.cfg.RefreshTTL),
		CreatedAt:         now,
	}

	if err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, getErr := s.users.GetByAddress(txCtx, address)
		switch {
		case getErr == nil:
			user = existing
		case getErr != nil && getErr == errorsx.ErrNotFound:
			createdUser, createErr := s.users.Create(txCtx, User{
				EVMAddress: address,
				Status:     s.cfg.DefaultUserState,
				CreatedAt:  now,
				UpdatedAt:  now,
			})
			if createErr != nil {
				return createErr
			}
			user = createdUser
		default:
			return getErr
		}

		if s.bootstrap != nil {
			if err := s.bootstrap.EnsureUserBootstrap(txCtx, user); err != nil {
				return err
			}
		}

		if err := s.nonces.MarkUsed(txCtx, nonce.ID); err != nil {
			return err
		}

		session.UserID = user.ID
		return s.sessions.Create(txCtx, session)
	}); err != nil {
		return LoginResult{}, err
	}

	accessToken, err := s.tokens.IssueAccessToken(ctx, user, session)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := s.tokens.IssueRefreshToken(ctx, user, session)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    session.AccessExpiresAt,
	}, nil
}
