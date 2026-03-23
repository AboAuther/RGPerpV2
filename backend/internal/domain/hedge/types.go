package hedge

import "time"

const (
	IntentStatusPending   = "PENDING"
	IntentStatusExecuting = "EXECUTING"
	IntentStatusCompleted = "COMPLETED"
	IntentStatusFailed    = "FAILED"

	OrderStatusPending = "PENDING"
	OrderStatusSent    = "SENT"
	OrderStatusFilled  = "FILLED"
	OrderStatusFailed  = "FAILED"
	OrderStatusPartial = "PARTIALLY_FILLED"

	OrderSideBuy  = "BUY"
	OrderSideSell = "SELL"

	PositionSideLong  = "LONG"
	PositionSideShort = "SHORT"
)

type Intent struct {
	ID                 string
	SymbolID           uint64
	Symbol             string
	Side               string
	TargetQty          string
	CurrentNetExposure string
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Order struct {
	ID           string
	IntentID     string
	Venue        string
	VenueOrderID *string
	Symbol       string
	Side         string
	Qty          string
	Price        *string
	Status       string
	ErrorCode    *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Fill struct {
	ID          string
	OrderID     string
	VenueFillID string
	Qty         string
	Price       string
	Fee         string
	CreatedAt   time.Time
}

type Position struct {
	Symbol        string
	Side          string
	Qty           string
	AvgEntryPrice string
	RealizedPnL   string
	UnrealizedPnL string
	UpdatedAt     time.Time
}

type ExecutionRequest struct {
	Symbol string
	Side   string
	Qty    string
	Price  *string
}

type ExecutionFill struct {
	VenueFillID string
	Qty         string
	Price       string
	Fee         string
}

type ExecutionResult struct {
	VenueOrderID string
	Status       string
	Fills        []ExecutionFill
}
