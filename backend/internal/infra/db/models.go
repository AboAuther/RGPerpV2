package db

import (
	"time"

	"gorm.io/gorm"
)

// These GORM models are the canonical persistence map for the runtime system.
// They are intentionally grouped in one file so schema review can track the
// full operational data model in one place.
type UserModel struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	EVMAddress string    `gorm:"column:evm_address;size:64;uniqueIndex;not null"`
	Status     string    `gorm:"column:status;size:32;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null"`
}

func (UserModel) TableName() string { return "users" }

type LoginNonceModel struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement"`
	EVMAddress string     `gorm:"column:evm_address;size:64;not null;index"`
	Nonce      string     `gorm:"column:nonce;size:128;uniqueIndex;not null"`
	ChainID    int64      `gorm:"column:chain_id;not null"`
	Domain     string     `gorm:"column:domain;size:255;not null"`
	ExpiresAt  time.Time  `gorm:"column:expires_at;not null"`
	UsedAt     *time.Time `gorm:"column:used_at"`
	CreatedAt  time.Time  `gorm:"column:created_at;not null"`
}

func (LoginNonceModel) TableName() string { return "login_nonces" }

type SessionModel struct {
	ID                uint64     `gorm:"primaryKey;autoIncrement"`
	UserID            uint64     `gorm:"column:user_id;not null;index"`
	AccessJTI         string     `gorm:"column:access_jti;size:128;uniqueIndex;not null"`
	RefreshJTI        string     `gorm:"column:refresh_jti;size:128;uniqueIndex;not null"`
	DeviceFingerprint string     `gorm:"column:device_fingerprint;size:255;not null"`
	IP                string     `gorm:"column:ip;size:64;not null"`
	UserAgent         string     `gorm:"column:user_agent;size:512;not null"`
	AccessExpiresAt   time.Time  `gorm:"column:access_expires_at;not null"`
	RefreshExpiresAt  time.Time  `gorm:"column:refresh_expires_at;not null"`
	RevokedAt         *time.Time `gorm:"column:revoked_at"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null"`
}

func (SessionModel) TableName() string { return "sessions" }

// Accounts, ledger transactions, and balance snapshots form the monetary core.
// Snapshot tables optimize reads, while ledger tables remain the durable source
// of truth for every balance-affecting transition.
type AccountModel struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	UserID      *uint64   `gorm:"column:user_id;index:uk_user_code_asset,unique"`
	AccountCode string    `gorm:"column:account_code;size:64;not null;index:uk_user_code_asset,unique"`
	AccountType string    `gorm:"column:account_type;size:64;not null"`
	Asset       string    `gorm:"column:asset;size:32;not null;index:uk_user_code_asset,unique"`
	Status      string    `gorm:"column:status;size:32;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;not null"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null"`
}

func (AccountModel) TableName() string { return "accounts" }

type LedgerTxModel struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	LedgerTxID     string    `gorm:"column:ledger_tx_id;size:64;uniqueIndex;not null"`
	EventID        string    `gorm:"column:event_id;size:64;uniqueIndex;not null"`
	BizType        string    `gorm:"column:biz_type;size:64;not null"`
	BizRefID       string    `gorm:"column:biz_ref_id;size:64;not null"`
	Asset          string    `gorm:"column:asset;size:32;not null"`
	IdempotencyKey string    `gorm:"column:idempotency_key;size:128;uniqueIndex;not null"`
	OperatorType   string    `gorm:"column:operator_type;size:32;not null"`
	OperatorID     string    `gorm:"column:operator_id;size:64;not null"`
	TraceID        string    `gorm:"column:trace_id;size:64;not null"`
	Status         string    `gorm:"column:status;size:32;not null"`
	MetadataJSON   *string   `gorm:"column:metadata_json;type:json"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (LedgerTxModel) TableName() string { return "ledger_tx" }

type LedgerEntryModel struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	LedgerTxID string    `gorm:"column:ledger_tx_id;size:64;not null;index"`
	AccountID  uint64    `gorm:"column:account_id;not null;index"`
	UserID     *uint64   `gorm:"column:user_id;index"`
	Asset      string    `gorm:"column:asset;size:32;not null"`
	Amount     string    `gorm:"column:amount;type:decimal(38,18);not null"`
	EntryType  string    `gorm:"column:entry_type;size:64;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
}

func (LedgerEntryModel) TableName() string { return "ledger_entries" }

