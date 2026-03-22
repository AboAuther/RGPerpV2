package order

import (
	"context"
	"errors"
	"testing"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeIDGen struct {
	values []string
	idx    int
}

func (f *fakeIDGen) NewID(_ string) string {
	v := f.values[f.idx]
	f.idx++
	return v
}

type stubTxManager struct{}

func (stubTxManager) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

type stubAccounts struct{}

func (stubAccounts) ResolveTradeAccounts(_ context.Context, userID uint64, _ string) (TradeAccounts, error) {
	return TradeAccounts{
		UserWalletAccountID:         userID*10 + 1,
		UserOrderMarginAccountID:    userID*10 + 2,
		UserPositionMarginAccountID: userID*10 + 3,
		SystemPoolAccountID:         9000,
		TradingFeeAccountID:         9001,
	}, nil
}

type stubBalances struct{ balance string }

func (s stubBalances) GetAccountBalanceForUpdate(_ context.Context, _ uint64, _ string) (string, error) {
	return s.balance, nil
}

type stubLedger struct {
	reqs []ledgerdomain.PostingRequest
}

func (s *stubLedger) Post(_ context.Context, req ledgerdomain.PostingRequest) error {
	s.reqs = append(s.reqs, req)
	return nil
}

type stubMarketRepo struct {
	symbol TradableSymbol
	err    error
}

func (s stubMarketRepo) GetTradableSymbol(_ context.Context, _ string) (TradableSymbol, error) {
	if s.symbol.SnapshotTS.IsZero() {
		s.symbol.SnapshotTS = time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)
	}
	return s.symbol, s.err
}

func testServiceConfig() ServiceConfig {
	return ServiceConfig{
		Asset:                 "USDC",
		TakerFeeRate:          "0.0006",
		MakerFeeRate:          "0.0002",
		DefaultMaxSlippageBps: 100,
		MaxMarketDataAge:      time.Hour,
	}
}

type stubOrderRepo struct {
	byClient  map[string]Order
	byOrderID map[string]Order
	position  Position
	fills     []Fill
}

func newStubOrderRepo() *stubOrderRepo {
	return &stubOrderRepo{
		byClient:  map[string]Order{},
		byOrderID: map[string]Order{},
	}
}

func (s *stubOrderRepo) GetByUserClientOrderID(_ context.Context, _ uint64, clientOrderID string) (Order, error) {
	order, ok := s.byClient[clientOrderID]
	if !ok {
		return Order{}, errorsx.ErrNotFound
	}
	return order, nil
}

func (s *stubOrderRepo) GetByUserOrderIDForUpdate(_ context.Context, _ uint64, orderID string) (Order, error) {
	order, ok := s.byOrderID[orderID]
	if !ok {
		return Order{}, errorsx.ErrNotFound
	}
	return order, nil
}

func (s *stubOrderRepo) CreateOrder(_ context.Context, order Order) error {
	s.byClient[order.ClientOrderID] = order
	s.byOrderID[order.OrderID] = order
	return nil
}

func (s *stubOrderRepo) UpdateOrder(_ context.Context, order Order) error {
	s.byClient[order.ClientOrderID] = order
	s.byOrderID[order.OrderID] = order
	return nil
}

func (s *stubOrderRepo) CreateFill(_ context.Context, fill Fill) error {
	s.fills = append(s.fills, fill)
	return nil
}

func (s *stubOrderRepo) GetPositionForUpdate(_ context.Context, _ uint64, _ uint64, _ string) (Position, error) {
	if s.position.PositionID == "" {
		return Position{}, errorsx.ErrNotFound
	}
	return s.position, nil
}

func (s *stubOrderRepo) UpsertPosition(_ context.Context, position Position) error {
	s.position = position
	return nil
}

func TestCreateOrder_MarketOpenFillsAndAllocatesMargin(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1", "ldg_hold", "evt_hold", "ldg_fill", "evt_fill", "fill_1", "pos_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              1,
			Symbol:                "BTC-PERP",
			ContractMultiplier:    "1",
			TickSize:              "0.1",
			StepSize:              "0.001",
			MinNotional:           "10",
			Status:                "TRADING",
			IndexPrice:            "100",
			MarkPrice:             "100",
			BestBid:               "99",
			BestAsk:               "101",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_1",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "1",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Status != OrderStatusFilled {
		t.Fatalf("expected filled order, got %s", order.Status)
	}
	if len(ledger.reqs) != 2 {
		t.Fatalf("expected 2 ledger postings, got %d", len(ledger.reqs))
	}
	if repo.position.Side != PositionSideLong || repo.position.Qty != "1" {
		t.Fatalf("unexpected position: %+v", repo.position)
	}
	if len(repo.fills) != 1 || repo.fills[0].Price != "101" {
		t.Fatalf("unexpected fills: %+v", repo.fills)
	}
}

