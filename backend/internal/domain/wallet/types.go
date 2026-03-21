package wallet

import "time"

type DepositChainTx struct {
	DepositID          string
	UserID             uint64
	ChainID            int64
	TxHash             string
	LogIndex           int64
	Amount             string
	Asset              string
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
	HoldLedgerTxID  string
	BroadcastTxHash string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type TransferRequest struct {
	TransferID string
	FromUserID uint64
	ToUserID   uint64
	Asset      string
	Amount     string
	TraceID    string
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

type ConfirmDepositInput struct {
	DepositID      string
	IdempotencyKey string
	TraceID        string
}

type BroadcastWithdrawInput struct {
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
