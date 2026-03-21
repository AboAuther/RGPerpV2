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

type ExplorerEvent struct {
	EventID     string         `json:"event_id"`
	EventType   string         `json:"event_type"`
	LedgerTxID  *string        `json:"ledger_tx_id,omitempty"`
	ChainTxHash *string        `json:"chain_tx_hash,omitempty"`
	Address     *string        `json:"address,omitempty"`
	Payload     map[string]any `json:"payload"`
}
