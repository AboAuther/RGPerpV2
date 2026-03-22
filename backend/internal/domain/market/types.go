package market

import "time"

type Symbol struct {
	ID                 uint64
	Symbol             string
	AssetClass         string
	BaseAsset          string
	QuoteAsset         string
	ContractMultiplier string
	TickSize           string
	StepSize           string
	MinNotional        string
	Status             string
	SessionPolicy      string
	Mappings           []SymbolMapping
}

type SymbolMapping struct {
	SourceName   string
	SourceSymbol string
	PriceScale   string
	QtyScale     string
	Status       string
}

type SourceQuote struct {
	SourceName   string
	SourceSymbol string
	Bid          string
	Ask          string
	Last         string
	SourceTS     time.Time
	ReceivedTS   time.Time
}

type SourceSymbolRequest struct {
	CanonicalSymbol string
	SourceSymbol    string
}

type SourcePriceSnapshot struct {
	SymbolID    uint64
	SourceName  string
	Bid         string
	Ask         string
	Last        string
	Mid         string
	SourceTS    time.Time
	ReceivedTS  time.Time
	CanonicalTS time.Time
}

type AggregatedPrice struct {
	SymbolID      uint64
	IndexPrice    string
	MarkPrice     string
	BestBid       string
	BestAsk       string
	CalcVersion   int64
	CreatedAt     time.Time
	HealthyCount  int
	AcceptedCount int
}

type SymbolRuntimeState struct {
	SymbolID       uint64
	DesiredStatus  string
	DegradedReason string
}

type SourceHealth struct {
	Enabled bool
	Weight  string
}

type AggregationConfig struct {
	MaxSourceAge     time.Duration
	MaxDeviationBps  string
	MinHealthySource int
	MarkClampBps     string
	SourceHealth     map[string]SourceHealth
}
