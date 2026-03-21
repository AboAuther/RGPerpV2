package wallet

import (
	"context"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
)

type DepositRepository interface {
	GetByID(ctx context.Context, depositID string) (DepositChainTx, error)
	MarkCredited(ctx context.Context, depositID string, ledgerTxID string) error
}

type WithdrawRepository interface {
	Create(ctx context.Context, withdraw WithdrawRequest) error
	GetByID(ctx context.Context, withdrawID string) (WithdrawRequest, error)
	MarkBroadcasted(ctx context.Context, withdrawID string, txHash string) error
	MarkRefunded(ctx context.Context, withdrawID string) error
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
