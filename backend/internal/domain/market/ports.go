package market

import "context"

type CatalogRepository interface {
	UpsertSymbols(ctx context.Context, symbols []Symbol) error
	ListActiveSymbols(ctx context.Context) ([]Symbol, error)
}

type SnapshotRepository interface {
	AppendSourceSnapshots(ctx context.Context, snapshots []SourcePriceSnapshot) error
	ApplyAggregatedState(ctx context.Context, snapshots []AggregatedPrice, runtimeStates []SymbolRuntimeState) error
}

type SourceClient interface {
	Name() string
	Fetch(ctx context.Context, symbols []SourceSymbolRequest) (map[string]SourceQuote, error)
}
