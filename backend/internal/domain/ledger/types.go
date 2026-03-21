package ledger

import "time"

type AccountSnapshot struct {
	AccountID uint64
	Asset     string
	Balance   string
	Version   int64
	UpdatedAt time.Time
}

type LedgerTx struct {
	ID             string
	EventID        string
	BizType        string
	BizRefID       string
	Asset          string
	IdempotencyKey string
	OperatorType   string
	OperatorID     string
	TraceID        string
	Status         string
	CreatedAt      time.Time
}

type LedgerEntry struct {
	AccountID uint64
	UserID    *uint64
	Asset     string
	Amount    string
	EntryType string
}

type PostingRequest struct {
	LedgerTx LedgerTx
	Entries  []LedgerEntry
}
