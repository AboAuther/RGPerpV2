package order

import "time"

const (
	OrderTypeMarket           = "MARKET"
	OrderTypeLimit            = "LIMIT"
	OrderTypeStopMarket       = "STOP_MARKET"
	OrderTypeTakeProfitMarket = "TAKE_PROFIT_MARKET"

	PositionEffectOpen   = "OPEN"
	PositionEffectReduce = "REDUCE"
	PositionEffectClose  = "CLOSE"

	OrderStatusTriggerWait = "TRIGGER_WAIT"
	OrderStatusResting     = "RESTING"
	OrderStatusFilled      = "FILLED"
	OrderStatusCanceled    = "CANCELED"
	OrderStatusRejected    = "REJECTED"

	PositionSideLong     = "LONG"
	PositionSideShort    = "SHORT"
	PositionStatusOpen   = "OPEN"
	PositionStatusClosed = "CLOSED"
	PositionStatusLiquidating = "LIQUIDATING"

	MarginModeIsolated = "ISOLATED"
	MarginModeCross    = "CROSS"
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
	Leverage       *string
	MarginMode     string
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
	OrderID             string
	ClientOrderID       string
	UserID              uint64
	SymbolID            uint64
	Symbol              string
	Side                string
	PositionEffect      string
	Type                string
	TimeInForce         string
	Price               *string
	TriggerPrice        *string
	Qty                 string
	FilledQty           string
	AvgFillPrice        string
	Leverage            string
	MarginMode          string
	ReduceOnly          bool
	MaxSlippageBps      int
	Status              string
	RejectReason        *string
	FrozenInitialMargin string
	FrozenFee           string
	FrozenMargin        string
	CreatedAt           time.Time
	UpdatedAt           time.Time
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

type Event struct {
	EventID       string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       any
	CreatedAt     time.Time
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
	Leverage          string
	MarginMode        string
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

type SymbolExposure struct {
	SymbolID uint64
	LongQty  string
	ShortQty string
}

type RiskTier struct {
	TierLevel          int
	MaxNotional        string
	MaxLeverage        string
	InitialMarginRate  string
	MaintenanceRate    string
	LiquidationFeeRate string
}

type TradableSymbol struct {
	SymbolID              uint64
	Symbol                string
	ContractMultiplier    string
	TickSize              string
	StepSize              string
	MinNotional           string
	Status                string
	SessionPolicy         string
	IndexPrice            string
	MarkPrice             string
	BestBid               string
	BestAsk               string
	InitialMarginRate     string
	MaintenanceMarginRate string
	RiskTiers             []RiskTier
	SnapshotTS            time.Time
}
