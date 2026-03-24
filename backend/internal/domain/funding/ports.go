package funding

import (
	"context"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
)

type Repository interface {
	ListSymbolsForFunding(ctx context.Context) ([]Symbol, error)
	DowngradeSymbolToReduceOnly(ctx context.Context, symbolID uint64) (bool, error)
	GetBatchByWindow(ctx context.Context, symbolID uint64, start time.Time, end time.Time) (Batch, error)
	ListExecutableBatches(ctx context.Context, limit int) ([]Batch, error)
	GetBatchByIDForUpdate(ctx context.Context, fundingBatchID string) (Batch, error)
	GetSettlementPriceAtOrBefore(ctx context.Context, symbolID uint64, cutoff time.Time) (SettlementPrice, error)
	ListLatestRateSnapshots(ctx context.Context, symbolID uint64, cutoff time.Time) ([]RateSnapshot, error)
	AppendRateSnapshots(ctx context.Context, snapshots []RateSnapshot) error
	CreateBatch(ctx context.Context, batch Batch) error
	UpdateBatch(ctx context.Context, batch Batch) error
	MarkBatchFailed(ctx context.Context, fundingBatchID string, failedAt time.Time) error
	CreateBatchItems(ctx context.Context, items []BatchItem) error
	ListBatchItemsForUpdate(ctx context.Context, fundingBatchID string) ([]BatchItem, error)
	UpdateBatchItem(ctx context.Context, item BatchItem) error
	ListOpenPositionsForUpdate(ctx context.Context, symbolID uint64) ([]PositionSnapshot, error)
	GetPositionByIDForUpdate(ctx context.Context, positionID string) (PositionSnapshot, error)
	UpdatePositionFundingAccrual(ctx context.Context, positionID string, fundingAccrual string, updatedAt time.Time) error
}

type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID(prefix string) string
}

type AccountResolver interface {
	ResolveFundingAccounts(ctx context.Context, userID uint64, asset string) (FundingAccounts, error)
}

type LedgerPoster interface {
	Post(ctx context.Context, req ledgerdomain.PostingRequest) error
}

type EventPublisher interface {
	PublishBatchApplied(ctx context.Context, event BatchAppliedEvent) error
	PublishBatchReversed(ctx context.Context, event BatchReversedEvent) error
}

type RateProvider interface {
	GetRates(ctx context.Context, symbol Symbol, windowStart time.Time, windowEnd time.Time) ([]SourceRate, error)
}

type RateSourceClient interface {
	Name() string
	FetchRates(ctx context.Context, symbols []SourceRateRequest) (map[string]SourceRateQuote, error)
}

type RuntimeConfigProvider interface {
	CurrentFundingRuntimeConfig(symbol string) ServiceConfig
}
