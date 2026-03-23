package funding

import "time"

const (
	ModelExternalAvg = "EXTERNAL_AVG"

	BatchStatusDraft    = "DRAFT"
	BatchStatusReady    = "READY"
	BatchStatusApplying = "APPLYING"
	BatchStatusApplied  = "APPLIED"
	BatchStatusFailed   = "FAILED"

	ItemStatusPending = "PENDING"
	ItemStatusApplied = "APPLIED"
	ItemStatusFailed  = "FAILED"

	PositionSideLong  = "LONG"
	PositionSideShort = "SHORT"
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
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type BatchItem struct {
	FundingBatchID string
	PositionID     string
	UserID         uint64
	FundingFee     string
	LedgerTxID     *string
	Status         string
	CreatedAt      time.Time
}

type FundingAccounts struct {
	UserPositionMarginAccountID uint64
	FundingPoolAccountID        uint64
}

type ApplyResult struct {
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
