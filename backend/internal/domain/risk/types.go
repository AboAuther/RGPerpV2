package risk

import (
	"time"

	hedgedomain "github.com/xiaobao/rgperp/backend/internal/domain/hedge"
)

const (
	RiskLevelSafe        = "SAFE"
	RiskLevelNoNewRisk   = "NO_NEW_RISK"
	RiskLevelLiquidating = "LIQUIDATING"
)

type AccountState struct {
	UserID                uint64
	WalletBalance         string
	OrderMarginBalance    string
	PositionMarginBalance string
	WithdrawHoldBalance   string
	Positions             []PositionExposure
	PendingOrders         []PendingOrderExposure
}

type PositionExposure struct {
	PositionID         string
	SymbolID           uint64
	Symbol             string
	Side               string
	MarginMode         string
	Qty                string
	AvgEntryPrice      string
	MarkPrice          string
	Notional           string
	InitialMargin      string
	MaintenanceMargin  string
	UnrealizedPnL      string
	FundingAccrual     string
	LiquidationPrice   string
	LiquidationFeeRate string
	ContractMultiplier string
	MarkPriceUpdatedAt time.Time
}

type PendingOrderExposure struct {
	OrderID             string
	SymbolID            uint64
	Symbol              string
	Side                string
	Type                string
	Qty                 string
	Price               *string
	TriggerPrice        *string
	FrozenInitialMargin string
	FrozenFee           string
	FrozenMargin        string
	MarkPrice           string
	ContractMultiplier  string
	MarkPriceUpdatedAt  time.Time
}

type Snapshot struct {
	ID                uint64
	UserID            uint64
	Equity            string
	AvailableBalance  string
	MaintenanceMargin string
	MarginRatio       string
	RiskLevel         string
	TriggeredBy       string
	CreatedAt         time.Time
}

type LiquidationTrigger struct {
	LiquidationID     string
	UserID            uint64
	Mode              string
	PositionID        string
	Symbol            string
	MarginRatio       string
	Equity            string
	MaintenanceMargin string
	TriggeredAt       time.Time
	Status            string
	SnapshotID        uint64
}

type AccountMetrics struct {
	Equity               string
	AvailableBalance     string
	InitialMargin        string
	MaintenanceMargin    string
	UnrealizedPnL        string
	MarginRatio          string
	RiskLevel            string
	PendingTradingFee    string
	LiquidationFeeBuffer string
	TotalNotional        string
}

type HedgeState struct {
	SymbolID         uint64
	Symbol           string
	InternalLongQty  string
	InternalShortQty string
	ManagedLongQty   string
	ManagedShortQty  string
}

type HedgeDecision struct {
	Intent hedgedomain.Intent
	Drift  string
}