type AccountBalanceSnapshotModel struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	AccountID uint64    `gorm:"column:account_id;not null;uniqueIndex:uk_account_asset"`
	Asset     string    `gorm:"column:asset;size:32;not null;uniqueIndex:uk_account_asset"`
	Balance   string    `gorm:"column:balance;type:decimal(38,18);not null"`
	Version   int64     `gorm:"column:version;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (AccountBalanceSnapshotModel) TableName() string { return "account_balance_snapshots" }

type LedgerAuditReportModel struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement"`
	AuditReportID     string    `gorm:"column:audit_report_id;size:64;uniqueIndex;not null"`
	ScopeAsset        string    `gorm:"column:scope_asset;size:32;not null;index"`
	Status            string    `gorm:"column:status;size:16;not null;index"`
	ExecutedBy        string    `gorm:"column:executed_by;size:64;not null"`
	OverviewJSON      string    `gorm:"column:overview_json;type:json;not null"`
	ChainBalancesJSON *string   `gorm:"column:chain_balances_json;type:json"`
	ChecksJSON        string    `gorm:"column:checks_json;type:json;not null"`
	StartedAt         time.Time `gorm:"column:started_at;not null"`
	FinishedAt        time.Time `gorm:"column:finished_at;not null"`
	CreatedAt         time.Time `gorm:"column:created_at;not null"`
}

func (LedgerAuditReportModel) TableName() string { return "ledger_audit_reports" }

type OutboxEventModel struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement"`
	EventID       string     `gorm:"column:event_id;size:64;uniqueIndex;not null"`
	AggregateType string     `gorm:"column:aggregate_type;size:64;not null"`
	AggregateID   string     `gorm:"column:aggregate_id;size:64;not null"`
	EventType     string     `gorm:"column:event_type;size:128;not null"`
	PayloadJSON   string     `gorm:"column:payload_json;type:json;not null"`
	Status        string     `gorm:"column:status;size:32;not null;index"`
	PublishedAt   *time.Time `gorm:"column:published_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null"`
}

func (OutboxEventModel) TableName() string { return "outbox_events" }

type AuditLogModel struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	AuditID      string    `gorm:"column:audit_id;size:64;uniqueIndex;not null"`
	ActorType    string    `gorm:"column:actor_type;size:32;not null"`
	ActorID      string    `gorm:"column:actor_id;size:64;not null"`
	Action       string    `gorm:"column:action;size:128;not null"`
	ResourceType string    `gorm:"column:resource_type;size:64;not null;index:idx_audit_logs_resource_created,priority:1"`
	ResourceID   string    `gorm:"column:resource_id;size:64;not null;index:idx_audit_logs_resource_created,priority:2"`
	BeforeJSON   *string   `gorm:"column:before_json;type:json"`
	AfterJSON    *string   `gorm:"column:after_json;type:json"`
	TraceID      string    `gorm:"column:trace_id;size:64;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;index:idx_audit_logs_resource_created,priority:3"`
}

func (AuditLogModel) TableName() string { return "audit_logs" }

type ConfigItemModel struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	ConfigKey   string    `gorm:"column:config_key;size:128;not null;uniqueIndex:uk_config_items_key_scope_version,priority:1;index:idx_config_items_key_scope_status,priority:1"`
	ScopeType   string    `gorm:"column:scope_type;size:64;not null;uniqueIndex:uk_config_items_key_scope_version,priority:2;index:idx_config_items_key_scope_status,priority:2"`
	ScopeValue  string    `gorm:"column:scope_value;size:128;not null;uniqueIndex:uk_config_items_key_scope_version,priority:3;index:idx_config_items_key_scope_status,priority:3"`
	Version     int64     `gorm:"column:version;not null;uniqueIndex:uk_config_items_key_scope_version,priority:4"`
	ValueJSON   string    `gorm:"column:value_json;type:json;not null"`
	EffectiveAt time.Time `gorm:"column:effective_at;not null"`
	Status      string    `gorm:"column:status;size:32;not null;index:idx_config_items_key_scope_status,priority:4"`
	CreatedBy   string    `gorm:"column:created_by;size:64;not null"`
	ApprovedBy  *string   `gorm:"column:approved_by;size:64"`
	Reason      string    `gorm:"column:reason;size:255;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;not null"`
}

func (ConfigItemModel) TableName() string { return "config_items" }

type ChainCursorModel struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	ChainID     int64     `gorm:"column:chain_id;not null;uniqueIndex:uk_chain_cursor"`
	CursorType  string    `gorm:"column:cursor_type;size:64;not null;uniqueIndex:uk_chain_cursor"`
	CursorValue string    `gorm:"column:cursor_value;size:128;not null"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null"`
}

func (ChainCursorModel) TableName() string { return "chain_cursors" }

type MessageConsumptionModel struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	ConsumerName string    `gorm:"column:consumer_name;size:128;not null;uniqueIndex:uk_consumer_event"`
	EventID      string    `gorm:"column:event_id;size:64;not null;uniqueIndex:uk_consumer_event"`
	ConsumedAt   time.Time `gorm:"column:consumed_at;not null"`
}

