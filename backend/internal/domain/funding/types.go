package funding

import "time"

const (
	ModelExternalAvg = "EXTERNAL_AVG"

	BatchStatusDraft    = "DRAFT"
	BatchStatusReady    = "READY"
	BatchStatusApplying = "APPLYING"
	BatchStatusApplied  = "APPLIED"
	BatchStatusFailed   = "FAILED"
	BatchStatusReversed = "REVERSED"

	ItemStatusPending  = "PENDING"
	ItemStatusApplied  = "APPLIED"
	ItemStatusFailed   = "FAILED"
	ItemStatusReversed = "REVERSED"

	PositionSideLong  = "LONG"
	PositionSideShort = "SHORT"
)

const (
	FailureReasonInsufficientSources = "INSUFFICIENT_SOURCES"
)

type SourceRate struct {
	SourceName      string
	Rate            string
	IntervalSeconds int
}

type SourceRateRequest struct {
	CanonicalSymbol string
	SourceSymbol    string
}

type SourceRateQuote struct {
	SourceName      string
	SourceSymbol    string
	Rate            string
	IntervalSeconds int
	SourceTS        time.Time
	ReceivedTS      time.Time
}

type RateSnapshot struct {
	SymbolID        uint64
	Symbol          string
	SourceName      string
	SourceSymbol    string
	Rate            string
	IntervalSeconds int
	SourceTS        time.Time
	ReceivedTS      time.Time
	CollectedAt     time.Time
}

type SymbolMapping struct {
	SourceName   string
	SourceSymbol string
	Status       string
}

type Symbol struct {
	ID         uint64
	Symbol     string
	AssetClass string
	Status     string
	Mappings   []SymbolMapping
}

type SettlementPrice struct {
	SymbolID  uint64
	Price     string
	CreatedAt time.Time
}

type PositionSnapshot struct {
	PositionID             string
	UserID                 uint64
	SymbolID               uint64
	Symbol                 string
	Side                   string
	Qty                    string
	ContractMultiplier     string
	AvgEntryPrice          string
	ExistingFundingAccrual string
}

type Batch struct {
	ID              string
	SymbolID        uint64
	Symbol          string
	TimeWindowStart time.Time
	TimeWindowEnd   time.Time
	NormalizedRate  string
	SettlementPrice string
	Status          string
	ReversedAt      *time.Time
	ReversedBy      *string
	ReversalReason  *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type BatchItem struct {
	FundingBatchID     string
	PositionID         string
	UserID             uint64
	FundingFee         string
	LedgerTxID         *string
	ReversalLedgerTxID *string
	Status             string
	CreatedAt          time.Time
	ReversedAt         *time.Time
}

type FundingAccounts struct {
	UserPositionMarginAccountID uint64
	FundingPoolAccountID        uint64
}

type ApplyResult struct {
	Batch   Batch
	UserIDs []uint64
}

type ReverseResult struct {
	Batch   Batch
	UserIDs []uint64
}

type BatchPlan struct {
	Batch Batch
	Items []BatchItem
}

type BuildBatchInput struct {
	FundingBatchID  string
	SymbolID        uint64
	Symbol          string
	TimeWindowStart time.Time
	TimeWindowEnd   time.Time
	SettlementPrice string
	Sources         []SourceRate
	Positions       []PositionSnapshot
	CreatedAt       time.Time
}

type BatchAppliedEvent struct {
	FundingBatchID  string    `json:"funding_batch_id"`
	Symbol          string    `json:"symbol"`
	TimeWindowStart time.Time `json:"time_window_start"`
	TimeWindowEnd   time.Time `json:"time_window_end"`
	NormalizedRate  string    `json:"normalized_rate"`
	Status          string    `json:"status"`
	AppliedCount    int       `json:"applied_count"`
}

type BatchReversedEvent struct {
	FundingBatchID  string    `json:"funding_batch_id"`
	Symbol          string    `json:"symbol"`
	TimeWindowStart time.Time `json:"time_window_start"`
	TimeWindowEnd   time.Time `json:"time_window_end"`
	NormalizedRate  string    `json:"normalized_rate"`
	Status          string    `json:"status"`
	ReversedCount   int       `json:"reversed_count"`
	ReversedAt      time.Time `json:"reversed_at"`
	ReversedBy      *string   `json:"reversed_by,omitempty"`
	ReversalReason  *string   `json:"reversal_reason,omitempty"`
}

type ReverseBatchInput struct {
	FundingBatchID string
	OperatorID     string
	TraceID        string
	Reason         string
}
