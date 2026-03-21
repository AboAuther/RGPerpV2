package indexer

import "time"

const (
	CursorTypeRouterScan   = "router_created_scan"
	CursorTypeDepositScan  = "deposit_forwarded_scan"
	CursorTypeWithdrawScan = "withdraw_executed_scan"
)

type ChainRule struct {
	ChainID                int64
	Asset                  string
	RequiredConfirmations  int
	VaultAddress           string
	TokenAddress           string
	FactoryAddress         string
}

type RouterCreated struct {
	ChainID        int64
	UserID         uint64
	RouterAddress  string
	FactoryAddress string
	Salt           string
	TxHash         string
	LogIndex       int64
	BlockNumber    int64
	ObservedAt     time.Time
	TraceID        string
}

type DepositObserved struct {
	ChainID        int64
	UserID         uint64
	TxHash         string
	LogIndex       int64
	BlockNumber    int64
	Confirmations  int
	RouterAddress  string
	VaultAddress   string
	TokenAddress   string
	FromAddress    string
	Amount         string
	Removed        bool
	TraceID        string
	ObservedAt     time.Time
}

type WithdrawExecuted struct {
	ChainID      int64
	WithdrawID   string
	TxHash       string
	LogIndex     int64
	BlockNumber  int64
	VaultAddress string
	TokenAddress string
	ToAddress    string
	Amount       string
	Operator     string
	TraceID      string
	ObservedAt   time.Time
}

type WithdrawFailed struct {
	ChainID    int64
	WithdrawID string
	TxHash     string
	Reason     string
	TraceID    string
	ObservedAt time.Time
}

type ReceiptStatus struct {
	Found       bool
	Success     bool
	BlockNumber int64
}

type EventEnvelope struct {
	EventID       string         `json:"event_id"`
	EventType     string         `json:"event_type"`
	AggregateType string         `json:"aggregate_type"`
	AggregateID   string         `json:"aggregate_id"`
	TraceID       string         `json:"trace_id"`
	Producer      string         `json:"producer"`
	Version       int            `json:"version"`
	OccurredAt    time.Time      `json:"occurred_at"`
	Payload       map[string]any `json:"payload"`
}