func (MessageConsumptionModel) TableName() string { return "message_consumptions" }

// Deposit and withdraw tables model the full on-chain settlement state machine.
// They preserve enough detail for replay, reconciliation, and operator review.
type DepositAddressModel struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `gorm:"column:user_id;not null;uniqueIndex:uk_user_chain_asset;index"`
	ChainID   int64     `gorm:"column:chain_id;not null;uniqueIndex:uk_user_chain_asset;uniqueIndex:uk_chain_address,priority:1"`
	Address   string    `gorm:"column:address;size:64;not null;uniqueIndex:uk_chain_address,priority:2"`
	Asset     string    `gorm:"column:asset;size:32;not null;uniqueIndex:uk_user_chain_asset"`
	Status    string    `gorm:"column:status;size:32;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (DepositAddressModel) TableName() string { return "deposit_addresses" }

type DepositChainTxModel struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement"`
	DepositID          string    `gorm:"column:deposit_id;size:64;uniqueIndex;not null"`
	UserID             uint64    `gorm:"column:user_id;not null;index"`
	ChainID            int64     `gorm:"column:chain_id;not null;uniqueIndex:uk_chain_tx_log,priority:1"`
	TxHash             string    `gorm:"column:tx_hash;size:128;not null;uniqueIndex:uk_chain_tx_log,priority:2"`
	LogIndex           int64     `gorm:"column:log_index;not null;uniqueIndex:uk_chain_tx_log,priority:3"`
	FromAddress        string    `gorm:"column:from_address;size:64;not null"`
	ToAddress          string    `gorm:"column:to_address;size:64;not null"`
	TokenAddress       string    `gorm:"column:token_address;size:64;not null"`
	Amount             string    `gorm:"column:amount;type:decimal(38,18);not null"`
	BlockNumber        int64     `gorm:"column:block_number;not null"`
	Confirmations      int       `gorm:"column:confirmations;not null"`
	Status             string    `gorm:"column:status;size:32;not null;index"`
	CreditedLedgerTxID string    `gorm:"column:credited_ledger_tx_id;size:64"`
	CreatedAt          time.Time `gorm:"column:created_at;not null"`
	UpdatedAt          time.Time `gorm:"column:updated_at;not null"`
}

func (DepositChainTxModel) TableName() string { return "deposit_chain_txs" }

type WithdrawRequestModel struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement"`
	WithdrawID      string     `gorm:"column:withdraw_id;size:64;uniqueIndex;not null"`
	UserID          uint64     `gorm:"column:user_id;not null;index"`
	ChainID         int64      `gorm:"column:chain_id;not null"`
	Asset           string     `gorm:"column:asset;size:32;not null"`
	Amount          string     `gorm:"column:amount;type:decimal(38,18);not null"`
	FeeAmount       string     `gorm:"column:fee_amount;type:decimal(38,18);not null"`
	ToAddress       string     `gorm:"column:to_address;size:64;not null"`
	Status          string     `gorm:"column:status;size:32;not null;index"`
	RiskFlag        *string    `gorm:"column:risk_flag;size:64"`
	HoldLedgerTxID  string     `gorm:"column:hold_ledger_tx_id;size:64;not null"`
	BroadcastTxHash string     `gorm:"column:broadcast_tx_hash;size:128"`
	BroadcastNonce  *int64     `gorm:"column:broadcast_nonce"`
	CompletedAt     *time.Time `gorm:"column:completed_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;not null"`
}

func (WithdrawRequestModel) TableName() string { return "withdraw_requests" }

type SignerNonceStateModel struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	ChainID       int64     `gorm:"column:chain_id;not null;uniqueIndex:uk_signer_nonce_state,priority:1"`
	SignerAddress string    `gorm:"column:signer_address;size:64;not null;uniqueIndex:uk_signer_nonce_state,priority:2"`
	NextNonce     int64     `gorm:"column:next_nonce;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;not null"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null"`
}

func (SignerNonceStateModel) TableName() string { return "signer_nonce_states" }

type SymbolModel struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement"`
	Symbol             string    `gorm:"column:symbol;size:64;uniqueIndex;not null"`
	AssetClass         string    `gorm:"column:asset_class;size:32;not null"`
	BaseAsset          string    `gorm:"column:base_asset;size:32;not null"`
	QuoteAsset         string    `gorm:"column:quote_asset;size:32;not null"`
	ContractMultiplier string    `gorm:"column:contract_multiplier;type:decimal(38,18);not null"`
	TickSize           string    `gorm:"column:tick_size;type:decimal(38,18);not null"`
	StepSize           string    `gorm:"column:step_size;type:decimal(38,18);not null"`
	MinNotional        string    `gorm:"column:min_notional;type:decimal(38,18);not null"`
	Status             string    `gorm:"column:status;size:32;not null"`
	SessionPolicy      string    `gorm:"column:session_policy;size:32;not null"`
	CreatedAt          time.Time `gorm:"column:created_at;not null"`
	UpdatedAt          time.Time `gorm:"column:updated_at;not null"`
}

