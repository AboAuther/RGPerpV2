package order

import (
	"context"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
)

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID(prefix string) string
}

type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type TradeAccounts struct {
	UserWalletAccountID         uint64
	UserOrderMarginAccountID    uint64
	UserPositionMarginAccountID uint64
	SystemPoolAccountID         uint64
	TradingFeeAccountID         uint64
}

type AccountResolver interface {
	ResolveTradeAccounts(ctx context.Context, userID uint64, asset string) (TradeAccounts, error)
}

type BalanceRepository interface {
	GetAccountBalanceForUpdate(ctx context.Context, accountID uint64, asset string) (string, error)
}

type LedgerPoster interface {
	Post(ctx context.Context, req ledgerdomain.PostingRequest) error
}

type MarketRepository interface {
	GetTradableSymbol(ctx context.Context, symbol string) (TradableSymbol, error)
}

type OrderRepository interface {
	GetByUserClientOrderID(ctx context.Context, userID uint64, clientOrderID string) (Order, error)
	GetByUserOrderIDForUpdate(ctx context.Context, userID uint64, orderID string) (Order, error)
	CreateOrder(ctx context.Context, order Order) error
	UpdateOrder(ctx context.Context, order Order) error
	CreateFill(ctx context.Context, fill Fill) error
	GetPositionForUpdate(ctx context.Context, userID uint64, symbolID uint64, side string) (Position, error)
	UpsertPosition(ctx context.Context, position Position) error
}
