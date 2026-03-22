package indexer

import (
	"context"
	"time"

	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
)

type Wallet interface {
	DetectDeposit(ctx context.Context, input walletdomain.DetectDepositInput) (walletdomain.DepositChainTx, error)
	AdvanceDeposit(ctx context.Context, input walletdomain.AdvanceDepositInput) error
	ConfirmDeposit(ctx context.Context, input walletdomain.ConfirmDepositInput) error
	ReverseDeposit(ctx context.Context, input walletdomain.ReverseDepositInput) error
	MarkWithdrawBroadcasted(ctx context.Context, input walletdomain.BroadcastWithdrawInput) error
	CompleteWithdraw(ctx context.Context, input walletdomain.CompleteWithdrawInput) error
	RefundWithdraw(ctx context.Context, input walletdomain.RefundWithdrawInput) error
}

type DepositRepository interface {
	GetByID(ctx context.Context, depositID string) (walletdomain.DepositChainTx, error)
	GetByTxLog(ctx context.Context, chainID int64, txHash string, logIndex int64) (walletdomain.DepositChainTx, error)
	ListPendingByChain(ctx context.Context, chainID int64, statuses []string, limit int) ([]walletdomain.DepositChainTx, error)
}

type WithdrawRepository interface {
	GetByID(ctx context.Context, withdrawID string) (walletdomain.WithdrawRequest, error)
	ListByChainStatuses(ctx context.Context, chainID int64, statuses []string, limit int) ([]walletdomain.WithdrawRequest, error)
	UpdateStatus(ctx context.Context, withdrawID string, from []string, to string) error
}

type DepositAddressResolver interface {
	GetByChainAddress(ctx context.Context, chainID int64, address string) (walletdomain.DepositAddress, error)
	AssignToUser(ctx context.Context, userID uint64, chainID int64, asset string, address string) error
}

type EventPublisher interface {
	Publish(ctx context.Context, envelope EventEnvelope) error
}

type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID(prefix string) string
}

type CursorRepository interface {
	Get(ctx context.Context, chainID int64, cursorType string) (Cursor, error)
	Upsert(ctx context.Context, chainID int64, cursorType string, cursorValue string, updatedAt time.Time) error
}

type Cursor struct {
	ChainID     int64
	CursorType  string
	CursorValue string
	UpdatedAt   time.Time
}

type EventSource interface {
	LatestBlockNumber(ctx context.Context, chainID int64) (int64, error)
	BlockHash(ctx context.Context, chainID int64, blockNumber int64) (string, error)
	ListRouterCreatedEvents(ctx context.Context, chainID int64, fromBlock int64, toBlock int64) ([]RouterCreated, error)
	ListDepositEvents(ctx context.Context, chainID int64, fromBlock int64, toBlock int64) ([]DepositObserved, error)
	ListWithdrawEvents(ctx context.Context, chainID int64, fromBlock int64, toBlock int64) ([]WithdrawExecuted, error)
	GetReceiptStatus(ctx context.Context, chainID int64, txHash string) (ReceiptStatus, error)
}