func TestCreateOrder_LimitOpenRestsWithFrozenMargin(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1", "ldg_hold", "evt_hold"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              1,
			Symbol:                "BTC-PERP",
			ContractMultiplier:    "1",
			TickSize:              "0.1",
			StepSize:              "0.001",
			MinNotional:           "10",
			Status:                "TRADING",
			IndexPrice:            "100",
			MarkPrice:             "100",
			BestBid:               "99",
			BestAsk:               "101",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	price := "100"
	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_1",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "LIMIT",
		Qty:            "1",
		Price:          &price,
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Status != OrderStatusResting {
		t.Fatalf("expected resting order, got %s", order.Status)
	}
	if len(ledger.reqs) != 1 {
		t.Fatalf("expected 1 ledger posting, got %d", len(ledger.reqs))
	}
	if repo.byOrderID[order.OrderID].FrozenMargin == "0" {
		t.Fatalf("expected frozen margin to remain on resting order")
	}
}

func TestCancelOrder_ReleasesFrozenMargin(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byOrderID["ord_1"] = Order{
		OrderID:       "ord_1",
		ClientOrderID: "cli_1",
		UserID:        7,
		Status:        OrderStatusResting,
		FrozenMargin:  "10.06",
	}
	repo.byClient["cli_1"] = repo.byOrderID["ord_1"]
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ldg_release", "evt_release"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := svc.CancelOrder(context.Background(), CancelOrderInput{
		UserID:         7,
		OrderID:        "ord_1",
		IdempotencyKey: "idem_cancel",
		TraceID:        "trace_cancel",
	}); err != nil {
		t.Fatalf("cancel order: %v", err)
	}
	if len(ledger.reqs) != 1 {
		t.Fatalf("expected 1 ledger release posting, got %d", len(ledger.reqs))
	}
	if repo.byOrderID["ord_1"].Status != OrderStatusCanceled {
		t.Fatalf("expected canceled order, got %+v", repo.byOrderID["ord_1"])
	}
}

func TestCreateOrder_DuplicateClientOrderReturnsExisting(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byClient["cli_1"] = Order{OrderID: "ord_existing", ClientOrderID: "cli_1", Status: OrderStatusResting}
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"unused"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_1",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "1",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.OrderID != "ord_existing" {
		t.Fatalf("expected existing order, got %+v", order)
	}
	if len(ledger.reqs) != 0 {
		t.Fatalf("expected no new ledger postings on duplicate request")
	}
}

func TestCreateOrder_InsufficientBalanceFailsClosed(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1"},
		ledger,
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              1,
			Symbol:                "BTC-PERP",
			ContractMultiplier:    "1",
			TickSize:              "0.1",
			StepSize:              "0.001",
			MinNotional:           "10",
			Status:                "TRADING",
			IndexPrice:            "100",
			MarkPrice:             "100",
			BestBid:               "99",
			BestAsk:               "101",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_1",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "1",
		IdempotencyKey: "idem_1",
		TraceID:        "trace_1",
	})
	if err == nil {
		t.Fatalf("expected insufficient balance error")
	}
	if len(ledger.reqs) != 0 {
		t.Fatalf("expected no ledger postings on rejected order")
	}
}

func TestCreateOrder_CloseLongRealizesProfit(t *testing.T) {
	repo := newStubOrderRepo()
	repo.position = Position{
		PositionID:        "pos_1",
		UserID:            7,
		SymbolID:          1,
		Side:              PositionSideLong,
		Qty:               "1",
		AvgEntryPrice:     "100",
		MarkPrice:         "120",
		Notional:          "120",
		InitialMargin:     "10",
		MaintenanceMargin: "5",
		RealizedPnL:       "0",
		UnrealizedPnL:     "20",
		FundingAccrual:    "0",
		LiquidationPrice:  "0",
		BankruptcyPrice:   "0",
		Status:            PositionStatusOpen,
		CreatedAt:         time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1", "ldg_fill", "evt_fill", "fill_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              1,
			Symbol:                "BTC-PERP",
			ContractMultiplier:    "1",
			TickSize:              "0.1",
			StepSize:              "0.001",
			MinNotional:           "10",
			Status:                "TRADING",
			IndexPrice:            "120",
			MarkPrice:             "120",
			BestBid:               "119",
			BestAsk:               "121",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_close_1",
		Symbol:         "BTC-PERP",
		Side:           "SELL",
		PositionEffect: "CLOSE",
		Type:           "MARKET",
		Qty:            "0.4",
		IdempotencyKey: "idem_close_1",
		TraceID:        "trace_close_1",
	})
	if err != nil {
		t.Fatalf("close order: %v", err)
	}
	if order.Status != OrderStatusFilled || order.FilledQty != "0.4" {
		t.Fatalf("unexpected close order: %+v", order)
	}
	if len(ledger.reqs) != 1 {
		t.Fatalf("expected a single close settlement posting, got %d", len(ledger.reqs))
	}
	if repo.position.Qty != "0.6" || repo.position.RealizedPnL != "7.6" {
		t.Fatalf("unexpected updated position: %+v", repo.position)
	}
}

