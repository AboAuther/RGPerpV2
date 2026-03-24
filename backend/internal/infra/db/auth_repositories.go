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
	result := DB(ctx, r.db).Model(&LoginNonceModel{}).
		Where("nonce = ? AND used_at IS NULL", nonceID).
		Update("used_at", gorm.Expr("CURRENT_TIMESTAMP"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var existing LoginNonceModel
	err := DB(ctx, r.db).Where("nonce = ?", nonceID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorsx.ErrNotFound
	}
	if err != nil {
		return err
	}
	return errorsx.ErrConflict
}

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByAddress(ctx context.Context, address string) (authdomain.User, error) {
	return r.getByScope(DB(ctx, r.db).Where("evm_address = ?", address))
}

func (r *UserRepository) GetByID(ctx context.Context, userID uint64) (authdomain.User, error) {
	return r.getByScope(DB(ctx, r.db).Where("id = ?", userID))
}

func (r *UserRepository) getByScope(tx *gorm.DB) (authdomain.User, error) {
	var model UserModel
	err := tx.First(&model).Error
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

func (r *UserRepository) ResolveUserIDByAddress(ctx context.Context, address string) (uint64, error) {
	user, err := r.GetByAddress(ctx, address)
	if err != nil {
		return 0, err
	}
	return user.ID, nil
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
		AccessExpiresAt:   session.AccessExpiresAt,
		RefreshExpiresAt:  session.RefreshExpiresAt,
		CreatedAt:         session.CreatedAt,
	}).Error
}

func (r *SessionRepository) GetActiveByAccessJTI(ctx context.Context, accessJTI string) (authdomain.Session, error) {
	var model SessionModel
	err := DB(ctx, r.db).
		Where("access_jti = ? AND revoked_at IS NULL", accessJTI).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return authdomain.Session{}, errorsx.ErrNotFound
		}
		return authdomain.Session{}, err
	}
	return authdomain.Session{
		UserID:            model.UserID,
		AccessJTI:         model.AccessJTI,
		RefreshJTI:        model.RefreshJTI,
		DeviceFingerprint: model.DeviceFingerprint,
		IP:                model.IP,
		UserAgent:         model.UserAgent,
		AccessExpiresAt:   model.AccessExpiresAt,
		RefreshExpiresAt:  model.RefreshExpiresAt,
		CreatedAt:         model.CreatedAt,
	}, nil
}

func (r *SessionRepository) GetActiveByRefreshJTI(ctx context.Context, refreshJTI string) (authdomain.Session, error) {
	var model SessionModel
	err := DB(ctx, r.db).
		Where("refresh_jti = ? AND revoked_at IS NULL", refreshJTI).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return authdomain.Session{}, errorsx.ErrNotFound
		}
		return authdomain.Session{}, err
	}
	return authdomain.Session{
		UserID:            model.UserID,
		AccessJTI:         model.AccessJTI,
		RefreshJTI:        model.RefreshJTI,
		DeviceFingerprint: model.DeviceFingerprint,
		IP:                model.IP,
		UserAgent:         model.UserAgent,
		AccessExpiresAt:   model.AccessExpiresAt,
		RefreshExpiresAt:  model.RefreshExpiresAt,
		CreatedAt:         model.CreatedAt,
	}, nil
}

func (r *SessionRepository) Rotate(ctx context.Context, previousRefreshJTI string, session authdomain.Session) error {
	updates := map[string]any{
		"access_jti":         session.AccessJTI,
		"refresh_jti":        session.RefreshJTI,
		"access_expires_at":  session.AccessExpiresAt,
		"refresh_expires_at": session.RefreshExpiresAt,
		"device_fingerprint": session.DeviceFingerprint,
		"ip":                 session.IP,
		"user_agent":         session.UserAgent,
	}
	result := DB(ctx, r.db).
		Model(&SessionModel{}).
		Where("refresh_jti = ? AND revoked_at IS NULL", previousRefreshJTI).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrNotFound
	}
	return nil
}

func (r *SessionRepository) RevokeByAccessJTI(ctx context.Context, accessJTI string) error {
	result := DB(ctx, r.db).
		Model(&SessionModel{}).
		Where("access_jti = ? AND revoked_at IS NULL", accessJTI).
		Update("revoked_at", gorm.Expr("CURRENT_TIMESTAMP"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrNotFound
	}
	return nil
}
