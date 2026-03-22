package db

import (
	"time"

	"gorm.io/gorm"
)

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

type DepositAddressModel struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `gorm:"column:user_id;not null;uniqueIndex:uk_user_chain_asset;index"`
	ChainID   int64     `gorm:"column:chain_id;not null;uniqueIndex:uk_user_chain_asset"`
	Address   string    `gorm:"column:address;size:64;not null;uniqueIndex:uk_chain_address"`
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
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	OrderID        string    `gorm:"column:order_id;size:64;uniqueIndex;not null"`
	ClientOrderID  string    `gorm:"column:client_order_id;size:128;not null;uniqueIndex:uk_orders_user_client,priority:2"`
	UserID         uint64    `gorm:"column:user_id;not null;uniqueIndex:uk_orders_user_client,priority:1;index"`
	SymbolID       uint64    `gorm:"column:symbol_id;not null;index"`
	Side           string    `gorm:"column:side;size:16;not null"`
	PositionEffect string    `gorm:"column:position_effect;size:16;not null"`
	Type           string    `gorm:"column:type;size:32;not null"`
	TimeInForce    string    `gorm:"column:time_in_force;size:16;not null"`
	Price          *string   `gorm:"column:price;type:decimal(38,18)"`
	TriggerPrice   *string   `gorm:"column:trigger_price;type:decimal(38,18)"`
	Qty            string    `gorm:"column:qty;type:decimal(38,18);not null"`
	FilledQty      string    `gorm:"column:filled_qty;type:decimal(38,18);not null"`
	AvgFillPrice   string    `gorm:"column:avg_fill_price;type:decimal(38,18);not null"`
	ReduceOnly     bool      `gorm:"column:reduce_only;not null"`
	MaxSlippageBps int       `gorm:"column:max_slippage_bps;not null"`
	Status         string    `gorm:"column:status;size:32;not null;index"`
	RejectReason   *string   `gorm:"column:reject_reason;size:255"`
	FrozenMargin   string    `gorm:"column:frozen_margin;type:decimal(38,18);not null"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
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
	UserID            uint64    `gorm:"column:user_id;not null;uniqueIndex:uk_positions_user_symbol_side,priority:1"`
	SymbolID          uint64    `gorm:"column:symbol_id;not null;uniqueIndex:uk_positions_user_symbol_side,priority:2"`
	Side              string    `gorm:"column:side;size:16;not null;uniqueIndex:uk_positions_user_symbol_side,priority:3"`
	Qty               string    `gorm:"column:qty;type:decimal(38,18);not null"`
	AvgEntryPrice     string    `gorm:"column:avg_entry_price;type:decimal(38,18);not null"`
	MarkPrice         string    `gorm:"column:mark_price;type:decimal(38,18);not null"`
	Notional          string    `gorm:"column:notional;type:decimal(38,18);not null"`
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

type FundingBatchModel struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement"`
	FundingBatchID  string    `gorm:"column:funding_batch_id;size:64;uniqueIndex;not null"`
	SymbolID        uint64    `gorm:"column:symbol_id;not null;uniqueIndex:uk_funding_symbol_window,priority:1"`
	TimeWindowStart time.Time `gorm:"column:time_window_start;not null;uniqueIndex:uk_funding_symbol_window,priority:2"`
	TimeWindowEnd   time.Time `gorm:"column:time_window_end;not null;uniqueIndex:uk_funding_symbol_window,priority:3"`
	NormalizedRate  string    `gorm:"column:normalized_rate;type:decimal(38,18);not null"`
	SettlementPrice string    `gorm:"column:settlement_price;type:decimal(38,18);not null"`
	Status          string    `gorm:"column:status;size:32;not null"`
	CreatedAt       time.Time `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null"`
}

func (FundingBatchModel) TableName() string { return "funding_batches" }

type FundingBatchItemModel struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	FundingBatchID string    `gorm:"column:funding_batch_id;size:64;not null;uniqueIndex:uk_batch_position,priority:1"`
	PositionID     string    `gorm:"column:position_id;size:64;not null;uniqueIndex:uk_batch_position,priority:2"`
	UserID         uint64    `gorm:"column:user_id;not null;index"`
	FundingFee     string    `gorm:"column:funding_fee;type:decimal(38,18);not null"`
	LedgerTxID     *string   `gorm:"column:ledger_tx_id;size:64"`
	Status         string    `gorm:"column:status;size:32;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (FundingBatchItemModel) TableName() string { return "funding_batch_items" }

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&UserModel{},
		&LoginNonceModel{},
		&SessionModel{},
		&AccountModel{},
		&LedgerTxModel{},
		&LedgerEntryModel{},
		&AccountBalanceSnapshotModel{},
		&OutboxEventModel{},
		&ChainCursorModel{},
		&MessageConsumptionModel{},
		&DepositAddressModel{},
		&DepositChainTxModel{},
		&WithdrawRequestModel{},
		&SymbolModel{},
		&SymbolMappingModel{},
		&RiskTierModel{},
		&MarketPriceSnapshotModel{},
		&MarkPriceSnapshotModel{},
		&OrderModel{},
		&FillModel{},
		&PositionModel{},
		&FundingBatchModel{},
		&FundingBatchItemModel{},
	)
}
