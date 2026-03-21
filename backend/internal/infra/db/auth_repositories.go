package db

import (
	"context"
	"errors"

	authdomain "github.com/xiaobao/rgperp/backend/internal/domain/auth"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
)

type NonceRepository struct {
	db *gorm.DB
}

func NewNonceRepository(db *gorm.DB) *NonceRepository {
	return &NonceRepository{db: db}
}

func (r *NonceRepository) Create(ctx context.Context, nonce authdomain.Nonce) error {
	return DB(ctx, r.db).Create(&LoginNonceModel{
		EVMAddress: nonce.Address,
		Nonce:      nonce.Value,
		ChainID:    nonce.ChainID,
		Domain:     nonce.Domain,
		ExpiresAt:  nonce.ExpiresAt,
		UsedAt:     nonce.UsedAt,
		CreatedAt:  nonce.CreatedAt,
	}).Error
}

func (r *NonceRepository) GetByValue(ctx context.Context, nonceValue string) (authdomain.Nonce, error) {
	var model LoginNonceModel
	err := DB(ctx, r.db).Where("nonce = ?", nonceValue).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return authdomain.Nonce{}, errorsx.ErrNotFound
		}
		return authdomain.Nonce{}, err
	}
	return authdomain.Nonce{
		ID:        model.Nonce,
		Address:   model.EVMAddress,
		ChainID:   model.ChainID,
		Domain:    model.Domain,
		Value:     model.Nonce,
		ExpiresAt: model.ExpiresAt,
		UsedAt:    model.UsedAt,
		CreatedAt: model.CreatedAt,
	}, nil
}

func (r *NonceRepository) MarkUsed(ctx context.Context, nonceID string) error {
	return DB(ctx, r.db).Model(&LoginNonceModel{}).
		Where("nonce = ? AND used_at IS NULL", nonceID).
		Update("used_at", gorm.Expr("CURRENT_TIMESTAMP")).
		Error
}

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByAddress(ctx context.Context, address string) (authdomain.User, error) {
	var model UserModel
	err := DB(ctx, r.db).Where("evm_address = ?", address).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return authdomain.User{}, errorsx.ErrNotFound
		}
		return authdomain.User{}, err
	}
	return authdomain.User{
		ID:         model.ID,
		EVMAddress: model.EVMAddress,
		Status:     model.Status,
		CreatedAt:  model.CreatedAt,
		UpdatedAt:  model.UpdatedAt,
	}, nil
}

func (r *UserRepository) Create(ctx context.Context, user authdomain.User) (authdomain.User, error) {
	model := UserModel{
		EVMAddress: user.EVMAddress,
		Status:     user.Status,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
	}
	if err := DB(ctx, r.db).Create(&model).Error; err != nil {
		return authdomain.User{}, err
	}
	user.ID = model.ID
	return user, nil
}

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, session authdomain.Session) error {
	return DB(ctx, r.db).Create(&SessionModel{
		UserID:            session.UserID,
		AccessJTI:         session.AccessJTI,
		RefreshJTI:        session.RefreshJTI,
		DeviceFingerprint: session.DeviceFingerprint,
		IP:                session.IP,
		UserAgent:         session.UserAgent,
		ExpiresAt:         session.ExpiresAt,
		CreatedAt:         session.CreatedAt,
	}).Error
}