func (SymbolModel) TableName() string { return "symbols" }

type SymbolMappingModel struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	SymbolID     uint64 `gorm:"column:symbol_id;not null;uniqueIndex:uk_symbol_source"`
	SourceName   string `gorm:"column:source_name;size:64;not null;uniqueIndex:uk_symbol_source"`
	SourceSymbol string `gorm:"column:source_symbol;size:64;not null"`
	PriceScale   string `gorm:"column:price_scale;type:decimal(38,18);not null"`
	QtyScale     string `gorm:"column:qty_scale;type:decimal(38,18);not null"`
	Status       string `gorm:"column:status;size:32;not null"`
}

func (SymbolMappingModel) TableName() string { return "symbol_mappings" }

type RiskTierModel struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement"`
	SymbolID           uint64    `gorm:"column:symbol_id;not null;uniqueIndex:uk_symbol_tier"`
	TierLevel          int       `gorm:"column:tier_level;not null;uniqueIndex:uk_symbol_tier"`
	MaxNotional        string    `gorm:"column:max_notional;type:decimal(38,18);not null"`
	MaxLeverage        string    `gorm:"column:max_leverage;type:decimal(38,18);not null"`
	IMR                string    `gorm:"column:imr;type:decimal(38,18);not null"`
	MMR                string    `gorm:"column:mmr;type:decimal(38,18);not null"`
	LiquidationFeeRate string    `gorm:"column:liquidation_fee_rate;type:decimal(38,18);not null"`
	CreatedAt          time.Time `gorm:"column:created_at;not null"`
}

func (RiskTierModel) TableName() string { return "risk_tiers" }

type MarketPriceSnapshotModel struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	SymbolID   uint64    `gorm:"column:symbol_id;not null;index:idx_market_symbol_created"`
	SourceName string    `gorm:"column:source_name;size:64;not null;index:idx_market_source_ts"`
	Bid        string    `gorm:"column:bid;type:decimal(38,18);not null"`
	Ask        string    `gorm:"column:ask;type:decimal(38,18);not null"`
	Last       string    `gorm:"column:last;type:decimal(38,18);not null"`
	Mid        string    `gorm:"column:mid;type:decimal(38,18);not null"`
	SourceTS   time.Time `gorm:"column:source_ts;not null;index:idx_market_source_ts"`
	ReceivedTS time.Time `gorm:"column:received_ts;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;index:idx_market_symbol_created"`
}

func (MarketPriceSnapshotModel) TableName() string { return "market_price_snapshots" }

type MarkPriceSnapshotModel struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	SymbolID    uint64    `gorm:"column:symbol_id;not null;index:idx_mark_symbol_created"`
	IndexPrice  string    `gorm:"column:index_price;type:decimal(38,18);not null"`
	MarkPrice   string    `gorm:"column:mark_price;type:decimal(38,18);not null"`
	CalcVersion int64     `gorm:"column:calc_version;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;index:idx_mark_symbol_created"`
}

func (MarkPriceSnapshotModel) TableName() string { return "mark_price_snapshots" }

type OrderModel struct {
	ID                  uint64    `gorm:"primaryKey;autoIncrement"`
	OrderID             string    `gorm:"column:order_id;size:64;uniqueIndex;not null"`
	ClientOrderID       string    `gorm:"column:client_order_id;size:128;not null;uniqueIndex:uk_orders_user_client,priority:2"`
	UserID              uint64    `gorm:"column:user_id;not null;uniqueIndex:uk_orders_user_client,priority:1;index"`
	SymbolID            uint64    `gorm:"column:symbol_id;not null;index"`
	Side                string    `gorm:"column:side;size:16;not null"`
	PositionEffect      string    `gorm:"column:position_effect;size:16;not null"`
	Type                string    `gorm:"column:type;size:32;not null"`
	TimeInForce         string    `gorm:"column:time_in_force;size:16;not null"`
	Price               *string   `gorm:"column:price;type:decimal(38,18)"`
	TriggerPrice        *string   `gorm:"column:trigger_price;type:decimal(38,18)"`
	Qty                 string    `gorm:"column:qty;type:decimal(38,18);not null"`
	FilledQty           string    `gorm:"column:filled_qty;type:decimal(38,18);not null"`
	AvgFillPrice        string    `gorm:"column:avg_fill_price;type:decimal(38,18);not null"`
	Leverage            string    `gorm:"column:leverage;type:decimal(38,18);not null;default:1"`
	MarginMode          string    `gorm:"column:margin_mode;size:16;not null;default:CROSS"`
	ReduceOnly          bool      `gorm:"column:reduce_only;not null"`
	MaxSlippageBps      int       `gorm:"column:max_slippage_bps;not null"`
	Status              string    `gorm:"column:status;size:32;not null;index"`
	RejectReason        *string   `gorm:"column:reject_reason;size:255"`
	FrozenInitialMargin string    `gorm:"column:frozen_initial_margin;type:decimal(38,18);not null"`
	FrozenFee           string    `gorm:"column:frozen_fee;type:decimal(38,18);not null"`
	FrozenMargin        string    `gorm:"column:frozen_margin;type:decimal(38,18);not null"`
	CreatedAt           time.Time `gorm:"column:created_at;not null"`
	UpdatedAt           time.Time `gorm:"column:updated_at;not null"`
}

