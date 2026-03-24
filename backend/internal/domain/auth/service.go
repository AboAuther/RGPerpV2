package auth

import (
	"context"
	"errors"
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

func BuildLoginMessage(domain string, chainID int64, nonce string) string {
	return fmt.Sprintf("RGPerp Login\nDomain: %s\nChain ID: %d\nNonce: %s", domain, chainID, nonce)
}

func (s *Service) IssueChallenge(ctx context.Context, input IssueChallengeInput) (IssueChallengeOutput, error) {
	address, err := authx.NormalizeEVMAddress(input.Address)
	if err != nil {
		return IssueChallengeOutput{}, err
	}
	if input.ChainID <= 0 {
		return IssueChallengeOutput{}, fmt.Errorf("%w: chain id must be positive", errorsx.ErrInvalidArgument)
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
		return IssueChallengeOutput{}, err
	}

	return IssueChallengeOutput{
		Nonce:     nonce.Value,
		Message:   BuildLoginMessage(nonce.Domain, nonce.ChainID, nonce.Value),
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

func (s *Service) Refresh(ctx context.Context, input RefreshInput) (LoginResult, error) {
	if input.UserID == 0 || input.Address == "" || input.RefreshJTI == "" {
		return LoginResult{}, fmt.Errorf("%w: incomplete refresh request", errorsx.ErrInvalidArgument)
	}

	now := s.clock.Now()
	var (
		user        User
		nextSession Session
	)
	if err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.sessions.GetActiveByRefreshJTI(txCtx, input.RefreshJTI)
		if err != nil {
			return err
		}
		if current.UserID != input.UserID {
			return fmt.Errorf("%w: refresh token binding mismatch", errorsx.ErrUnauthorized)
		}
		if !now.Before(current.RefreshExpiresAt) {
			return fmt.Errorf("%w: refresh token expired", errorsx.ErrExpired)
		}

		user, err = s.users.GetByID(txCtx, current.UserID)
		if err != nil {
			return err
		}
		if user.EVMAddress != input.Address {
			return fmt.Errorf("%w: refresh token address mismatch", errorsx.ErrUnauthorized)
		}

		nextSession = current
		nextSession.ID = input.SessionID
		nextSession.AccessJTI = s.idgen.NewID("access")
		nextSession.RefreshJTI = s.idgen.NewID("refresh")
		nextSession.AccessExpiresAt = now.Add(s.cfg.AccessTTL)
		nextSession.RefreshExpiresAt = now.Add(s.cfg.RefreshTTL)
		return s.sessions.Rotate(txCtx, input.RefreshJTI, nextSession)
	}); err != nil {
		return LoginResult{}, err
	}

	accessToken, err := s.tokens.IssueAccessToken(ctx, user, nextSession)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := s.tokens.IssueRefreshToken(ctx, user, nextSession)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    nextSession.AccessExpiresAt,
	}, nil
}

func (s *Service) Logout(ctx context.Context, input LogoutInput) error {
	if input.AccessJTI == "" {
		return fmt.Errorf("%w: missing access jti", errorsx.ErrInvalidArgument)
	}
	err := s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		return s.sessions.RevokeByAccessJTI(txCtx, input.AccessJTI)
	})
	if errors.Is(err, errorsx.ErrNotFound) {
		return nil
	}
	return err
}
