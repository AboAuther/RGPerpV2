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
	)
}