func (OrderModel) TableName() string { return "orders" }

type FillModel struct {
	ID                       uint64    `gorm:"primaryKey;autoIncrement"`
	FillID                   string    `gorm:"column:fill_id;size:64;uniqueIndex;not null"`
	OrderID                  string    `gorm:"column:order_id;size:64;not null;index"`
	UserID                   uint64    `gorm:"column:user_id;not null;index"`
	SymbolID                 uint64    `gorm:"column:symbol_id;not null"`
	Side                     string    `gorm:"column:side;size:16;not null"`
	Qty                      string    `gorm:"column:qty;type:decimal(38,18);not null"`
	Price                    string    `gorm:"column:price;type:decimal(38,18);not null"`
	FeeAmount                string    `gorm:"column:fee_amount;type:decimal(38,18);not null"`
	ExecutionPriceSnapshotID *uint64   `gorm:"column:execution_price_snapshot_id"`
	LedgerTxID               string    `gorm:"column:ledger_tx_id;size:64;not null"`
	CreatedAt                time.Time `gorm:"column:created_at;not null"`
}

func (FillModel) TableName() string { return "fills" }

type PositionModel struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement"`
	PositionID        string    `gorm:"column:position_id;size:64;uniqueIndex;not null"`
	UserID            uint64    `gorm:"column:user_id;not null;uniqueIndex:uk_positions_user_symbol_side_mode,priority:1"`
	SymbolID          uint64    `gorm:"column:symbol_id;not null;uniqueIndex:uk_positions_user_symbol_side_mode,priority:2"`
	Side              string    `gorm:"column:side;size:16;not null;uniqueIndex:uk_positions_user_symbol_side_mode,priority:3"`
	MarginMode        string    `gorm:"column:margin_mode;size:16;not null;default:CROSS;uniqueIndex:uk_positions_user_symbol_side_mode,priority:4"`
	Qty               string    `gorm:"column:qty;type:decimal(38,18);not null"`
	AvgEntryPrice     string    `gorm:"column:avg_entry_price;type:decimal(38,18);not null"`
	MarkPrice         string    `gorm:"column:mark_price;type:decimal(38,18);not null"`
	Notional          string    `gorm:"column:notional;type:decimal(38,18);not null"`
	Leverage          string    `gorm:"column:leverage;type:decimal(38,18);not null;default:1"`
	InitialMargin     string    `gorm:"column:initial_margin;type:decimal(38,18);not null"`
	MaintenanceMargin string    `gorm:"column:maintenance_margin;type:decimal(38,18);not null"`
	RealizedPnL       string    `gorm:"column:realized_pnl;type:decimal(38,18);not null"`
	UnrealizedPnL     string    `gorm:"column:unrealized_pnl;type:decimal(38,18);not null"`
	FundingAccrual    string    `gorm:"column:funding_accrual;type:decimal(38,18);not null"`
	LiquidationPrice  string    `gorm:"column:liquidation_price;type:decimal(38,18);not null"`
	BankruptcyPrice   string    `gorm:"column:bankruptcy_price;type:decimal(38,18);not null"`
	Status            string    `gorm:"column:status;size:32;not null"`
	UpdatedAt         time.Time `gorm:"column:updated_at;not null"`
	CreatedAt         time.Time `gorm:"column:created_at;not null"`
}

func (PositionModel) TableName() string { return "positions" }

type RiskSnapshotModel struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement"`
	UserID            uint64    `gorm:"column:user_id;not null;index:idx_risk_user_created"`
	Equity            string    `gorm:"column:equity;type:decimal(38,18);not null"`
	AvailableBalance  string    `gorm:"column:available_balance;type:decimal(38,18);not null"`
	MaintenanceMargin string    `gorm:"column:maintenance_margin;type:decimal(38,18);not null"`
	MarginRatio       string    `gorm:"column:margin_ratio;type:decimal(38,18);not null"`
	RiskLevel         string    `gorm:"column:risk_level;size:32;not null"`
	TriggeredBy       string    `gorm:"column:triggered_by;size:64;not null"`
	CreatedAt         time.Time `gorm:"column:created_at;not null"`
}