func TestCreateOrder_ReduceOnlyWithoutPositionFailsClosed(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              1,
			Symbol:                "BTC-PERP",
			ContractMultiplier:    "1",
			TickSize:              "0.1",
			StepSize:              "0.001",
			MinNotional:           "10",
			Status:                "TRADING",
			IndexPrice:            "120",
			MarkPrice:             "120",
			BestBid:               "119",
			BestAsk:               "121",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_reduce_1",
		Symbol:         "BTC-PERP",
		Side:           "SELL",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "0.4",
		ReduceOnly:     true,
		IdempotencyKey: "idem_reduce_1",
		TraceID:        "trace_reduce_1",
	})
	if err == nil {
		t.Fatalf("expected reduce-only without position to fail")
	}
	if len(ledger.reqs) != 0 {
		t.Fatalf("expected no ledger write on rejected reduce-only")
	}
}

func TestCreateOrder_OpenForbiddenWhenSymbolIsReduceOnly(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 2, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              1,
			Symbol:                "BTC-PERP",
			ContractMultiplier:    "1",
			TickSize:              "0.1",
			StepSize:              "0.001",
			MinNotional:           "10",
			Status:                "REDUCE_ONLY",
			IndexPrice:            "120",
			MarkPrice:             "120",
			BestBid:               "119",
			BestAsk:               "121",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_reduce_only_open",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "0.4",
		IdempotencyKey: "idem_reduce_only_open",
		TraceID:        "trace_reduce_only_open",
	})
	if err == nil {
		t.Fatalf("expected open order on reduce-only symbol to fail")
	}
	if len(ledger.reqs) != 0 {
		t.Fatalf("expected no ledger write on rejected open order")
	}
}

func TestCreateOrder_CloseAllowedWhenSymbolIsReduceOnly(t *testing.T) {
	repo := newStubOrderRepo()
	repo.position = Position{
		PositionID:        "pos_1",
		UserID:            7,
		SymbolID:          1,
		Side:              PositionSideLong,
		Qty:               "1",
		AvgEntryPrice:     "100",
		MarkPrice:         "120",
		Notional:          "120",
		InitialMargin:     "10",
		MaintenanceMargin: "5",
		RealizedPnL:       "0",
		UnrealizedPnL:     "20",
		FundingAccrual:    "0",
		LiquidationPrice:  "0",
		BankruptcyPrice:   "0",
		Status:            PositionStatusOpen,
		CreatedAt:         time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 3, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1", "ldg_fill", "evt_fill", "fill_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              1,
			Symbol:                "BTC-PERP",
			ContractMultiplier:    "1",
			TickSize:              "0.1",
			StepSize:              "0.001",
			MinNotional:           "10",
			Status:                "REDUCE_ONLY",
			IndexPrice:            "120",
			MarkPrice:             "120",
			BestBid:               "119",
			BestAsk:               "121",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_reduce_only_close",
		Symbol:         "BTC-PERP",
		Side:           "SELL",
		PositionEffect: "CLOSE",
		Type:           "MARKET",
		Qty:            "0.4",
		IdempotencyKey: "idem_reduce_only_close",
		TraceID:        "trace_reduce_only_close",
	})
	if err != nil {
		t.Fatalf("expected close order on reduce-only symbol to succeed: %v", err)
	}
	if order.Status != OrderStatusFilled {
		t.Fatalf("expected filled close order, got %+v", order)
	}
}

func TestCreateOrder_OpenForbiddenWhenMarketDataIsStale(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	cfg := testServiceConfig()
	cfg.MaxMarketDataAge = time.Minute
	svc, err := NewService(
		cfg,
		fakeClock{now: time.Date(2026, 3, 22, 1, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              1,
			Symbol:                "BTC-PERP",
			ContractMultiplier:    "1",
			TickSize:              "0.1",
			StepSize:              "0.001",
			MinNotional:           "10",
			Status:                "TRADING",
			IndexPrice:            "100",
			MarkPrice:             "100",
			BestBid:               "99",
			BestAsk:               "101",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
			SnapshotTS:            time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_stale_open",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "1",
		IdempotencyKey: "idem_stale_open",
		TraceID:        "trace_stale_open",
	})
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("expected stale market data to forbid open, got %v", err)
	}
}
