package liquidation

import (
	"context"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
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
	GetLiquidationByIDForUpdate(ctx context.Context, liquidationID string) (Liquidation, error)
	CreateLiquidation(ctx context.Context, liquidation Liquidation) error
	UpdateLiquidation(ctx context.Context, liquidation Liquidation) error
	CreateItem(ctx context.Context, item Item) error
	LockSymbolForUpdate(ctx context.Context, symbolID uint64) error
	ListOpenPositionsForUpdate(ctx context.Context, userID uint64) ([]Position, error)
	GetPositionForLiquidationByID(ctx context.Context, userID uint64, positionID string) (Position, error)
	ListRiskIncreaseOrdersForUpdate(ctx context.Context, userID uint64) ([]RiskIncreaseOrder, error)
	CancelOrders(ctx context.Context, orderIDs []string, updatedAt time.Time) error
	CreateOrder(ctx context.Context, order OrderRecord) error
	CreateFill(ctx context.Context, fill FillRecord) error
	UpsertPosition(ctx context.Context, position Position) error
	GetCoverageBalancesForUpdate(ctx context.Context, userID uint64, asset string) (CoverageBalances, error)
}

type AccountResolver interface {
	ResolveLiquidationAccounts(ctx context.Context, userID uint64, asset string) (Accounts, error)
}

type LedgerPoster interface {
	Post(ctx context.Context, req ledgerdomain.PostingRequest) error
}

type RuntimeConfigProvider interface {
	CurrentLiquidationRuntimeConfig(symbol string) ServiceConfig
}

type RiskPreviewer interface {
	PreviewAccountRisk(ctx context.Context, userID uint64) (riskdomain.Snapshot, error)
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