func (RiskSnapshotModel) TableName() string { return "risk_snapshots" }

type LiquidationModel struct {
	ID                    uint64    `gorm:"primaryKey;autoIncrement"`
	LiquidationID         string    `gorm:"column:liquidation_id;size:64;uniqueIndex;not null"`
	UserID                uint64    `gorm:"column:user_id;not null;index:idx_liquidations_user_status_created,priority:1"`
	SymbolID              *uint64   `gorm:"column:symbol_id"`
	Mode                  string    `gorm:"column:mode;size:32;not null"`
	Status                string    `gorm:"column:status;size:32;not null;index:idx_liquidations_user_status_created,priority:2"`
	TriggerRiskSnapshotID uint64    `gorm:"column:trigger_risk_snapshot_id;not null"`
	PenaltyAmount         string    `gorm:"column:penalty_amount;type:decimal(38,18);not null"`
	InsuranceFundUsed     string    `gorm:"column:insurance_fund_used;type:decimal(38,18);not null"`
	BankruptAmount        string    `gorm:"column:bankrupt_amount;type:decimal(38,18);not null"`
	AbortReason           *string   `gorm:"column:abort_reason;size:64"`
	PreAccountJSON        *string   `gorm:"column:pre_account_snapshot_json;type:json"`
	PostAccountJSON       *string   `gorm:"column:post_account_snapshot_json;type:json"`
	PrePositionsJSON      *string   `gorm:"column:pre_positions_snapshot_json;type:json"`
	PostPositionsJSON     *string   `gorm:"column:post_positions_snapshot_json;type:json"`
	ReleasedOrdersJSON    *string   `gorm:"column:released_orders_json;type:json"`
	PriceSnapshotJSON     *string   `gorm:"column:price_snapshot_json;type:json"`
	ConfigSnapshotJSON    *string   `gorm:"column:config_snapshot_json;type:json"`
	SettlementJSON        *string   `gorm:"column:settlement_snapshot_json;type:json"`
	CreatedAt             time.Time `gorm:"column:created_at;not null;index:idx_liquidations_user_status_created,priority:3"`
	UpdatedAt             time.Time `gorm:"column:updated_at;not null"`
}

func (LiquidationModel) TableName() string { return "liquidations" }

type LiquidationItemModel struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	LiquidationID  string    `gorm:"column:liquidation_id;size:64;not null;uniqueIndex:uk_liquidation_items_liq_pos,priority:1"`
	PositionID     string    `gorm:"column:position_id;size:64;not null;uniqueIndex:uk_liquidation_items_liq_pos,priority:2"`
	LiquidatedQty  string    `gorm:"column:liquidated_qty;type:decimal(38,18);not null"`
	ExecutionPrice string    `gorm:"column:execution_price;type:decimal(38,18);not null"`
	LedgerTxID     string    `gorm:"column:ledger_tx_id;size:64;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (LiquidationItemModel) TableName() string { return "liquidation_items" }

type FundingBatchModel struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement"`
	FundingBatchID  string     `gorm:"column:funding_batch_id;size:64;uniqueIndex;not null"`
	SymbolID        uint64     `gorm:"column:symbol_id;not null;uniqueIndex:uk_funding_symbol_window,priority:1"`
	TimeWindowStart time.Time  `gorm:"column:time_window_start;not null;uniqueIndex:uk_funding_symbol_window,priority:2"`
	TimeWindowEnd   time.Time  `gorm:"column:time_window_end;not null;uniqueIndex:uk_funding_symbol_window,priority:3"`
	NormalizedRate  string     `gorm:"column:normalized_rate;type:decimal(38,18);not null"`
	SettlementPrice string     `gorm:"column:settlement_price;type:decimal(38,18);not null"`
	Status          string     `gorm:"column:status;size:32;not null"`
	ReversedAt      *time.Time `gorm:"column:reversed_at"`
	ReversedBy      *string    `gorm:"column:reversed_by;size:64"`
	ReversalReason  *string    `gorm:"column:reversal_reason;size:255"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;not null"`
}

func (FundingBatchModel) TableName() string { return "funding_batches" }

