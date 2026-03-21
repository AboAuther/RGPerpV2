package readmodel

type AccountSummary struct {
	Equity                 string `json:"equity"`
	AvailableBalance       string `json:"available_balance"`
	TotalInitialMargin     string `json:"total_initial_margin"`
	TotalMaintenanceMargin string `json:"total_maintenance_margin"`
	UnrealizedPnL          string `json:"unrealized_pnl"`
	MarginRatio            string `json:"margin_ratio"`
}

type BalanceItem struct {
	AccountCode string `json:"account_code"`
	Asset       string `json:"asset"`
	Balance     string `json:"balance"`
}

type RiskSnapshot struct {
	AccountStatus  string   `json:"account_status"`
	RiskState      string   `json:"risk_state"`
	MarkPriceStale bool     `json:"mark_price_stale"`
	CanOpenRisk    bool     `json:"can_open_risk"`
	Notes          []string `json:"notes"`
}

type DepositAddressItem struct {
	ChainID       int64  `json:"chain_id"`
	Asset         string `json:"asset"`
	Address       string `json:"address"`
	Confirmations int    `json:"confirmations"`
}

type DepositItem struct {
	DepositID             string `json:"deposit_id"`
	ChainID               int64  `json:"chain_id"`
	Asset                 string `json:"asset"`
	Amount                string `json:"amount"`
	TxHash                string `json:"tx_hash"`
	Confirmations         int    `json:"confirmations"`
	RequiredConfirmations int    `json:"required_confirmations"`
	Status                string `json:"status"`
	Address               string `json:"address"`
	DetectedAt            string `json:"detected_at"`
}

type WithdrawItem struct {
	WithdrawID string  `json:"withdraw_id"`
	ChainID    int64   `json:"chain_id"`
	Asset      string  `json:"asset"`
	Amount     string  `json:"amount"`
	FeeAmount  string  `json:"fee_amount"`
	ToAddress  string  `json:"to_address"`
	Status     string  `json:"status"`
	TxHash     *string `json:"tx_hash"`
	CreatedAt  string  `json:"created_at"`
}

type TransferItem struct {
	TransferID  string `json:"transfer_id"`
	Asset       string `json:"asset"`
	Amount      string `json:"amount"`
	FromAccount string `json:"from_account"`
	ToAccount   string `json:"to_account"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type SymbolItem struct {
	Symbol      string `json:"symbol"`
	AssetClass  string `json:"asset_class"`
	TickSize    string `json:"tick_size"`
	StepSize    string `json:"step_size"`
	MinNotional string `json:"min_notional"`
	Status      string `json:"status"`
}

type TickerItem struct {
	Symbol     string `json:"symbol"`
	IndexPrice string `json:"index_price"`
	MarkPrice  string `json:"mark_price"`
	BestBid    string `json:"best_bid"`
	BestAsk    string `json:"best_ask"`
	TS         string `json:"ts"`
}

type OrderItem struct {
	OrderID        string  `json:"order_id"`
	ClientOrderID  string  `json:"client_order_id"`
	Symbol         string  `json:"symbol"`
	Side           string  `json:"side"`
	PositionEffect string  `json:"position_effect"`
	Type           string  `json:"type"`
	Qty            string  `json:"qty"`
	FilledQty      string  `json:"filled_qty"`
	AvgFillPrice   string  `json:"avg_fill_price"`
	Price          *string `json:"price"`
	TriggerPrice   *string `json:"trigger_price"`
	ReduceOnly     bool    `json:"reduce_only"`
	Status         string  `json:"status"`
	RejectReason   *string `json:"reject_reason"`
}

type FillItem struct {
	FillID    string `json:"fill_id"`
	OrderID   string `json:"order_id"`
	Symbol    string `json:"symbol"`
	Side      string `json:"side"`
	Qty       string `json:"qty"`
	Price     string `json:"price"`
	FeeAmount string `json:"fee_amount"`
	CreatedAt string `json:"created_at"`
}

type PositionItem struct {
	PositionID        string `json:"position_id"`
	Symbol            string `json:"symbol"`
	Side              string `json:"side"`
	Qty               string `json:"qty"`
	AvgEntryPrice     string `json:"avg_entry_price"`
	MarkPrice         string `json:"mark_price"`
	InitialMargin     string `json:"initial_margin"`
	MaintenanceMargin string `json:"maintenance_margin"`
	RealizedPnL       string `json:"realized_pnl"`
	UnrealizedPnL     string `json:"unrealized_pnl"`
	FundingAccrual    string `json:"funding_accrual"`
	LiquidationPrice  string `json:"liquidation_price"`
	Status            string `json:"status"`
}

type FundingItem struct {
	FundingID string `json:"funding_id"`
	Symbol    string `json:"symbol"`
	Direction string `json:"direction"`
	Rate      string `json:"rate"`
	Amount    string `json:"amount"`
	SettledAt string `json:"settled_at"`
	BatchID   string `json:"batch_id"`
}

type ExplorerEvent struct {
	EventID     string         `json:"event_id"`
	EventType   string         `json:"event_type"`
	LedgerTxID  *string        `json:"ledger_tx_id,omitempty"`
	ChainTxHash *string        `json:"chain_tx_hash,omitempty"`
	Address     *string        `json:"address,omitempty"`
	Payload     map[string]any `json:"payload"`
}
