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
	GetAccountBalancesForUpdate(ctx context.Context, accountIDs []uint64, asset string) (map[uint64]string, error)
}

type LedgerPoster interface {
	Post(ctx context.Context, req ledgerdomain.PostingRequest) error
}

type MarketRepository interface {
	GetTradableSymbol(ctx context.Context, symbol string) (TradableSymbol, error)
}

type PostTradeRiskProcessor interface {
	RecalculateAfterTrade(ctx context.Context, userID uint64, traceID string) error
}

type RuntimeConfig struct {
	GlobalReadOnly                bool
	GlobalReduceOnly              bool
	MaxMarketDataAge              time.Duration
	MaxOpenOrdersPerUserPerSymbol int
	NetExposureHardLimit          string
	MaxExposureSlippageBps        int
	TakerFeeRate                  string
	MakerFeeRate                  string
	DefaultMaxSlippageBps         int
	MaxLeverage                   string
	SessionPolicy                 string
	LiquidationPenaltyRate        string
	LiquidationExtraSlippageBps   int
	MaintenanceMarginUpliftRatio  string
}

type RuntimeConfigProvider interface {
	CurrentOrderRuntimeConfig(symbol string) RuntimeConfig
}

type OrderRepository interface {
	GetByUserClientOrderID(ctx context.Context, userID uint64, clientOrderID string) (Order, error)
	GetByUserOrderIDForUpdate(ctx context.Context, userID uint64, orderID string) (Order, error)
	CountActiveOrdersForUserSymbol(ctx context.Context, userID uint64, symbolID uint64) (int, error)
	ListRestingOpenLimitOrders(ctx context.Context, limit int) ([]Order, error)
	ListTriggerWaitingOrders(ctx context.Context, limit int) ([]Order, error)
	LockSymbolForUpdate(ctx context.Context, symbolID uint64) error
	GetLatestRiskLevelForUpdate(ctx context.Context, userID uint64) (string, error)
	GetSymbolExposureForUpdate(ctx context.Context, symbolID uint64) (SymbolExposure, error)
	CreateOrder(ctx context.Context, order Order) error
	UpdateOrder(ctx context.Context, order Order) error
	CreateFill(ctx context.Context, fill Fill) error
	CreateEvent(ctx context.Context, event Event) error
	GetPositionForUpdate(ctx context.Context, userID uint64, symbolID uint64, side string, marginMode string) (Position, error)
	UpsertPosition(ctx context.Context, position Position) error
}
