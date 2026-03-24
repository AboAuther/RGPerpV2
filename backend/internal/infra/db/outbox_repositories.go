package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OutboxRepository struct {
	db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

type OutboxMessage struct {
	EventID       string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       any
	Status        string
	CreatedAt     time.Time
}

func (r *OutboxRepository) Create(ctx context.Context, msg OutboxMessage) error {
	payload, err := json.Marshal(msg.Payload)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}
	status := msg.Status
	if status == "" {
		status = "PENDING"
	}
	createdAt := msg.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return DB(ctx, r.db).Create(&OutboxEventModel{
		EventID:       msg.EventID,
		AggregateType: msg.AggregateType,
		AggregateID:   msg.AggregateID,
		EventType:     msg.EventType,
		PayloadJSON:   string(payload),
		Status:        status,
		CreatedAt:     createdAt,
	}).Error
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

func (r *OutboxRepository) ListPendingByEventType(ctx context.Context, eventType string, limit int) ([]OutboxEventModel, error) {
	if limit <= 0 {
		limit = 100
	}
	var events []OutboxEventModel
	if err := DB(ctx, r.db).
		Where("status = ? AND event_type = ?", "PENDING", eventType).
		Order("created_at ASC").
		Limit(limit).
		Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (r *OutboxRepository) ListPendingByEventTypeForConsumer(ctx context.Context, eventType string, consumerName string, limit int) ([]OutboxEventModel, error) {
	if limit <= 0 {
		limit = 100
	}
	var events []OutboxEventModel
	if err := DB(ctx, r.db).
		Table("outbox_events").
		Select("outbox_events.*").
		Joins("LEFT JOIN message_consumptions ON message_consumptions.event_id = outbox_events.event_id AND message_consumptions.consumer_name = ?", consumerName).
		Where("outbox_events.status = ? AND outbox_events.event_type = ? AND message_consumptions.event_id IS NULL", "PENDING", eventType).
		Order("outbox_events.created_at ASC").
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
	result := DB(ctx, r.db).Clauses(clause.OnConflict{DoNothing: true}).Create(&MessageConsumptionModel{
		ConsumerName: consumerName,
		EventID:      eventID,
		ConsumedAt:   consumedAt,
	})
	if result.Error != nil {
		err := result.Error
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return false, nil
		}
		if isUniqueConstraintError(err) {
			return false, nil
		}
		return false, err
	}
	return result.RowsAffected > 0, nil
}

func (r *MessageConsumptionRepository) Delete(ctx context.Context, consumerName string, eventID string) error {
	return DB(ctx, r.db).
		Where("consumer_name = ? AND event_id = ?", consumerName, eventID).
		Delete(&MessageConsumptionModel{}).Error
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

type ChainCursorRepository struct {
	db *gorm.DB
}

func NewChainCursorRepository(db *gorm.DB) *ChainCursorRepository {
	return &ChainCursorRepository{db: db}
}

func (r *ChainCursorRepository) Get(ctx context.Context, chainID int64, cursorType string) (ChainCursorModel, error) {
	var cursor ChainCursorModel
	err := DB(ctx, r.db).
		Where("chain_id = ? AND cursor_type = ?", chainID, cursorType).
		First(&cursor).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ChainCursorModel{}, errorsx.ErrNotFound
		}
		return ChainCursorModel{}, err
	}
	return cursor, nil
}

func (r *ChainCursorRepository) Upsert(ctx context.Context, chainID int64, cursorType string, cursorValue string, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	return DB(ctx, r.db).
		Where("chain_id = ? AND cursor_type = ?", chainID, cursorType).
		Assign(map[string]any{
			"cursor_value": cursorValue,
			"updated_at":   updatedAt,
		}).
		FirstOrCreate(&ChainCursorModel{
			ChainID:     chainID,
			CursorType:  cursorType,
			CursorValue: cursorValue,
			UpdatedAt:   updatedAt,
		}).Error
}
