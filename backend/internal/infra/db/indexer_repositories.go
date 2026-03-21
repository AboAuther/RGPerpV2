package db

import (
	"context"
	"time"

	indexerdomain "github.com/xiaobao/rgperp/backend/internal/domain/indexer"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
)

type IndexerEventPublisher struct {
	outbox *OutboxRepository
}

func NewIndexerEventPublisher(db *gorm.DB) *IndexerEventPublisher {
	return &IndexerEventPublisher{outbox: NewOutboxRepository(db)}
}

func (p *IndexerEventPublisher) Publish(ctx context.Context, envelope indexerdomain.EventEnvelope) error {
	return p.outbox.Create(ctx, OutboxMessage{
		EventID:       envelope.EventID,
		AggregateType: envelope.AggregateType,
		AggregateID:   envelope.AggregateID,
		EventType:     envelope.EventType,
		Payload:       envelope,
		Status:        "PENDING",
		CreatedAt:     envelope.OccurredAt,
	})
}

type IndexerCursorRepository struct {
	db *gorm.DB
}

func NewIndexerCursorRepository(db *gorm.DB) *IndexerCursorRepository {
	return &IndexerCursorRepository{db: db}
}

func (r *IndexerCursorRepository) Get(ctx context.Context, chainID int64, cursorType string) (indexerdomain.Cursor, error) {
	var model ChainCursorModel
	err := DB(ctx, r.db).
		Where("chain_id = ? AND cursor_type = ?", chainID, cursorType).
		First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return indexerdomain.Cursor{}, errorsx.ErrNotFound
		}
		return indexerdomain.Cursor{}, err
	}
	return indexerdomain.Cursor{
		ChainID:     model.ChainID,
		CursorType:  model.CursorType,
		CursorValue: model.CursorValue,
		UpdatedAt:   model.UpdatedAt,
	}, nil
}

func (r *IndexerCursorRepository) Upsert(ctx context.Context, chainID int64, cursorType string, cursorValue string, updatedAt time.Time) error {
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
