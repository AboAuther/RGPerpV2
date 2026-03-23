package liquidation

import "time"

const (
	StatusExecuting     = "EXECUTING"
	StatusExecuted      = "EXECUTED"
	StatusAborted       = "ABORTED"
	StatusPendingManual = "PENDING_MANUAL"

	ModeFull = "FULL"
)

type ExecuteInput struct {
	LiquidationID         string
	UserID                uint64
	TriggerRiskSnapshotID uint64
	TraceID               string
}

type Liquidation struct {
	ID                    string
	UserID                uint64
	SymbolID              *uint64
	Mode                  string
	Status                string
	TriggerRiskSnapshotID uint64
	PenaltyAmount         string
	InsuranceFundUsed     string
	BankruptAmount        string
	AbortReason           *string
	PreAccountSnapshot    *AccountSnapshot
	PostAccountSnapshot   *AccountSnapshot
	PrePositionsSnapshot  []PositionSnapshot
	PostPositionsSnapshot []PositionSnapshot
	ReleasedOrders        []ReleasedOrderSnapshot
	PriceSnapshot         []PriceSnapshot
	ConfigSnapshot        *ConfigSnapshot
	SettlementSnapshot    *SettlementSnapshot
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type AccountSnapshot struct {
	Equity            string `json:"equity"`
	AvailableBalance  string `json:"available_balance"`
	MaintenanceMargin string `json:"maintenance_margin"`
	MarginRatio       string `json:"margin_ratio"`
	RiskLevel         string `json:"risk_level"`
	WalletBalance     string `json:"wallet_balance"`
	InsuranceBalance  string `json:"insurance_balance"`
}

type PositionSnapshot struct {
	PositionID        string `json:"position_id"`
	SymbolID          uint64 `json:"symbol_id"`
	Symbol            string `json:"symbol"`
	Side              string `json:"side"`
	Qty               string `json:"qty"`
	AvgEntryPrice     string `json:"avg_entry_price"`
	MarkPrice         string `json:"mark_price"`
	Notional          string `json:"notional"`
	InitialMargin     string `json:"initial_margin"`
	MaintenanceMargin string `json:"maintenance_margin"`
	UnrealizedPnL     string `json:"unrealized_pnl"`
	FundingAccrual    string `json:"funding_accrual"`
	LiquidationPrice  string `json:"liquidation_price"`
	BankruptcyPrice   string `json:"bankruptcy_price"`
	Status            string `json:"status"`
	ExecutionPrice    string `json:"execution_price,omitempty"`
	RealizedPnL       string `json:"realized_pnl,omitempty"`
	Penalty           string `json:"penalty,omitempty"`
}

type ReleasedOrderSnapshot struct {
	OrderID       string `json:"order_id"`
	ClientOrderID string `json:"client_order_id"`
	SymbolID      uint64 `json:"symbol_id"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	Qty           string `json:"qty"`
	FrozenMargin  string `json:"frozen_margin"`
	Status        string `json:"status"`
}

type PriceSnapshot struct {
	PositionID      string `json:"position_id"`
	SymbolID        uint64 `json:"symbol_id"`
	Symbol          string `json:"symbol"`
	MarkPrice       string `json:"mark_price"`
	ExecutionPrice  string `json:"execution_price,omitempty"`
	PriceSource     string `json:"price_source"`
	SlippageApplied string `json:"slippage_applied"`
}

type ConfigSnapshot struct {
	Asset            string `json:"asset"`
	PenaltyRate      string `json:"penalty_rate"`
	ExtraSlippageBps int    `json:"extra_slippage_bps"`
}

type SettlementSnapshot struct {
	ReleasedOrderMargin string `json:"released_order_margin"`
	TotalInitialMargin  string `json:"total_initial_margin"`
	TotalRealizedPnL    string `json:"total_realized_pnl"`
	TotalPenalty        string `json:"total_penalty"`
	WalletDebit         string `json:"wallet_debit"`
	WalletCredit        string `json:"wallet_credit"`
	InsuranceDebit      string `json:"insurance_debit"`
	BankruptAmount      string `json:"bankrupt_amount"`
}

type Item struct {
	LiquidationID  string
	PositionID     string
	LiquidatedQty  string
	ExecutionPrice string
	LedgerTxID     string
	CreatedAt      time.Time
}

type Position struct {
	PositionID         string
	UserID             uint64
	SymbolID           uint64
	Symbol             string
	Side               string
	Qty                string
	AvgEntryPrice      string
	MarkPrice          string
	Notional           string
	InitialMargin      string
	MaintenanceMargin  string
	RealizedPnL        string
	UnrealizedPnL      string
	FundingAccrual     string
	LiquidationPrice   string
	BankruptcyPrice    string
	ContractMultiplier string
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type RiskIncreaseOrder struct {
	OrderID       string
	ClientOrderID string
	UserID        uint64
	SymbolID      uint64
	Symbol        string
	Side          string
	Qty           string
	FrozenMargin  string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type OrderRecord struct {
	OrderID        string
	ClientOrderID  string
	UserID         uint64
	SymbolID       uint64
	Side           string
	PositionEffect string
	Type           string
	TimeInForce    string
	Qty            string
	FilledQty      string
	AvgFillPrice   string
	ReduceOnly     bool
	MaxSlippageBps int
	Status         string
	FrozenMargin   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type FillRecord struct {
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

type Accounts struct {
	UserWalletAccountID         uint64
	UserOrderMarginAccountID    uint64
	UserPositionMarginAccountID uint64
	SystemPoolAccountID         uint64
	PenaltyAccountID            uint64
	InsuranceFundAccountID      uint64
}

type CoverageBalances struct {
	UserWalletBalance    string
	InsuranceFundBalance string
}