type FundingRateSnapshotModel struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement"`
	SymbolID        uint64    `gorm:"column:symbol_id;not null;index:idx_funding_rate_symbol_source_created,priority:1"`
	SourceName      string    `gorm:"column:source_name;size:64;not null;index:idx_funding_rate_symbol_source_created,priority:2"`
	SourceSymbol    string    `gorm:"column:source_symbol;size:64;not null"`
	FundingRate     string    `gorm:"column:funding_rate;type:decimal(38,18);not null"`
	IntervalSeconds int       `gorm:"column:interval_seconds;not null"`
	SourceTS        time.Time `gorm:"column:source_ts;not null"`
	ReceivedTS      time.Time `gorm:"column:received_ts;not null"`
	CreatedAt       time.Time `gorm:"column:created_at;not null;index:idx_funding_rate_symbol_source_created,priority:3"`
}

func (FundingRateSnapshotModel) TableName() string { return "funding_rate_snapshots" }

type FundingBatchItemModel struct {
	ID                 uint64     `gorm:"primaryKey;autoIncrement"`
	FundingBatchID     string     `gorm:"column:funding_batch_id;size:64;not null;uniqueIndex:uk_batch_position,priority:1"`
	PositionID         string     `gorm:"column:position_id;size:64;not null;uniqueIndex:uk_batch_position,priority:2"`
	UserID             uint64     `gorm:"column:user_id;not null;index"`
	FundingFee         string     `gorm:"column:funding_fee;type:decimal(38,18);not null"`
	LedgerTxID         *string    `gorm:"column:ledger_tx_id;size:64"`
	ReversalLedgerTxID *string    `gorm:"column:reversal_ledger_tx_id;size:64"`
	Status             string     `gorm:"column:status;size:32;not null"`
	CreatedAt          time.Time  `gorm:"column:created_at;not null"`
	ReversedAt         *time.Time `gorm:"column:reversed_at"`
}

func (FundingBatchItemModel) TableName() string { return "funding_batch_items" }

type HedgeIntentModel struct {
	ID                 uint64    `gorm:"primaryKey;autoIncrement"`
	HedgeIntentID      string    `gorm:"column:hedge_intent_id;size:64;uniqueIndex;not null"`
	SymbolID           uint64    `gorm:"column:symbol_id;not null;index:idx_hedge_intents_symbol_status_created,priority:1"`
	Side               string    `gorm:"column:side;size:16;not null"`
	TargetQty          string    `gorm:"column:target_qty;type:decimal(38,18);not null"`
	CurrentNetExposure string    `gorm:"column:current_net_exposure;type:decimal(38,18);not null"`
	Status             string    `gorm:"column:status;size:32;not null;index:idx_hedge_intents_symbol_status_created,priority:2"`
	CreatedAt          time.Time `gorm:"column:created_at;not null;index:idx_hedge_intents_symbol_status_created,priority:3"`
	UpdatedAt          time.Time `gorm:"column:updated_at;not null"`
}

func (HedgeIntentModel) TableName() string { return "hedge_intents" }

type HedgeOrderModel struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	HedgeOrderID  string    `gorm:"column:hedge_order_id;size:64;uniqueIndex;not null"`
	HedgeIntentID string    `gorm:"column:hedge_intent_id;size:64;not null;index:idx_hedge_orders_intent_created,priority:1"`
	Venue         string    `gorm:"column:venue;size:32;not null"`
	VenueOrderID  *string   `gorm:"column:venue_order_id;size:128"`
	Symbol        string    `gorm:"column:symbol;size:64;not null"`
	Side          string    `gorm:"column:side;size:16;not null"`
	Qty           string    `gorm:"column:qty;type:decimal(38,18);not null"`
	Price         *string   `gorm:"column:price;type:decimal(38,18)"`
	Status        string    `gorm:"column:status;size:32;not null"`
	ErrorCode     *string   `gorm:"column:error_code;size:64"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;index:idx_hedge_orders_intent_created,priority:2"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null"`
}

func (HedgeOrderModel) TableName() string { return "hedge_orders" }

type HedgeFillModel struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	HedgeFillID  string    `gorm:"column:hedge_fill_id;size:64;uniqueIndex;not null"`
	HedgeOrderID string    `gorm:"column:hedge_order_id;size:64;not null;index"`
	VenueFillID  string    `gorm:"column:venue_fill_id;size:128;uniqueIndex;not null"`
	Qty          string    `gorm:"column:qty;type:decimal(38,18);not null"`
	Price        string    `gorm:"column:price;type:decimal(38,18);not null"`
	Fee          string    `gorm:"column:fee;type:decimal(38,18);not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
}

func (HedgeFillModel) TableName() string { return "hedge_fills" }

type HedgePositionModel struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement"`
	Symbol        string    `gorm:"column:symbol;size:64;not null;uniqueIndex:uk_hedge_positions_symbol_side,priority:1"`
	Side          string    `gorm:"column:side;size:16;not null;uniqueIndex:uk_hedge_positions_symbol_side,priority:2"`
	Qty           string    `gorm:"column:qty;type:decimal(38,18);not null"`
	AvgEntryPrice string    `gorm:"column:avg_entry_price;type:decimal(38,18);not null"`
	RealizedPnL   string    `gorm:"column:realized_pnl;type:decimal(38,18);not null"`
	UnrealizedPnL string    `gorm:"column:unrealized_pnl;type:decimal(38,18);not null"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null"`
}

