package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestAuthRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	nonceRepo := NewNonceRepository(db)
	userRepo := NewUserRepository(db)
	sessionRepo := NewSessionRepository(db)
	ctx := context.Background()

	now := time.Now().UTC()
	err := nonceRepo.Create(ctx, authdomain.Nonce{
		Address:   "0x0000000000000000000000000000000000000001",
		ChainID:   8453,
		Domain:    "localhost",
		Value:     "challenge_1",
		ExpiresAt: now.Add(time.Minute),
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create nonce: %v", err)
	}

	nonce, err := nonceRepo.GetByValue(ctx, "challenge_1")
	if err != nil {
		t.Fatalf("get nonce: %v", err)
	}
	if nonce.Value != "challenge_1" {
		t.Fatalf("unexpected nonce")
	}

	user, err := userRepo.Create(ctx, authdomain.User{
		EVMAddress: "0x0000000000000000000000000000000000000001",
		Status:     "ACTIVE",
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.ID == 0 {
		t.Fatalf("expected user id")
	}

	gotUser, err := userRepo.GetByAddress(ctx, user.EVMAddress)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if gotUser.ID != user.ID {
		t.Fatalf("unexpected user id")
	}

	if err := sessionRepo.Create(ctx, authdomain.Session{
		UserID:            user.ID,
		AccessJTI:         "access_1",
		RefreshJTI:        "refresh_1",
		DeviceFingerprint: "device",
		IP:                "127.0.0.1",
		UserAgent:         "ua",
		AccessExpiresAt:   now.Add(time.Hour),
		RefreshExpiresAt:  now.Add(24 * time.Hour),
		CreatedAt:         now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

func TestNonceRepository_MarkUsed_IsSingleUse(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNonceRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := repo.Create(ctx, authdomain.Nonce{
		Address:   "0x0000000000000000000000000000000000000001",
		ChainID:   8453,
		Domain:    "localhost",
		Value:     "challenge_1",
		ExpiresAt: now.Add(time.Minute),
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("create nonce: %v", err)
	}

	if err := repo.MarkUsed(ctx, "challenge_1"); err != nil {
		t.Fatalf("first mark used: %v", err)
	}
	err := repo.MarkUsed(ctx, "challenge_1")
	if !errors.Is(err, errorsx.ErrConflict) {
		t.Fatalf("expected conflict on second mark, got %v", err)
	}
}
