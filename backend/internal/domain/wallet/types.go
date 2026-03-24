package wallet

import "time"

type DepositChainTx struct {
	DepositID          string
	UserID             uint64
	ChainID            int64
	TxHash             string
	LogIndex           int64
	FromAddress        string
	ToAddress          string
	TokenAddress       string
	Amount             string
	Asset              string
	BlockNumber        int64
	Confirmations      int
	RequiredConfs      int
	Status             string
	CreditedLedgerTxID string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type WithdrawRequest struct {
	WithdrawID      string
	UserID          uint64
	ChainID         int64
	Asset           string
	Amount          string
	FeeAmount       string
	ToAddress       string
	Status          string
	RiskFlag        string
	HoldLedgerTxID  string
	BroadcastTxHash string
	BroadcastNonce  *uint64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type WithdrawRiskInput struct {
	UserID    uint64
	ChainID   int64
	Asset     string
	Amount    string
	FeeAmount string
	ToAddress string
}

type WithdrawDecision struct {
	Status   string
	RiskFlag string
}

type DepositAddress struct {
	UserID        uint64
	ChainID       int64
	Asset         string
	Address       string
	Status        string
	Confirmations int
	CreatedAt     time.Time
}

type TransferRequest struct {
	TransferID     string
	FromUserID     uint64
	ToUserID       uint64
	Asset          string
	Amount         string
	IdempotencyKey string
	TraceID        string
}

type DetectDepositInput struct {
	UserID         uint64
	ChainID        int64
	TxHash         string
	LogIndex       int64
	FromAddress    string
	ToAddress      string
	TokenAddress   string
	Amount         string
	Asset          string
	BlockNumber    int64
	Confirmations  int
	RequiredConfs  int
	IdempotencyKey string
	TraceID        string
}

type AdvanceDepositInput struct {
	DepositID      string
	Confirmations  int
	RequiredConfs  int
	IdempotencyKey string
	TraceID        string
}

type RequestWithdrawInput struct {
	UserID         uint64
	ChainID        int64
	Asset          string
	Amount         string
	FeeAmount      string
	ToAddress      string
	IdempotencyKey string
	TraceID        string
}

type GenerateDepositAddressInput struct {
	UserID  uint64
	ChainID int64
	Asset   string
	TraceID string
}

type ConfirmDepositInput struct {
	DepositID      string
	IdempotencyKey string
	TraceID        string
}

type ReverseDepositInput struct {
	DepositID      string
	IdempotencyKey string
	TraceID        string
}

type BroadcastWithdrawInput struct {
	WithdrawID     string
	TxHash         string
	BroadcastNonce *uint64
	IdempotencyKey string
	TraceID        string
}

type ApproveWithdrawInput struct {
	WithdrawID     string
	OperatorID     string
	IdempotencyKey string
	TraceID        string
}

type ReturnWithdrawToReviewInput struct {
	WithdrawID string
	OperatorID string
	TraceID    string
}

type FailWithdrawInput struct {
	WithdrawID string
	RiskFlag   string
	TraceID    string
}

type CompleteWithdrawInput struct {
	WithdrawID     string
	TxHash         string
	IdempotencyKey string
	TraceID        string
}

type RefundWithdrawInput struct {
	WithdrawID     string
	IdempotencyKey string
	TraceID        string
}
