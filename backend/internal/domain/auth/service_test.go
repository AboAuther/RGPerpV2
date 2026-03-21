package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeIDGen struct {
	mu     sync.Mutex
	values []string
	idx    int
}

func (f *fakeIDGen) NewID(prefix string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	value := f.values[f.idx]
	f.idx++
	return value
}

type stubNonceRepo struct {
	created []Nonce
	nonce   Nonce
	markID  string
	err     error
}

func (s *stubNonceRepo) Create(_ context.Context, nonce Nonce) error {
	s.created = append(s.created, nonce)
	return s.err
}
func (s *stubNonceRepo) GetByValue(_ context.Context, _ string) (Nonce, error) {
	if s.err != nil {
		return Nonce{}, s.err
	}
	return s.nonce, nil
}
func (s *stubNonceRepo) MarkUsed(_ context.Context, nonceID string) error {
	s.markID = nonceID
	return s.err
}

type stubUserRepo struct {
	user        User
	getErr      error
	createdUser User
	createErr   error
}

func (s *stubUserRepo) GetByAddress(_ context.Context, _ string) (User, error) {
	return s.user, s.getErr
}
func (s *stubUserRepo) Create(_ context.Context, user User) (User, error) {
	if s.createErr != nil {
		return User{}, s.createErr
	}
	s.createdUser = user
	user.ID = 42
	return user, nil
}

type stubSessionRepo struct {
	session Session
	err     error
}

func (s *stubSessionRepo) Create(_ context.Context, session Session) error {
	s.session = session
	return s.err
}

type stubVerifier struct{ err error }

func (s stubVerifier) VerifyLogin(_ context.Context, _ string, _ int64, _ string, _ string, _ string) error {
	return s.err
}

type stubTokens struct {
	access  string
	refresh string
	err     error
}

func (s stubTokens) IssueAccessToken(_ context.Context, _ User, _ Session) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.access, nil
}
func (s stubTokens) IssueRefreshToken(_ context.Context, _ User, _ Session) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.refresh, nil
}

type stubTxManager struct{ err error }

func (s stubTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.err != nil {
		return s.err
	}
	return fn(ctx)
}

type stubBootstrapper struct {
	users []User
	err   error
}

func (s *stubBootstrapper) EnsureUserBootstrap(_ context.Context, user User) error {
	s.users = append(s.users, user)
	return s.err
}