func (HedgePositionModel) TableName() string { return "hedge_positions" }

type SystemHedgeSnapshotModel struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement"`
	SymbolID         uint64    `gorm:"column:symbol_id;not null;index:idx_system_hedge_snapshots_symbol_created,priority:1"`
	Symbol           string    `gorm:"column:symbol;size:64;not null"`
	InternalNetQty   string    `gorm:"column:internal_net_qty;type:decimal(38,18);not null"`
	TargetHedgeQty   string    `gorm:"column:target_hedge_qty;type:decimal(38,18);not null"`
	ManagedHedgeQty  string    `gorm:"column:managed_hedge_qty;type:decimal(38,18);not null"`
	ExternalHedgeQty string    `gorm:"column:external_hedge_qty;type:decimal(38,18);not null"`
	ManagedDriftQty  string    `gorm:"column:managed_drift_qty;type:decimal(38,18);not null"`
	ExternalDriftQty string    `gorm:"column:external_drift_qty;type:decimal(38,18);not null"`
	HedgeHealthy     bool      `gorm:"column:hedge_healthy;not null"`
	CreatedAt        time.Time `gorm:"column:created_at;not null;index:idx_system_hedge_snapshots_symbol_created,priority:2"`
}

func (SystemHedgeSnapshotModel) TableName() string { return "system_hedge_snapshots" }

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&UserModel{},
		&LoginNonceModel{},
		&SessionModel{},
		&AccountModel{},
		&LedgerTxModel{},
		&LedgerEntryModel{},
		&AccountBalanceSnapshotModel{},
		&LedgerAuditReportModel{},
		&OutboxEventModel{},
		&AuditLogModel{},
		&ConfigItemModel{},
		&ChainCursorModel{},
		&MessageConsumptionModel{},
		&DepositAddressModel{},
		&DepositChainTxModel{},
		&WithdrawRequestModel{},
		&SignerNonceStateModel{},
		&SymbolModel{},
		&SymbolMappingModel{},
		&RiskTierModel{},
		&MarketPriceSnapshotModel{},
		&MarkPriceSnapshotModel{},
		&OrderModel{},
		&FillModel{},
		&PositionModel{},
		&RiskSnapshotModel{},
		&LiquidationModel{},
		&LiquidationItemModel{},
		&FundingBatchModel{},
		&FundingRateSnapshotModel{},
		&FundingBatchItemModel{},
		&HedgeIntentModel{},
		&HedgeOrderModel{},
		&HedgeFillModel{},
		&HedgePositionModel{},
		&SystemHedgeSnapshotModel{},
	); err != nil {
		return err
	}
	if err := ensureDepositAddressIndexes(db); err != nil {
		return err
	}
	return ensurePositionMarginModeIndexes(db)
}

func ensureDepositAddressIndexes(db *gorm.DB) error {
	migrator := db.Migrator()
	if migrator.HasIndex(&DepositAddressModel{}, "address") {
		if err := migrator.DropIndex(&DepositAddressModel{}, "address"); err != nil {
			return err
		}
	}
	if migrator.HasIndex(&DepositAddressModel{}, "uk_deposit_addresses_chain_address") {
		if err := migrator.DropIndex(&DepositAddressModel{}, "uk_deposit_addresses_chain_address"); err != nil {
			return err
		}
	}
	if migrator.HasIndex(&DepositAddressModel{}, "uk_chain_address") {
		if err := migrator.DropIndex(&DepositAddressModel{}, "uk_chain_address"); err != nil {
			return err
		}
	}
	return migrator.CreateIndex(&DepositAddressModel{}, "uk_chain_address")
}

func ensurePositionMarginModeIndexes(db *gorm.DB) error {
	migrator := db.Migrator()
	if migrator.HasIndex(&PositionModel{}, "uk_positions_user_symbol_side") {
		if err := migrator.DropIndex(&PositionModel{}, "uk_positions_user_symbol_side"); err != nil {
			return err
		}
	}
	if migrator.HasIndex(&PositionModel{}, "uk_positions_user_symbol_side_mode") {
		if err := migrator.DropIndex(&PositionModel{}, "uk_positions_user_symbol_side_mode"); err != nil {
			return err
		}
	}
	return migrator.CreateIndex(&PositionModel{}, "uk_positions_user_symbol_side_mode")
}
