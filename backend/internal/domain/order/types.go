package order

import "time"

const (
	OrderTypeMarket = "MARKET"
	OrderTypeLimit  = "LIMIT"

	PositionEffectOpen   = "OPEN"
	PositionEffectReduce = "REDUCE"
	PositionEffectClose  = "CLOSE"

	OrderStatusResting  = "RESTING"
	OrderStatusFilled   = "FILLED"
	OrderStatusCanceled = "CANCELED"
	OrderStatusRejected = "REJECTED"

	PositionSideLong     = "LONG"
	PositionSideShort    = "SHORT"
	PositionStatusOpen   = "OPEN"
	PositionStatusClosed = "CLOSED"
)

type CreateOrderInput struct {
	UserID         uint64
	ClientOrderID  string
	Symbol         string
	Side           string
	PositionEffect string
	Type           string
	TimeInForce    string
	Price          *string
	TriggerPrice   *string
	Qty            string
	ReduceOnly     bool
	MaxSlippageBps int
	IdempotencyKey string
	TraceID        string
}

type CancelOrderInput struct {
	UserID         uint64
	OrderID        string
	IdempotencyKey string
	TraceID        string
}

type Order struct {
	OrderID        string
	ClientOrderID  string
	UserID         uint64
	SymbolID       uint64
	Symbol         string
	Side           string
	PositionEffect string
	Type           string
	TimeInForce    string
	Price          *string
	TriggerPrice   *string
	Qty            string
	FilledQty      string
	AvgFillPrice   string
	ReduceOnly     bool
	MaxSlippageBps int
	Status         string
	RejectReason   *string
	FrozenMargin   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Fill struct {
	FillID     string
	OrderID    string
	UserID     uint64
	SymbolID   uint64
	Side       string
	Qty        string
	Price      string
	FeeAmount  string
	LedgerTxID string
	CreatedAt  time.Time
}

type Position struct {
	PositionID        string
	UserID            uint64
	SymbolID          uint64
	Side              string
	Qty               string
	AvgEntryPrice     string
	MarkPrice         string
	Notional          string
	InitialMargin     string
	MaintenanceMargin string
	RealizedPnL       string
	UnrealizedPnL     string
	FundingAccrual    string
	LiquidationPrice  string
	BankruptcyPrice   string
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type TradableSymbol struct {
	SymbolID              uint64
	Symbol                string
	ContractMultiplier    string
	TickSize              string
	StepSize              string
	MinNotional           string
	Status                string
	IndexPrice            string
	MarkPrice             string
	BestBid               string
	BestAsk               string
	InitialMarginRate     string
	MaintenanceMarginRate string
	SnapshotTS            time.Time
}