func TestIssueChallenge_Success(t *testing.T) {
	clock := fakeClock{now: time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)}
	ids := &fakeIDGen{values: []string{"nonce_1", "challenge_1"}}
	nonces := &stubNonceRepo{}
	svc := NewService(
		ServiceConfig{Domain: "localhost", NonceTTL: 5 * time.Minute},
		clock,
		ids,
		nonces,
		&stubUserRepo{},
		&stubSessionRepo{},
		stubVerifier{},
		stubTokens{},
		stubTxManager{},
		nil,
	)

	got, err := svc.IssueChallenge(context.Background(), IssueChallengeInput{
		Address: "0x0000000000000000000000000000000000000001",
		ChainID: 8453,
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if got.Nonce != "challenge_1" {
		t.Fatalf("unexpected nonce: %s", got.Nonce)
	}
	if got.Message == "" {
		t.Fatalf("expected challenge message")
	}
	if len(nonces.created) != 1 {
		t.Fatalf("expected nonce create call")
	}
}

func TestLogin_CreatesUserAndSession(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	nonceUsedAt := (*time.Time)(nil)
	nonces := &stubNonceRepo{nonce: Nonce{
		ID:        "nonce_1",
		Address:   "0x0000000000000000000000000000000000000001",
		ChainID:   8453,
		Domain:    "localhost",
		Value:     "challenge_1",
		ExpiresAt: now.Add(time.Minute),
		UsedAt:    nonceUsedAt,
	}}
	users := &stubUserRepo{getErr: errorsx.ErrNotFound}
	sessions := &stubSessionRepo{}
	bootstrap := &stubBootstrapper{}
	svc := NewService(
		ServiceConfig{
			Domain:           "localhost",
			NonceTTL:         5 * time.Minute,
			AccessTTL:        time.Hour,
			RefreshTTL:       24 * time.Hour,
			DefaultUserState: "ACTIVE",
		},
		fakeClock{now: now},
		&fakeIDGen{values: []string{"session_1", "access_1", "refresh_1"}},
		nonces,
		users,
		sessions,
		stubVerifier{},
		stubTokens{access: "access-token", refresh: "refresh-token"},
		stubTxManager{},
		bootstrap,
	)

	got, err := svc.Login(context.Background(), LoginInput{
		Address:           "0x0000000000000000000000000000000000000001",
		ChainID:           8453,
		Nonce:             "challenge_1",
		Signature:         "0xsig",
		DeviceFingerprint: "device",
		IP:                "127.0.0.1",
		UserAgent:         "ua",
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if got.User.ID != 42 {
		t.Fatalf("expected created user ID")
	}
	if sessions.session.ID != "session_1" {
		t.Fatalf("expected session to be persisted")
	}
	if sessions.session.AccessExpiresAt.IsZero() || sessions.session.RefreshExpiresAt.IsZero() {
		t.Fatalf("expected token expiries to be set")
	}
	if nonces.markID != "nonce_1" {
		t.Fatalf("expected nonce marked used")
	}
	if len(bootstrap.users) != 1 || bootstrap.users[0].ID != 42 {
		t.Fatalf("expected bootstrap invoked for created user")
	}
}

func TestLogin_RejectsExpiredNonce(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	nonces := &stubNonceRepo{nonce: Nonce{
		ID:        "nonce_1",
		Address:   "0x0000000000000000000000000000000000000001",
		ChainID:   8453,
		Domain:    "localhost",
		Value:     "challenge_1",
		ExpiresAt: now.Add(-time.Second),
	}}

	svc := NewService(
		ServiceConfig{Domain: "localhost", AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour},
		fakeClock{now: now},
		&fakeIDGen{values: []string{"session_1", "access_1", "refresh_1"}},
		nonces,
		&stubUserRepo{},
		&stubSessionRepo{},
		stubVerifier{},
		stubTokens{access: "access-token", refresh: "refresh-token"},
		stubTxManager{},
		nil,
	)

	_, err := svc.Login(context.Background(), LoginInput{
		Address:   "0x0000000000000000000000000000000000000001",
		ChainID:   8453,
		Nonce:     "challenge_1",
		Signature: "0xsig",
	})
	if err == nil || !errors.Is(err, errorsx.ErrExpired) {
		t.Fatalf("expected expired error, got %v", err)
	}
}

type mutexTxManager struct {
	mu sync.Mutex
}

func (m *mutexTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(ctx)
}

func TestLogin_SameNonceConcurrentOnlyOneSucceeds(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	nonces := &stubNonceRepo{nonce: Nonce{
		ID:        "nonce_1",
		Address:   "0x0000000000000000000000000000000000000001",
		ChainID:   8453,
		Domain:    "localhost",
		Value:     "challenge_1",
		ExpiresAt: now.Add(time.Minute),
	}}
	users := &stubUserRepo{user: User{
		ID:         42,
		EVMAddress: "0x0000000000000000000000000000000000000001",
		Status:     "ACTIVE",
	}}
	txManager := &mutexTxManager{}

	repo := &concurrentNonceRepo{nonce: nonces.nonce}
	svc := NewService(
		ServiceConfig{
			Domain:           "localhost",
			NonceTTL:         5 * time.Minute,
			AccessTTL:        time.Hour,
			RefreshTTL:       24 * time.Hour,
			DefaultUserState: "ACTIVE",
		},
		fakeClock{now: now},
		&fakeIDGen{values: []string{"session_1", "access_1", "refresh_1", "session_2", "access_2", "refresh_2"}},
		repo,
		users,
		&stubSessionRepo{},
		stubVerifier{},
		stubTokens{access: "access-token", refresh: "refresh-token"},
		txManager,
		nil,
	)

	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := svc.Login(context.Background(), LoginInput{
				Address:           "0x0000000000000000000000000000000000000001",
				ChainID:           8453,
				Nonce:             "challenge_1",
				Signature:         "0xsig",
				DeviceFingerprint: "device",
				IP:                "127.0.0.1",
				UserAgent:         "ua",
			})
			results <- err
		}()
	}

	var successCount int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successCount++
			continue
		}
		if !errors.Is(err, errorsx.ErrConflict) {
			t.Fatalf("expected nonce conflict, got %v", err)
		}
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one successful login, got %d", successCount)
	}
}

type concurrentNonceRepo struct {
	mu    sync.Mutex
	nonce Nonce
}

func (r *concurrentNonceRepo) Create(_ context.Context, nonce Nonce) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nonce = nonce
	return nil
}

func (r *concurrentNonceRepo) GetByValue(_ context.Context, _ string) (Nonce, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nonce, nil
}

func (r *concurrentNonceRepo) MarkUsed(_ context.Context, nonceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.nonce.ID != nonceID {
		return errorsx.ErrNotFound
	}
	if r.nonce.UsedAt != nil {
		return errorsx.ErrConflict
	}
	now := time.Now().UTC()
	r.nonce.UsedAt = &now
	return nil
}
