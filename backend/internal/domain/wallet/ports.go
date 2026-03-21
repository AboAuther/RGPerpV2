package wallet

import (
	"context"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
)

type DepositRepository interface {
	Create(ctx context.Context, deposit DepositChainTx) error
	GetByID(ctx context.Context, depositID string) (DepositChainTx, error)
	GetByTxLog(ctx context.Context, chainID int64, txHash string, logIndex int64) (DepositChainTx, error)
	ListPendingByChain(ctx context.Context, chainID int64, statuses []string, limit int) ([]DepositChainTx, error)
	UpdateConfirmations(ctx context.Context, depositID string, confirmations int, status string) error
	MarkCredited(ctx context.Context, depositID string, ledgerTxID string) error
	MarkReorgReversed(ctx context.Context, depositID string) error
	ListByUser(ctx context.Context, userID uint64) ([]DepositChainTx, error)
}

type WithdrawRepository interface {
	Create(ctx context.Context, withdraw WithdrawRequest) error
	GetByID(ctx context.Context, withdrawID string) (WithdrawRequest, error)
	UpdateStatus(ctx context.Context, withdrawID string, from []string, to string) error
	MarkBroadcasted(ctx context.Context, withdrawID string, txHash string) error
	MarkCompleted(ctx context.Context, withdrawID string) error
	MarkRefunded(ctx context.Context, withdrawID string) error
	ListByUser(ctx context.Context, userID uint64) ([]WithdrawRequest, error)
}

type TransferResolver interface {
	ResolveUserIDByAddress(ctx context.Context, address string) (uint64, error)
}

type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type Ledger interface {
	Post(ctx context.Context, req ledgerdomain.PostingRequest) error
}

type BalanceRepository interface {
	GetAccountBalanceForUpdate(ctx context.Context, accountID uint64, asset string) (string, error)
}

type DepositAddressRepository interface {
	ListByUser(ctx context.Context, userID uint64) ([]DepositAddress, error)
	GetByUserChainAsset(ctx context.Context, userID uint64, chainID int64, asset string) (DepositAddress, error)
	Upsert(ctx context.Context, address DepositAddress) error
}

type DepositAddressAllocator interface {
	Allocate(ctx context.Context, userID uint64, chainID int64, asset string) (string, error)
}
