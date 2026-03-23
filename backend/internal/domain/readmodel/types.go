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

type SystemChainItem struct {
	ChainID           int64   `json:"chain_id"`
	Key               string  `json:"key"`
	Name              string  `json:"name"`
	Asset             string  `json:"asset"`
	Confirmations     int     `json:"confirmations"`
	LocalTestnet      bool    `json:"local_testnet"`
	LocalToolsEnabled bool    `json:"local_tools_enabled"`
	DepositEnabled    bool    `json:"deposit_enabled"`
	WithdrawEnabled   bool    `json:"withdraw_enabled"`
	USDCAddress       *string `json:"usdc_address,omitempty"`
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

type AdminWithdrawReviewItem struct {
	WithdrawID  string  `json:"withdraw_id"`
	UserID      uint64  `json:"user_id"`
	UserAddress string  `json:"user_address"`
	ChainID     int64   `json:"chain_id"`
	Asset       string  `json:"asset"`
	Amount      string  `json:"amount"`
	FeeAmount   string  `json:"fee_amount"`
	ToAddress   string  `json:"to_address"`
	Status      string  `json:"status"`
	RiskFlag    *string `json:"risk_flag,omitempty"`
	TxHash      *string `json:"tx_hash,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

type LedgerAssetOverview struct {
	Asset                 string `json:"asset"`
	UserWallet            string `json:"user_wallet"`
	UserOrderMargin       string `json:"user_order_margin"`
	UserPositionMargin    string `json:"user_position_margin"`
	UserWithdrawHold      string `json:"user_withdraw_hold"`
	UserMargin            string `json:"user_margin"`
	UserLiability         string `json:"user_liability"`
	SystemPool            string `json:"system_pool"`
	TradingFeeAccount     string `json:"trading_fee_account"`
	WithdrawFeeAccount    string `json:"withdraw_fee_account"`
	PenaltyAccount        string `json:"penalty_account"`
	FundingPool           string `json:"funding_pool"`
	InsuranceFund         string `json:"insurance_fund"`
	RoundingDiffAccount   string `json:"rounding_diff_account"`
	DepositPendingConfirm string `json:"deposit_pending_confirm"`
	WithdrawInTransit     string `json:"withdraw_in_transit"`
	SweepInTransit        string `json:"sweep_in_transit"`
	CustodyHot            string `json:"custody_hot"`
	CustodyWarm           string `json:"custody_warm"`
	CustodyCold           string `json:"custody_cold"`
	TestFaucetPool        string `json:"test_faucet_pool"`
	PlatformRevenue       string `json:"platform_revenue"`
	RiskBuffer            string `json:"risk_buffer"`
	InFlight              string `json:"in_flight"`
	CustodyMirror         string `json:"custody_mirror"`
	NetBalance            string `json:"net_balance"`
}

type LedgerOverview struct {
	ScopeAsset  string                `json:"scope_asset"`
	GeneratedAt string                `json:"generated_at"`
	Notes       []string              `json:"notes"`
	Assets      []LedgerAssetOverview `json:"assets"`
}

type LedgerChainBalance struct {
	RowType        string `json:"row_type"`
	ChainID        int64  `json:"chain_id"`
	ChainKey       string `json:"chain_key"`
	ChainName      string `json:"chain_name"`
	Asset          string `json:"asset"`
	VaultAddress   string `json:"vault_address"`
	OnchainBalance string `json:"onchain_balance"`
	CustodyMirror  string `json:"custody_mirror"`
	Delta          string `json:"delta"`
	Status         string `json:"status"`
}

type LedgerAuditCheck struct {
	CheckKey   string   `json:"check_key"`
	Label      string   `json:"label"`
	Status     string   `json:"status"`
	Value      string   `json:"value"`
	Summary    string   `json:"summary"`
	SampleRefs []string `json:"sample_refs,omitempty"`
}

type LedgerAuditReport struct {
	AuditReportID string                `json:"audit_report_id"`
	ScopeAsset    string                `json:"scope_asset"`
	Status        string                `json:"status"`
	ExecutedBy    string                `json:"executed_by"`
	StartedAt     string                `json:"started_at"`
	FinishedAt    string                `json:"finished_at"`
	Overview      []LedgerAssetOverview `json:"overview"`
	ChainBalances []LedgerChainBalance  `json:"chain_balances,omitempty"`
	Checks        []LedgerAuditCheck    `json:"checks"`
}

type SymbolNetExposureItem struct {
	Symbol            string  `json:"symbol"`
	Status            string  `json:"status"`
	MarkPrice         string  `json:"mark_price"`
	LongQty           string  `json:"long_qty"`
	ShortQty          string  `json:"short_qty"`
	NetQty            string  `json:"net_qty"`
	NetNotional       string  `json:"net_notional"`
	HardLimitNotional string  `json:"hard_limit_notional"`
	UtilizationRatio  string  `json:"utilization_ratio"`
	BlockedOpenSide   *string `json:"blocked_open_side,omitempty"`
	BuyAdjustmentBps  int     `json:"buy_adjustment_bps"`
	SellAdjustmentBps int     `json:"sell_adjustment_bps"`
}

type RiskMonitorDashboard struct {
	GeneratedAt           string                  `json:"generated_at"`
	HardLimitNotional     string                  `json:"hard_limit_notional"`
	MaxDynamicSlippageBps int                     `json:"max_dynamic_slippage_bps"`
	Items                 []SymbolNetExposureItem `json:"items"`
}

type AdminRiskRecalculationResult struct {
	UserID            uint64  `json:"user_id"`
	RiskSnapshotID    uint64  `json:"risk_snapshot_id"`
	MarginRatio       string  `json:"margin_ratio"`
	RiskLevel         string  `json:"risk_level"`
	TriggeredBy       string  `json:"triggered_by"`
	LiquidationID     *string `json:"liquidation_id,omitempty"`
	LiquidationStatus *string `json:"liquidation_status,omitempty"`
}

type RuntimeConfigSnapshotView struct {
	SystemMode                        string `json:"system_mode"`
	ReadOnly                          bool   `json:"read_only"`
	ReduceOnly                        bool   `json:"reduce_only"`
	TraceHeaderRequired               bool   `json:"trace_header_required"`
	RiskGlobalBufferRatio             string `json:"risk_global_buffer_ratio"`
	RiskMarkPriceStaleSec             int    `json:"risk_mark_price_stale_sec"`
	RiskForceReduceOnlyOnStalePrice   bool   `json:"risk_force_reduce_only_on_stale_price"`
	RiskLiquidationPenaltyRate        string `json:"risk_liquidation_penalty_rate"`
	RiskLiquidationExtraSlippageBps   int    `json:"risk_liquidation_extra_slippage_bps"`
	RiskMaxOpenOrdersPerUserPerSymbol int    `json:"risk_max_open_orders_per_user_per_symbol"`
	RiskNetExposureHardLimit          string `json:"risk_net_exposure_hard_limit"`
	RiskMaxExposureSlippageBps        int    `json:"risk_max_exposure_slippage_bps"`
	HedgeEnabled                      bool   `json:"hedge_enabled"`
	HedgeSoftThresholdRatio           string `json:"hedge_soft_threshold_ratio"`
	HedgeHardThresholdRatio           string `json:"hedge_hard_threshold_ratio"`
}

type RuntimeConfigHistoryItem struct {
	ConfigKey   string  `json:"config_key"`
	ScopeType   string  `json:"scope_type"`
	ScopeValue  string  `json:"scope_value"`
	Version     int64   `json:"version"`
	Value       any     `json:"value"`
	Status      string  `json:"status"`
	CreatedBy   string  `json:"created_by"`
	ApprovedBy  *string `json:"approved_by,omitempty"`
	Reason      string  `json:"reason"`
	EffectiveAt string  `json:"effective_at"`
	CreatedAt   string  `json:"created_at"`
}

type RuntimeConfigView struct {
	Snapshot    RuntimeConfigSnapshotView  `json:"snapshot"`
	GeneratedAt string                     `json:"generated_at"`
	History     []RuntimeConfigHistoryItem `json:"history"`
}

type TransferItem struct {
	TransferID          string `json:"transfer_id"`
	Asset               string `json:"asset"`
	Amount              string `json:"amount"`
	Direction           string `json:"direction"`
	CounterpartyAddress string `json:"counterparty_address"`
	FromAccount         string `json:"from_account"`
	ToAccount           string `json:"to_account"`
	Status              string `json:"status"`
	CreatedAt           string `json:"created_at"`
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
	Status     string `json:"status"`
	Stale      bool   `json:"stale"`
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
	CreatedAt      string  `json:"created_at"`
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
	Asset       *string        `json:"asset,omitempty"`
	Amount      *string        `json:"amount,omitempty"`
	CreatedAt   string         `json:"created_at"`
	LedgerTxID  *string        `json:"ledger_tx_id,omitempty"`
	ChainTxHash *string        `json:"chain_tx_hash,omitempty"`
	OrderID     *string        `json:"order_id,omitempty"`
	FillID      *string        `json:"fill_id,omitempty"`
	PositionID  *string        `json:"position_id,omitempty"`
	Address     *string        `json:"address,omitempty"`
	Payload     map[string]any `json:"payload"`
}
