package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
)

type OutboxRepository struct {
	db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) ListPending(ctx context.Context, limit int) ([]OutboxEventModel, error) {
	if limit <= 0 {
		limit = 100
	}
	var events []OutboxEventModel
	if err := DB(ctx, r.db).
		Where("status = ?", "PENDING").
		Order("created_at ASC").
		Limit(limit).
		Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	result := DB(ctx, r.db).Model(&OutboxEventModel{}).
		Where("event_id = ?", eventID).
		Updates(map[string]any{
			"status":       "PUBLISHED",
			"published_at": publishedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errorsx.ErrNotFound
	}
	return nil
}

type MessageConsumptionRepository struct {
	db *gorm.DB
}

func NewMessageConsumptionRepository(db *gorm.DB) *MessageConsumptionRepository {
	return &MessageConsumptionRepository{db: db}
}

func (r *MessageConsumptionRepository) TryBegin(ctx context.Context, consumerName string, eventID string, consumedAt time.Time) (bool, error) {
	err := DB(ctx, r.db).Create(&MessageConsumptionModel{
		ConsumerName: consumerName,
		EventID:      eventID,
		ConsumedAt:   consumedAt,
	}).Error
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return false, nil
	}
	if isUniqueConstraintError(err) {
		return false, nil
	}
	return false, err
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return containsAny(text, "UNIQUE constraint failed", "Duplicate entry", "duplicated key")
}

func containsAny(text string, items ...string) bool {
	for _, item := range items {
		if item != "" && strings.Contains(text, item) {
			return true
		}
	}
	return false
}
