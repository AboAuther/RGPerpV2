package hedge

import (
	"context"
	"time"
)

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID(prefix string) string
}

type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type Repository interface {
	GetIntentForUpdate(ctx context.Context, intentID string) (Intent, error)
	GetLatestOpenIntentForSymbolForUpdate(ctx context.Context, symbolID uint64) (Intent, error)
	GetLatestOrderByIntentForUpdate(ctx context.Context, intentID string) (Order, error)
	CreateOrder(ctx context.Context, order Order) error
	UpdateOrder(ctx context.Context, order Order) error
	UpdateIntent(ctx context.Context, intent Intent) error
	CreateFill(ctx context.Context, fill Fill) error
	GetPositionForUpdate(ctx context.Context, symbol string, side string) (Position, error)
	UpsertPosition(ctx context.Context, position Position) error
}

type VenueAdapter interface {
	PlaceOrder(ctx context.Context, req ExecutionRequest) (ExecutionResult, error)
}

type OutboxPublisher interface {
	Publish(ctx context.Context, event DomainEvent) error
}

type DomainEvent struct {
	EventID       string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       any
	CreatedAt     time.Time
}
