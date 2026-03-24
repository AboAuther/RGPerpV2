package db

import (
	"context"
	"time"

	fundingdomain "github.com/xiaobao/rgperp/backend/internal/domain/funding"
	"github.com/xiaobao/rgperp/backend/internal/pkg/idgen"
	"gorm.io/gorm"
)

type FundingOutboxPublisher struct {
	outbox *OutboxRepository
	idgen  idgen.Generator
}

func NewFundingOutboxPublisher(db *gorm.DB) *FundingOutboxPublisher {
	return &FundingOutboxPublisher{
		outbox: NewOutboxRepository(db),
		idgen:  &idgen.TimeBasedGenerator{},
	}
}

func (p *FundingOutboxPublisher) PublishBatchApplied(ctx context.Context, event fundingdomain.BatchAppliedEvent) error {
	return p.outbox.Create(ctx, OutboxMessage{
		EventID:       p.idgen.NewID("evt"),
		AggregateType: "funding_batch",
		AggregateID:   event.FundingBatchID,
		EventType:     "risk.funding.batch.applied",
		Payload:       event,
		CreatedAt:     time.Now().UTC(),
	})
}

func (p *FundingOutboxPublisher) PublishBatchReversed(ctx context.Context, event fundingdomain.BatchReversedEvent) error {
	return p.outbox.Create(ctx, OutboxMessage{
		EventID:       p.idgen.NewID("evt"),
		AggregateType: "funding_batch",
		AggregateID:   event.FundingBatchID,
		EventType:     "funding.batch.reversed",
		Payload:       event,
		CreatedAt:     time.Now().UTC(),
	})
}
