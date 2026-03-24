package risk

import (
	"context"
	"time"

	hedgedomain "github.com/xiaobao/rgperp/backend/internal/domain/hedge"
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
	GetAccountStateForUpdate(ctx context.Context, userID uint64) (AccountState, error)
	GetLatestRiskSnapshot(ctx context.Context, userID uint64) (Snapshot, error)
	CreateRiskSnapshot(ctx context.Context, snapshot Snapshot) (Snapshot, error)
	MarkPositionLiquidating(ctx context.Context, positionID string, updatedAt time.Time) (bool, error)

	ListActiveSymbols(ctx context.Context) ([]HedgeState, error)
	GetHedgeStateForUpdate(ctx context.Context, symbolID uint64) (HedgeState, error)
	GetLatestOpenHedgeIntentForUpdate(ctx context.Context, symbolID uint64) (hedgedomain.Intent, error)
	SupersedePendingHedgeIntentsForUpdate(ctx context.Context, symbolID uint64, updatedAt time.Time) error
	CreateHedgeIntent(ctx context.Context, intent hedgedomain.Intent) error
}

type OutboxPublisher interface {
	Publish(ctx context.Context, event DomainEvent) error
}

type RuntimeConfigProvider interface {
	CurrentRiskRuntimeConfig() ServiceConfig
}

type DomainEvent struct {
	EventID       string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       any
	CreatedAt     time.Time
}
