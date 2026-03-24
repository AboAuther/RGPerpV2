package order

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeIDGen struct {
	values []string
	idx    int
}

func (f *fakeIDGen) NewID(_ string) string {
	if f.idx >= len(f.values) {
		v := fmt.Sprintf("auto_%d", f.idx)
		f.idx++
		return v
	}
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

func (s stubBalances) GetAccountBalancesForUpdate(_ context.Context, accountIDs []uint64, _ string) (map[uint64]string, error) {
	out := make(map[uint64]string, len(accountIDs))
	for _, accountID := range accountIDs {
		out[accountID] = s.balance
	}
	return out, nil
}

type stubLedger struct {
	reqs []ledgerdomain.PostingRequest
}

func (s *stubLedger) Post(_ context.Context, req ledgerdomain.PostingRequest) error {
	s.reqs = append(s.reqs, req)
	return nil
}

type stubPostTradeRisk struct {
	userIDs []uint64
	traces  []string
}

func (s *stubPostTradeRisk) RecalculateAfterTrade(_ context.Context, userID uint64, traceID string) error {
	s.userIDs = append(s.userIDs, userID)
	s.traces = append(s.traces, traceID)
	return nil
}

type trackingBalances struct {
	values map[uint64]string
}

func (b *trackingBalances) GetAccountBalanceForUpdate(_ context.Context, accountID uint64, _ string) (string, error) {
	if b.values == nil {
		return "0", nil
	}
	if value, ok := b.values[accountID]; ok {
		return value, nil
	}
	return "0", nil
}

func (b *trackingBalances) GetAccountBalancesForUpdate(_ context.Context, accountIDs []uint64, _ string) (map[uint64]string, error) {
	out := make(map[uint64]string, len(accountIDs))
	for _, accountID := range accountIDs {
		if b.values == nil {
			out[accountID] = "0"
			continue
		}
		if value, ok := b.values[accountID]; ok {
			out[accountID] = value
			continue
		}
		out[accountID] = "0"
	}
	return out, nil
}

func (b *trackingBalances) apply(entry ledgerdomain.LedgerEntry) {
	current := decimalx.MustFromString("0")
	if b.values != nil {
		if raw, ok := b.values[entry.AccountID]; ok {
			current = decimalx.MustFromString(raw)
		}
	} else {
		b.values = map[uint64]string{}
	}
	b.values[entry.AccountID] = current.Add(decimalx.MustFromString(entry.Amount)).String()
}

type applyingLedger struct {
	reqs     []ledgerdomain.PostingRequest
	balances *trackingBalances
}

func (s *applyingLedger) Post(_ context.Context, req ledgerdomain.PostingRequest) error {
	s.reqs = append(s.reqs, req)
	if s.balances != nil {
		for _, entry := range req.Entries {
			s.balances.apply(entry)
		}
	}
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

type stubOrderRuntimeProvider struct {
	bySymbol map[string]RuntimeConfig
}

func (s stubOrderRuntimeProvider) CurrentOrderRuntimeConfig(symbol string) RuntimeConfig {
	if cfg, ok := s.bySymbol[symbol]; ok {
		return cfg
	}
	return RuntimeConfig{}
}

func testServiceConfig() ServiceConfig {
	return ServiceConfig{
		Asset:                  "USDC",
		TakerFeeRate:           "0.0006",
		MakerFeeRate:           "0.0002",
		DefaultMaxSlippageBps:  100,
		MaxMarketDataAge:       time.Hour,
		NetExposureHardLimit:   "250",
		MaxExposureSlippageBps: 40,
	}
}

func testTradableSymbol(ts time.Time) TradableSymbol {
	return TradableSymbol{
		SymbolID:              1,
		Symbol:                "BTC-PERP",
		ContractMultiplier:    "1",
		TickSize:              "0.1",
		StepSize:              "0.001",
		MinNotional:           "10",
		Status:                "TRADING",
		SessionPolicy:         "ALWAYS_OPEN",
		IndexPrice:            "100",
		MarkPrice:             "100",
		BestBid:               "99",
		BestAsk:               "101",
		InitialMarginRate:     "0.1",
		MaintenanceMarginRate: "0.05",
		SnapshotTS:            ts,
	}
}

type stubOrderRepo struct {
	byClient        map[string]Order
	byOrderID       map[string]Order
	position        Position
	exposure        SymbolExposure
	latestRiskLevel string
	fills           []Fill
	events          []Event
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

func (s *stubOrderRepo) CountActiveOrdersForUserSymbol(_ context.Context, userID uint64, symbolID uint64) (int, error) {
	count := 0
	for _, order := range s.byOrderID {
		if order.UserID != userID || order.SymbolID != symbolID {
			continue
		}
		if order.Status == OrderStatusResting || order.Status == OrderStatusTriggerWait {
			count++
		}
	}
	return count, nil
}

func (s *stubOrderRepo) ListRestingOpenLimitOrders(_ context.Context, limit int) ([]Order, error) {
	out := make([]Order, 0, len(s.byOrderID))
	for _, order := range s.byOrderID {
		if order.Status == OrderStatusResting && order.Type == OrderTypeLimit && order.PositionEffect == PositionEffectOpen {
			out = append(out, order)
		}
	}
	if len(out) > limit && limit > 0 {
		out = out[:limit]
	}
	return out, nil
}

func (s *stubOrderRepo) ListTriggerWaitingOrders(_ context.Context, limit int) ([]Order, error) {
	out := make([]Order, 0, len(s.byOrderID))
	for _, order := range s.byOrderID {
		if order.Status == OrderStatusTriggerWait && isTriggerOrderType(order.Type) {
			out = append(out, order)
		}
	}
	if len(out) > limit && limit > 0 {
		out = out[:limit]
	}
	return out, nil
}

func (s *stubOrderRepo) LockSymbolForUpdate(_ context.Context, _ uint64) error {
	return nil
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

func (s *stubOrderRepo) CreateEvent(_ context.Context, event Event) error {
	s.events = append(s.events, event)
	return nil
}

func (s *stubOrderRepo) GetSymbolExposureForUpdate(_ context.Context, symbolID uint64) (SymbolExposure, error) {
	if s.exposure.SymbolID == 0 {
		s.exposure = SymbolExposure{SymbolID: symbolID, LongQty: "0", ShortQty: "0"}
	}
	return s.exposure, nil
}

func (s *stubOrderRepo) GetLatestRiskLevelForUpdate(_ context.Context, _ uint64) (string, error) {
	if s.latestRiskLevel == "" {
		return "", errorsx.ErrNotFound
	}
	return s.latestRiskLevel, nil
}

func (s *stubOrderRepo) GetPositionForUpdate(_ context.Context, _ uint64, _ uint64, _ string, _ string) (Position, error) {
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
	postTradeRisk := &stubPostTradeRisk{}
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
			SessionPolicy:         "ALWAYS_OPEN",
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
	svc.SetPostTradeRiskProcessor(postTradeRisk)

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
	if len(repo.events) != 3 || repo.events[0].EventType != "trade.order.accepted" || repo.events[1].EventType != "trade.fill.created" || repo.events[2].EventType != "trade.position.updated" {
		t.Fatalf("unexpected trade events: %+v", repo.events)
	}
	if repo.position.Side != PositionSideLong || repo.position.Qty != "1" {
		t.Fatalf("unexpected position: %+v", repo.position)
	}
	if repo.position.LiquidationPrice == "0" || repo.position.BankruptcyPrice == "0" {
		t.Fatalf("expected display liquidation/bankruptcy prices to be computed, got %+v", repo.position)
	}
	if len(repo.fills) != 1 || repo.fills[0].Price != "101" {
		t.Fatalf("unexpected fills: %+v", repo.fills)
	}
	if len(postTradeRisk.userIDs) != 1 || postTradeRisk.userIDs[0] != 7 {
		t.Fatalf("expected post-trade risk recalc for user 7, got %+v", postTradeRisk.userIDs)
	}
}

func TestCreateOrder_RejectsWhenActiveOrderLimitReached(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byOrderID["ord_existing"] = Order{
		OrderID:        "ord_existing",
		ClientOrderID:  "cli_existing",
		UserID:         42,
		SymbolID:       1,
		Symbol:         "BTC-PERP",
		PositionEffect: PositionEffectOpen,
		Type:           OrderTypeLimit,
		Status:         OrderStatusResting,
	}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		&stubLedger{},
		stubMarketRepo{symbol: testTradableSymbol(time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC))},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetRuntimeConfigProvider(stubOrderRuntimeProvider{bySymbol: map[string]RuntimeConfig{
		"BTC-PERP": {
			MaxOpenOrdersPerUserPerSymbol: 1,
		},
	}})

	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         42,
		ClientOrderID:  "cli_new",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "LIMIT",
		TimeInForce:    "GTC",
		Qty:            "0.1",
		Price:          stringPtr("99"),
		MarginMode:     "ISOLATED",
		Leverage:       stringPtr("5"),
		TraceID:        "trace_limit",
	})
	if !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestCreateOrder_DefaultsToTierMaxLeverageForMarginHold(t *testing.T) {
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
			SessionPolicy:         "ALWAYS_OPEN",
			IndexPrice:            "100",
			MarkPrice:             "100",
			BestBid:               "99",
			BestAsk:               "101",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
			RiskTiers: []RiskTier{
				{TierLevel: 1, MaxNotional: "100", MaxLeverage: "10", InitialMarginRate: "0.1", MaintenanceRate: "0.05"},
				{TierLevel: 2, MaxNotional: "1000", MaxLeverage: "5", InitialMarginRate: "0.2", MaintenanceRate: "0.1"},
			},
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	repo.position = Position{
		PositionID:    "pos_1",
		UserID:        7,
		SymbolID:      1,
		Side:          PositionSideLong,
		Qty:           "0.5",
		AvgEntryPrice: "100",
		InitialMargin: "5",
		Status:        PositionStatusOpen,
	}
	price := "100"
	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_tier",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "LIMIT",
		Price:          &price,
		Qty:            "1",
		IdempotencyKey: "idem_tier",
		TraceID:        "trace_tier",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.FrozenMargin != "10.06" {
		t.Fatalf("expected default frozen margin 10.06, got %s", order.FrozenMargin)
	}
}

func TestCreateOrder_BlocksOpenOutsideSessionPolicy(t *testing.T) {
	repo := newStubOrderRepo()
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 24, 21, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		&stubLedger{},
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              2,
			Symbol:                "AAPL-USDC",
			ContractMultiplier:    "1",
			TickSize:              "0.01",
			StepSize:              "0.01",
			MinNotional:           "10",
			Status:                "TRADING",
			SessionPolicy:         "US_EQUITY_REGULAR",
			IndexPrice:            "180",
			MarkPrice:             "180",
			BestBid:               "179.9",
			BestAsk:               "180.1",
			InitialMarginRate:     "0.2",
			MaintenanceMarginRate: "0.1",
			SnapshotTS:            time.Date(2026, 3, 24, 20, 59, 0, 0, time.UTC),
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_closed",
		Symbol:         "AAPL-USDC",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "1",
		IdempotencyKey: "idem_closed",
		TraceID:        "trace_closed",
	})
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("expected forbidden outside session, got %v", err)
	}
}

func TestCreateOrder_AllowsReduceOutsideSessionPolicy(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 24, 21, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_reduce", "fill_reduce", "ldg_fill", "evt_fill", "evt_pos"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              2,
			Symbol:                "AAPL-USDC",
			ContractMultiplier:    "1",
			TickSize:              "0.01",
			StepSize:              "0.01",
			MinNotional:           "10",
			Status:                "TRADING",
			SessionPolicy:         "US_EQUITY_REGULAR",
			IndexPrice:            "180",
			MarkPrice:             "180",
			BestBid:               "179.9",
			BestAsk:               "180.1",
			InitialMarginRate:     "0.2",
			MaintenanceMarginRate: "0.1",
			SnapshotTS:            time.Date(2026, 3, 24, 20, 59, 0, 0, time.UTC),
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	repo.position = Position{
		PositionID:        "pos_1",
		UserID:            7,
		SymbolID:          2,
		Side:              PositionSideLong,
		Qty:               "2",
		AvgEntryPrice:     "150",
		MarkPrice:         "180",
		Notional:          "360",
		Leverage:          "3",
		InitialMargin:     "120",
		MaintenanceMargin: "30",
		RealizedPnL:       "0",
		UnrealizedPnL:     "60",
		FundingAccrual:    "0",
		LiquidationPrice:  "120",
		BankruptcyPrice:   "90",
		Status:            PositionStatusOpen,
	}

	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_reduce",
		Symbol:         "AAPL-USDC",
		Side:           "SELL",
		PositionEffect: "REDUCE",
		Type:           "MARKET",
		Qty:            "1",
		IdempotencyKey: "idem_reduce",
		TraceID:        "trace_reduce",
	})
	if err != nil {
		t.Fatalf("expected reduce outside session to succeed, got %v", err)
	}
	if order.Status != OrderStatusFilled {
		t.Fatalf("expected reduce order filled, got %s", order.Status)
	}
}

func TestExecuteRestingOrders_SkipsOpenFillOutsideSessionPolicy(t *testing.T) {
	repo := newStubOrderRepo()
	limitPrice := "180.00"
	repo.byOrderID["ord_1"] = Order{
		OrderID:        "ord_1",
		ClientOrderID:  "cli_1",
		UserID:         7,
		SymbolID:       2,
		Symbol:         "AAPL-USDC",
		Side:           "BUY",
		PositionEffect: PositionEffectOpen,
		Type:           OrderTypeLimit,
		TimeInForce:    "GTC",
		Price:          &limitPrice,
		Qty:            "1",
		Leverage:       "1",
		Status:         OrderStatusResting,
		CreatedAt:      time.Date(2026, 3, 24, 19, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 3, 24, 19, 0, 0, 0, time.UTC),
	}
	repo.byClient["cli_1"] = repo.byOrderID["ord_1"]

	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 24, 21, 0, 0, 0, time.UTC)},
		&fakeIDGen{},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		&stubLedger{},
		stubMarketRepo{symbol: TradableSymbol{
			SymbolID:              2,
			Symbol:                "AAPL-USDC",
			ContractMultiplier:    "1",
			TickSize:              "0.01",
			StepSize:              "0.01",
			MinNotional:           "10",
			Status:                "TRADING",
			SessionPolicy:         "US_EQUITY_REGULAR",
			IndexPrice:            "180",
			MarkPrice:             "180",
			BestBid:               "179.9",
			BestAsk:               "180.0",
			InitialMarginRate:     "0.2",
			MaintenanceMarginRate: "0.1",
			SnapshotTS:            time.Date(2026, 3, 24, 20, 59, 0, 0, time.UTC),
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	executed, err := svc.ExecuteRestingOrders(context.Background(), 10)
	if err != nil {
		t.Fatalf("ExecuteRestingOrders: %v", err)
	}
	if executed != 0 {
		t.Fatalf("expected no resting order execution outside session, got %d", executed)
	}
	if repo.byOrderID["ord_1"].Status != OrderStatusResting {
		t.Fatalf("expected resting order to remain pending, got %s", repo.byOrderID["ord_1"].Status)
	}
}

func TestCreateOrder_UsesRequestedLeverageForRealMarginHold(t *testing.T) {
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
			RiskTiers: []RiskTier{
				{TierLevel: 1, MaxNotional: "1000", MaxLeverage: "40", InitialMarginRate: "0.1", MaintenanceRate: "0.05"},
			},
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_leverage",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "1",
		Leverage:       strPtr("40"),
		IdempotencyKey: "idem_leverage",
		TraceID:        "trace_leverage",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Leverage != "40" {
		t.Fatalf("expected order leverage 40, got %s", order.Leverage)
	}
	if len(repo.byOrderID) != 1 {
		t.Fatalf("expected order persisted")
	}
	position := repo.position
	if position.PositionID == "" {
		t.Fatalf("expected position to be created")
	}
	if position.InitialMargin != "2.525" {
		t.Fatalf("expected position initial margin 2.525, got %s", position.InitialMargin)
	}
	if position.Leverage != "40" {
		t.Fatalf("expected position leverage 40, got %s", position.Leverage)
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

func TestCreateOrder_MarketableLimitOpenFillsImmediately(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	postTradeRisk := &stubPostTradeRisk{}
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
	svc.SetPostTradeRiskProcessor(postTradeRisk)
	price := "102"

	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_marketable_limit",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "LIMIT",
		Qty:            "1",
		Price:          &price,
		IdempotencyKey: "idem_marketable_limit",
		TraceID:        "trace_marketable_limit",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Status != OrderStatusFilled {
		t.Fatalf("expected filled order, got %s", order.Status)
	}
	if order.AvgFillPrice != "101" {
		t.Fatalf("expected immediate fill at best ask 101, got %s", order.AvgFillPrice)
	}
	if len(repo.fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(repo.fills))
	}
	if len(postTradeRisk.userIDs) != 1 || postTradeRisk.userIDs[0] != 7 {
		t.Fatalf("expected post-trade risk recalc for user 7, got %+v", postTradeRisk.userIDs)
	}
}

func TestCreateOrder_MarketableLimitSellOpenUsesExecutionPriceForHold(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	postTradeRisk := &stubPostTradeRisk{}
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
			BestBid:               "101",
			BestAsk:               "103",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetPostTradeRiskProcessor(postTradeRisk)
	price := "100"

	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_marketable_limit_sell",
		Symbol:         "BTC-PERP",
		Side:           "SELL",
		PositionEffect: "OPEN",
		Type:           "LIMIT",
		Qty:            "1",
		Price:          &price,
		IdempotencyKey: "idem_marketable_limit_sell",
		TraceID:        "trace_marketable_limit_sell",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Status != OrderStatusFilled {
		t.Fatalf("expected filled order, got %s", order.Status)
	}
	if order.AvgFillPrice != "101" {
		t.Fatalf("expected immediate fill at best bid 101, got %s", order.AvgFillPrice)
	}
	if len(repo.fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(repo.fills))
	}
	if len(postTradeRisk.userIDs) != 1 || postTradeRisk.userIDs[0] != 7 {
		t.Fatalf("expected post-trade risk recalc for user 7, got %+v", postTradeRisk.userIDs)
	}
}

func TestCreateOrder_OpenRejectedWhenSymbolExposureHardLimitWouldWorsen(t *testing.T) {
	repo := newStubOrderRepo()
	repo.exposure = SymbolExposure{
		SymbolID: 1,
		LongQty:  "2.4",
		ShortQty: "0",
	}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		&stubLedger{},
		stubMarketRepo{symbol: testTradableSymbol(time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC))},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_hard_limit",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "0.2",
		IdempotencyKey: "idem_hard_limit",
		TraceID:        "trace_hard_limit",
	})
	if err == nil {
		t.Fatalf("expected hard limit rejection")
	}
	if !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestCreateOrder_OpenRejectedWhenAccountIsNoNewRisk(t *testing.T) {
	repo := newStubOrderRepo()
	repo.latestRiskLevel = "NO_NEW_RISK"
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: testTradableSymbol(time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC))},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_no_new_risk",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "1",
		IdempotencyKey: "idem_no_new_risk",
		TraceID:        "trace_no_new_risk",
	})
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("expected NO_NEW_RISK rejection, got %v", err)
	}
	if len(ledger.reqs) != 0 {
		t.Fatalf("expected no ledger postings when account cannot open risk")
	}
}

func TestCreateOrder_OpenMarketAppliesDynamicSlippageAgainstDominantExposure(t *testing.T) {
	repo := newStubOrderRepo()
	repo.exposure = SymbolExposure{
		SymbolID: 1,
		LongQty:  "1",
		ShortQty: "0",
	}
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ord_1", "ldg_hold", "evt_hold", "ldg_fill", "evt_fill", "fill_1", "pos_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: testTradableSymbol(time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC))},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_soft_hedge",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           "MARKET",
		Qty:            "1",
		MaxSlippageBps: 150,
		IdempotencyKey: "idem_soft_hedge",
		TraceID:        "trace_soft_hedge",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.AvgFillPrice != "101.1616" {
		t.Fatalf("expected dynamically adjusted fill price, got %s", order.AvgFillPrice)
	}
	if len(repo.fills) != 1 || repo.fills[0].Price != "101.1616" {
		t.Fatalf("unexpected fill price: %+v", repo.fills)
	}
}

func TestCreateOrder_RestingOpenHoldPreventsOverspend(t *testing.T) {
	repo := newStubOrderRepo()
	balances := &trackingBalances{values: map[uint64]string{
		71: "15",
	}}
	ledger := &applyingLedger{balances: balances}
	now := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: now},
		&fakeIDGen{values: []string{"ord_1", "ldg_hold_1", "evt_hold_1", "ord_2"}},
		stubTxManager{},
		stubAccounts{},
		balances,
		ledger,
		stubMarketRepo{symbol: testTradableSymbol(now)},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	price := "100"
	first, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_hold_1",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: PositionEffectOpen,
		Type:           OrderTypeLimit,
		Qty:            "1",
		Price:          &price,
		IdempotencyKey: "idem_hold_1",
		TraceID:        "trace_hold_1",
	})
	if err != nil {
		t.Fatalf("create first resting order: %v", err)
	}
	if first.Status != OrderStatusResting {
		t.Fatalf("expected first order resting, got %+v", first)
	}
	if got := balances.values[71]; got != "4.94" {
		t.Fatalf("expected wallet balance reduced by frozen margin, got %s", got)
	}

	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_hold_2",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: PositionEffectOpen,
		Type:           OrderTypeLimit,
		Qty:            "1",
		Price:          &price,
		IdempotencyKey: "idem_hold_2",
		TraceID:        "trace_hold_2",
	})
	if err == nil || !errors.Is(err, errorsx.ErrConflict) {
		t.Fatalf("expected insufficient balance after first hold, got %v", err)
	}
	if len(ledger.reqs) != 1 {
		t.Fatalf("expected second order to fail before posting, got %d ledger writes", len(ledger.reqs))
	}
}

func TestCreateOrder_StopMarketOpenCreatesTriggerWaitAndHoldsMargin(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	now := time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: now},
		&fakeIDGen{values: []string{"ord_1", "ldg_hold", "evt_hold"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: testTradableSymbol(now)},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	triggerPrice := "110"
	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_trigger_open",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: "OPEN",
		Type:           OrderTypeStopMarket,
		Qty:            "1",
		TriggerPrice:   &triggerPrice,
		IdempotencyKey: "idem_trigger_open",
		TraceID:        "trace_trigger_open",
	})
	if err != nil {
		t.Fatalf("create trigger order: %v", err)
	}
	if order.Status != OrderStatusTriggerWait {
		t.Fatalf("expected trigger wait status, got %+v", order)
	}
	if order.TriggerPrice == nil || *order.TriggerPrice != triggerPrice {
		t.Fatalf("expected trigger price to be persisted, got %+v", order.TriggerPrice)
	}
	if order.FrozenMargin == "0" {
		t.Fatalf("expected trigger open order to hold margin")
	}
	if order.FrozenInitialMargin == "0" || order.FrozenFee == "0" {
		t.Fatalf("expected trigger open order to persist split hold components, got %+v", order)
	}
	if len(ledger.reqs) != 1 {
		t.Fatalf("expected 1 hold ledger posting, got %d", len(ledger.reqs))
	}
	if len(repo.events) != 1 || repo.events[0].EventType != "trade.order.accepted" {
		t.Fatalf("expected accepted event for trigger order, got %+v", repo.events)
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

func TestCancelOrder_TriggerWaitReleasesFrozenMargin(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byOrderID["ord_trigger"] = Order{
		OrderID:       "ord_trigger",
		ClientOrderID: "cli_trigger",
		UserID:        7,
		Status:        OrderStatusTriggerWait,
		FrozenMargin:  "11.176660000000000000",
	}
	repo.byClient["cli_trigger"] = repo.byOrderID["ord_trigger"]
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
		OrderID:        "ord_trigger",
		IdempotencyKey: "idem_cancel_trigger",
		TraceID:        "trace_cancel_trigger",
	}); err != nil {
		t.Fatalf("cancel trigger order: %v", err)
	}
	if len(ledger.reqs) != 1 {
		t.Fatalf("expected 1 ledger release posting, got %d", len(ledger.reqs))
	}
	if repo.byOrderID["ord_trigger"].Status != OrderStatusCanceled || repo.byOrderID["ord_trigger"].FrozenMargin != "0" {
		t.Fatalf("expected trigger order canceled and released, got %+v", repo.byOrderID["ord_trigger"])
	}
	if len(repo.events) != 1 || repo.events[0].EventType != "trade.order.canceled" {
		t.Fatalf("expected canceled event, got %+v", repo.events)
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

func TestCreateOrder_CloseReleasesFundingAccrualFromPositionMargin(t *testing.T) {
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
		FundingAccrual:    "3",
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
		ClientOrderID:  "cli_close_funding",
		Symbol:         "BTC-PERP",
		Side:           "SELL",
		PositionEffect: "CLOSE",
		Type:           "MARKET",
		Qty:            "1",
		IdempotencyKey: "idem_close_funding",
		TraceID:        "trace_close_funding",
	})
	if err != nil {
		t.Fatalf("close order: %v", err)
	}
	if order.Status != OrderStatusFilled || order.FilledQty != "1" {
		t.Fatalf("unexpected close order: %+v", order)
	}
	if len(ledger.reqs) != 1 {
		t.Fatalf("expected a single close settlement posting, got %d", len(ledger.reqs))
	}
	entries := ledger.reqs[0].Entries
	if entries[0].Amount != "-13" {
		t.Fatalf("expected full position margin including funding accrual to be released, got %+v", entries)
	}
	if entries[1].Amount != "31.9286" {
		t.Fatalf("expected wallet settlement to include funding accrual, got %+v", entries)
	}
	if repo.position.Status != PositionStatusClosed || repo.position.InitialMargin != "0" || repo.position.FundingAccrual != "0" {
		t.Fatalf("expected position to be fully closed and released, got %+v", repo.position)
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

func TestCreateOrder_CloseRejectedWhenPositionIsLiquidating(t *testing.T) {
	repo := newStubOrderRepo()
	repo.position = Position{
		PositionID:        "pos_liq_close",
		UserID:            7,
		SymbolID:          1,
		Side:              PositionSideLong,
		Qty:               "1",
		AvgEntryPrice:     "100",
		MarkPrice:         "120",
		Notional:          "120",
		InitialMargin:     "10",
		MaintenanceMargin: "5",
		Status:            PositionStatusLiquidating,
		CreatedAt:         time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 3, 0, 0, time.UTC)},
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
		ClientOrderID:  "cli_liq_close",
		Symbol:         "BTC-PERP",
		Side:           "SELL",
		PositionEffect: "CLOSE",
		Type:           "MARKET",
		Qty:            "0.4",
		IdempotencyKey: "idem_liq_close",
		TraceID:        "trace_liq_close",
	})
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("expected close rejection for liquidating position, got %v", err)
	}
	if len(ledger.reqs) != 0 {
		t.Fatalf("expected no ledger write when liquidating position is already under admin handling")
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

func TestExecuteTriggerOrders_FillsTriggeredCloseOrder(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byOrderID["ord_tp"] = Order{
		OrderID:        "ord_tp",
		ClientOrderID:  "cli_tp",
		UserID:         7,
		SymbolID:       1,
		Symbol:         "BTC-PERP",
		Side:           "SELL",
		PositionEffect: PositionEffectClose,
		Type:           OrderTypeTakeProfitMarket,
		Qty:            "0.4",
		FilledQty:      "0",
		AvgFillPrice:   "0",
		MaxSlippageBps: 100,
		Status:         OrderStatusTriggerWait,
		TriggerPrice:   strPtr("90"),
		CreatedAt:      time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	repo.byClient["cli_tp"] = repo.byOrderID["ord_tp"]
	repo.position = Position{
		PositionID:        "pos_1",
		UserID:            7,
		SymbolID:          1,
		Side:              PositionSideLong,
		Qty:               "1",
		AvgEntryPrice:     "100",
		MarkPrice:         "100",
		Notional:          "100",
		InitialMargin:     "10",
		MaintenanceMargin: "5",
		RealizedPnL:       "0",
		UnrealizedPnL:     "0",
		FundingAccrual:    "0",
		LiquidationPrice:  "0",
		BankruptcyPrice:   "0",
		Status:            PositionStatusOpen,
		CreatedAt:         time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	ledger := &stubLedger{}
	now := time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC)
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: now},
		&fakeIDGen{values: []string{"ldg_fill", "evt_fill", "fill_1"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: testTradableSymbol(now)},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	executed, err := svc.ExecuteTriggerOrders(context.Background(), 10)
	if err != nil {
		t.Fatalf("execute trigger orders: %v", err)
	}
	if executed != 1 {
		t.Fatalf("expected 1 executed trigger order, got %d", executed)
	}
	order := repo.byOrderID["ord_tp"]
	if order.Status != OrderStatusFilled || order.AvgFillPrice != "99" || order.FilledQty != "0.4" {
		t.Fatalf("unexpected triggered order state: %+v", order)
	}
	if len(repo.fills) != 1 {
		t.Fatalf("expected 1 fill for trigger order, got %d", len(repo.fills))
	}
	if len(repo.events) != 2 || repo.events[0].EventType != "trade.fill.created" || repo.events[1].EventType != "trade.position.updated" {
		t.Fatalf("expected trigger execution events, got %+v", repo.events)
	}
	if repo.position.Qty != "0.6" || repo.position.RealizedPnL != "-0.4" {
		t.Fatalf("unexpected updated position after trigger close: %+v", repo.position)
	}
}

func TestExecuteTriggerOrders_CancelsTriggeredCloseWhenPositionIsLiquidating(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byOrderID["ord_tp_liq"] = Order{
		OrderID:        "ord_tp_liq",
		ClientOrderID:  "cli_tp_liq",
		UserID:         7,
		SymbolID:       1,
		Symbol:         "BTC-PERP",
		Side:           "SELL",
		PositionEffect: PositionEffectClose,
		Type:           OrderTypeTakeProfitMarket,
		Qty:            "0.4",
		FilledQty:      "0",
		AvgFillPrice:   "0",
		MaxSlippageBps: 100,
		Status:         OrderStatusTriggerWait,
		TriggerPrice:   strPtr("90"),
		CreatedAt:      time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	repo.byClient["cli_tp_liq"] = repo.byOrderID["ord_tp_liq"]
	repo.position = Position{
		PositionID:        "pos_liq",
		UserID:            7,
		SymbolID:          1,
		Side:              PositionSideLong,
		Qty:               "1",
		AvgEntryPrice:     "100",
		MarkPrice:         "100",
		Notional:          "100",
		InitialMargin:     "10",
		MaintenanceMargin: "5",
		Status:            PositionStatusLiquidating,
		CreatedAt:         time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	ledger := &stubLedger{}
	now := time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC)
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: now},
		&fakeIDGen{values: []string{}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: testTradableSymbol(now)},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	executed, err := svc.ExecuteTriggerOrders(context.Background(), 10)
	if err != nil {
		t.Fatalf("execute trigger orders: %v", err)
	}
	if executed != 0 {
		t.Fatalf("expected no executed trigger order, got %d", executed)
	}
	order := repo.byOrderID["ord_tp_liq"]
	if order.Status != OrderStatusCanceled {
		t.Fatalf("expected triggered close to cancel when position is liquidating, got %+v", order)
	}
	if len(repo.fills) != 0 || len(ledger.reqs) != 0 {
		t.Fatalf("expected no fills or ledger writes, fills=%d ledger=%d", len(repo.fills), len(ledger.reqs))
	}
}

func TestExecuteTriggerOrders_OpenTriggerSkipsWhenSymbolIsReduceOnly(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byOrderID["ord_stop_open"] = Order{
		OrderID:        "ord_stop_open",
		ClientOrderID:  "cli_stop_open",
		UserID:         7,
		SymbolID:       1,
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: PositionEffectOpen,
		Type:           OrderTypeStopMarket,
		Qty:            "1",
		FilledQty:      "0",
		AvgFillPrice:   "0",
		MaxSlippageBps: 100,
		Status:         OrderStatusTriggerWait,
		TriggerPrice:   strPtr("90"),
		FrozenMargin:   "20",
		CreatedAt:      time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	repo.byClient["cli_stop_open"] = repo.byOrderID["ord_stop_open"]
	ledger := &stubLedger{}
	now := time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC)
	market := testTradableSymbol(now)
	market.Status = "REDUCE_ONLY"
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: now},
		&fakeIDGen{values: []string{"unused"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: market},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	executed, err := svc.ExecuteTriggerOrders(context.Background(), 10)
	if err != nil {
		t.Fatalf("execute trigger orders: %v", err)
	}
	if executed != 0 {
		t.Fatalf("expected no trigger execution under reduce-only status, got %d", executed)
	}
	if repo.byOrderID["ord_stop_open"].Status != OrderStatusTriggerWait {
		t.Fatalf("expected trigger order to remain waiting, got %+v", repo.byOrderID["ord_stop_open"])
	}
	if len(repo.fills) != 0 || len(ledger.reqs) != 0 {
		t.Fatalf("expected no fill or ledger write, fills=%d ledger=%d", len(repo.fills), len(ledger.reqs))
	}
}

func TestExecuteTriggerOrders_SkipsWhenMarketDataIsStale(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byOrderID["ord_stale"] = Order{
		OrderID:        "ord_stale",
		ClientOrderID:  "cli_stale",
		UserID:         7,
		SymbolID:       1,
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: PositionEffectOpen,
		Type:           OrderTypeStopMarket,
		Qty:            "1",
		FilledQty:      "0",
		AvgFillPrice:   "0",
		MaxSlippageBps: 100,
		Status:         OrderStatusTriggerWait,
		TriggerPrice:   strPtr("90"),
		FrozenMargin:   "20",
		CreatedAt:      time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	repo.byClient["cli_stale"] = repo.byOrderID["ord_stale"]
	ledger := &stubLedger{}
	cfg := testServiceConfig()
	cfg.MaxMarketDataAge = time.Minute
	now := time.Date(2026, 3, 22, 1, 0, 0, 0, time.UTC)
	market := testTradableSymbol(time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC))
	svc, err := NewService(
		cfg,
		fakeClock{now: now},
		&fakeIDGen{values: []string{"unused"}},
		stubTxManager{},
		stubAccounts{},
		stubBalances{balance: "1000"},
		ledger,
		stubMarketRepo{symbol: market},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	executed, err := svc.ExecuteTriggerOrders(context.Background(), 10)
	if err != nil {
		t.Fatalf("execute trigger orders: %v", err)
	}
	if executed != 0 {
		t.Fatalf("expected stale market data to skip trigger execution, got %d", executed)
	}
	if repo.byOrderID["ord_stale"].Status != OrderStatusTriggerWait {
		t.Fatalf("expected stale trigger order to remain waiting, got %+v", repo.byOrderID["ord_stale"])
	}
	if len(repo.fills) != 0 || len(ledger.reqs) != 0 {
		t.Fatalf("expected no fill or ledger write, fills=%d ledger=%d", len(repo.fills), len(ledger.reqs))
	}
}

func TestExecuteRestingOrders_FillsExecutableOpenLimit(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byOrderID["ord_limit"] = Order{
		OrderID:             "ord_limit",
		ClientOrderID:       "cli_limit",
		UserID:              7,
		SymbolID:            1,
		Symbol:              "BTC-PERP",
		Side:                "BUY",
		PositionEffect:      PositionEffectOpen,
		Type:                OrderTypeLimit,
		Qty:                 "1",
		FilledQty:           "0",
		AvgFillPrice:        "0",
		Leverage:            "10",
		Status:              OrderStatusResting,
		FrozenInitialMargin: "10.2",
		FrozenFee:           "0.0612",
		FrozenMargin:        "10.2612",
		Price:               strPtr("102"),
		CreatedAt:           time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		UpdatedAt:           time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	repo.byClient["cli_limit"] = repo.byOrderID["ord_limit"]
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ldg_fill", "evt_fill", "fill_1", "pos_1"}},
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
			BestBid:               "100",
			BestAsk:               "101",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
			SnapshotTS:            time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC),
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	executed, err := svc.ExecuteRestingOrders(context.Background(), 10)
	if err != nil {
		t.Fatalf("execute resting orders: %v", err)
	}
	if executed != 1 {
		t.Fatalf("expected 1 executed order, got %d", executed)
	}
	order := repo.byOrderID["ord_limit"]
	if order.Status != OrderStatusFilled || order.AvgFillPrice != "101" {
		t.Fatalf("unexpected updated order: %+v", order)
	}
	if len(repo.fills) != 1 {
		t.Fatalf("expected fill written, got %d", len(repo.fills))
	}
}

func TestCreateOrder_UsesPairRuntimeOverridesForOpenMarket(t *testing.T) {
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
		stubMarketRepo{symbol: testTradableSymbol(time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC))},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetRuntimeConfigProvider(stubOrderRuntimeProvider{bySymbol: map[string]RuntimeConfig{
		"BTC-PERP": {
			TakerFeeRate:                 "0.001",
			DefaultMaxSlippageBps:        150,
			MaxLeverage:                  "5",
			MaintenanceMarginUpliftRatio: "0.1",
		},
	}})

	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_pair",
		IdempotencyKey: "idem_pair",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: PositionEffectOpen,
		Type:           OrderTypeMarket,
		Qty:            "1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.MaxSlippageBps != 150 {
		t.Fatalf("expected pair slippage override, got %d", order.MaxSlippageBps)
	}
	if order.Leverage != "5" {
		t.Fatalf("expected pair max leverage to be applied, got %s", order.Leverage)
	}
	if len(repo.fills) != 1 || repo.fills[0].FeeAmount != "0.101" {
		t.Fatalf("expected taker fee override to be applied, fills=%+v", repo.fills)
	}
	if repo.position.MaintenanceMargin != "5.5" {
		t.Fatalf("expected maintenance uplift override, got %+v", repo.position)
	}
}

func TestCreateOrder_AllowsRaisedPairMaxLeverageOverride(t *testing.T) {
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
			InitialMarginRate:     "0.025",
			MaintenanceMarginRate: "0.0125",
			RiskTiers: []RiskTier{
				{TierLevel: 1, MaxNotional: "1000", MaxLeverage: "40", InitialMarginRate: "0.025", MaintenanceRate: "0.0125"},
			},
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetRuntimeConfigProvider(stubOrderRuntimeProvider{bySymbol: map[string]RuntimeConfig{
		"BTC-PERP": {
			MaxLeverage: "2000",
		},
	}})

	order, err := svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_pair_raised",
		IdempotencyKey: "idem_pair_raised",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: PositionEffectOpen,
		Type:           OrderTypeMarket,
		Qty:            "1",
		Leverage:       strPtr("2000"),
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Leverage != "2000" {
		t.Fatalf("expected raised pair max leverage to be applied, got %s", order.Leverage)
	}
	if repo.position.Leverage != "2000" {
		t.Fatalf("expected position leverage 2000, got %+v", repo.position)
	}
}

func TestCreateOrder_RejectsIsolatedOpenWhenItWouldEnterLiquidationImmediately(t *testing.T) {
	repo := newStubOrderRepo()
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
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
			InitialMarginRate:     "0.025",
			MaintenanceMarginRate: "0.0125",
			RiskTiers: []RiskTier{
				{TierLevel: 1, MaxNotional: "1000", MaxLeverage: "40", InitialMarginRate: "0.025", MaintenanceRate: "0.0125", LiquidationFeeRate: "0.005"},
			},
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetRuntimeConfigProvider(stubOrderRuntimeProvider{bySymbol: map[string]RuntimeConfig{
		"BTC-PERP": {
			MaxLeverage: "2000",
		},
	}})

	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_pair_raised_iso",
		IdempotencyKey: "idem_pair_raised_iso",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: PositionEffectOpen,
		Type:           OrderTypeMarket,
		Qty:            "1",
		Leverage:       strPtr("2000"),
		MarginMode:     MarginModeIsolated,
	})
	if err == nil || !errors.Is(err, errorsx.ErrInvalidArgument) {
		t.Fatalf("expected immediate-liquidation rejection, got %v", err)
	}
	if len(ledger.reqs) != 0 {
		t.Fatalf("expected no ledger postings on rejected isolated order")
	}
	if len(repo.byOrderID) != 0 || len(repo.fills) != 0 {
		t.Fatalf("expected rejected order to avoid persistence, orders=%d fills=%d", len(repo.byOrderID), len(repo.fills))
	}
}

func TestCreateOrder_RejectsOpenWhenTargetPositionIsLiquidating(t *testing.T) {
	repo := newStubOrderRepo()
	repo.position = Position{
		PositionID: "pos_liq",
		UserID:     7,
		SymbolID:   1,
		Side:       PositionSideLong,
		MarginMode: MarginModeIsolated,
		Qty:        "1",
		Status:     PositionStatusLiquidating,
	}
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC)},
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
			RiskTiers: []RiskTier{
				{TierLevel: 1, MaxNotional: "1000", MaxLeverage: "20", InitialMarginRate: "0.05", MaintenanceRate: "0.025", LiquidationFeeRate: "0.005"},
			},
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.CreateOrder(context.Background(), CreateOrderInput{
		UserID:         7,
		ClientOrderID:  "cli_liq_blocked",
		IdempotencyKey: "idem_liq_blocked",
		Symbol:         "BTC-PERP",
		Side:           "BUY",
		PositionEffect: PositionEffectOpen,
		Type:           OrderTypeMarket,
		Qty:            "1",
		Leverage:       strPtr("5"),
		MarginMode:     MarginModeIsolated,
	})
	if err == nil || !errors.Is(err, errorsx.ErrForbidden) {
		t.Fatalf("expected liquidating-position rejection, got %v", err)
	}
	if len(ledger.reqs) != 0 {
		t.Fatalf("expected no ledger postings on blocked open order")
	}
	if len(repo.byOrderID) != 0 || len(repo.fills) != 0 {
		t.Fatalf("expected blocked open order to avoid persistence, orders=%d fills=%d", len(repo.byOrderID), len(repo.fills))
	}
}

func TestExecuteRestingOrders_RejectsIsolatedOrderThatWouldEnterLiquidationImmediately(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byOrderID["ord_limit_iso"] = Order{
		OrderID:             "ord_limit_iso",
		ClientOrderID:       "cli_limit_iso",
		UserID:              7,
		SymbolID:            1,
		Symbol:              "BTC-PERP",
		Side:                "BUY",
		MarginMode:          MarginModeIsolated,
		PositionEffect:      PositionEffectOpen,
		Type:                OrderTypeLimit,
		Qty:                 "1",
		FilledQty:           "0",
		AvgFillPrice:        "0",
		Leverage:            "2000",
		Status:              OrderStatusResting,
		FrozenInitialMargin: "0.051",
		FrozenFee:           "0.0612",
		FrozenMargin:        "0.1122",
		Price:               strPtr("102"),
		CreatedAt:           time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		UpdatedAt:           time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	repo.byClient["cli_limit_iso"] = repo.byOrderID["ord_limit_iso"]
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ldg_reject", "evt_reject", "evt_order_rejected"}},
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
			BestBid:               "100",
			BestAsk:               "101",
			InitialMarginRate:     "0.025",
			MaintenanceMarginRate: "0.0125",
			RiskTiers: []RiskTier{
				{TierLevel: 1, MaxNotional: "1000", MaxLeverage: "40", InitialMarginRate: "0.025", MaintenanceRate: "0.0125", LiquidationFeeRate: "0.005"},
			},
			SnapshotTS: time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC),
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	executed, err := svc.ExecuteRestingOrders(context.Background(), 10)
	if err != nil {
		t.Fatalf("execute resting orders: %v", err)
	}
	if executed != 0 {
		t.Fatalf("expected rejected isolated order to not count as executed, got %d", executed)
	}
	order := repo.byOrderID["ord_limit_iso"]
	if order.Status != OrderStatusRejected {
		t.Fatalf("expected resting order to be rejected, got %+v", order)
	}
	if order.RejectReason == nil || *order.RejectReason != rejectReasonImmediateLiquidation {
		t.Fatalf("unexpected reject reason: %+v", order.RejectReason)
	}
	if len(ledger.reqs) != 1 {
		t.Fatalf("expected one release ledger posting, got %d", len(ledger.reqs))
	}
	if len(repo.fills) != 0 {
		t.Fatalf("expected no fills for rejected isolated order, got %d", len(repo.fills))
	}
	if len(repo.events) != 1 || repo.events[0].EventType != "trade.order.rejected" {
		t.Fatalf("expected rejected event, got %+v", repo.events)
	}
}

func TestExecuteRestingOrders_RejectsOpenOrderWhenTargetPositionIsLiquidating(t *testing.T) {
	repo := newStubOrderRepo()
	repo.position = Position{
		PositionID: "pos_liq",
		UserID:     7,
		SymbolID:   1,
		Side:       PositionSideLong,
		MarginMode: MarginModeIsolated,
		Qty:        "1",
		Status:     PositionStatusLiquidating,
	}
	repo.byOrderID["ord_limit_liq"] = Order{
		OrderID:             "ord_limit_liq",
		ClientOrderID:       "cli_limit_liq",
		UserID:              7,
		SymbolID:            1,
		Symbol:              "BTC-PERP",
		Side:                "BUY",
		MarginMode:          MarginModeIsolated,
		PositionEffect:      PositionEffectOpen,
		Type:                OrderTypeLimit,
		Qty:                 "1",
		FilledQty:           "0",
		AvgFillPrice:        "0",
		Leverage:            "10",
		Status:              OrderStatusResting,
		FrozenInitialMargin: "10",
		FrozenFee:           "0.06",
		FrozenMargin:        "10.06",
		Price:               strPtr("102"),
		CreatedAt:           time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		UpdatedAt:           time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	repo.byClient["cli_limit_liq"] = repo.byOrderID["ord_limit_liq"]
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ldg_reject", "evt_reject", "evt_order_rejected"}},
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
			BestBid:               "100",
			BestAsk:               "101",
			InitialMarginRate:     "0.025",
			MaintenanceMarginRate: "0.0125",
			RiskTiers: []RiskTier{
				{TierLevel: 1, MaxNotional: "1000", MaxLeverage: "40", InitialMarginRate: "0.025", MaintenanceRate: "0.0125", LiquidationFeeRate: "0.005"},
			},
			SnapshotTS: time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC),
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	executed, err := svc.ExecuteRestingOrders(context.Background(), 10)
	if err != nil {
		t.Fatalf("execute resting orders: %v", err)
	}
	if executed != 0 {
		t.Fatalf("expected liquidating-position rejection to not count as executed, got %d", executed)
	}
	order := repo.byOrderID["ord_limit_liq"]
	if order.Status != OrderStatusRejected {
		t.Fatalf("expected resting order to be rejected, got %+v", order)
	}
	if order.RejectReason == nil || *order.RejectReason != rejectReasonPositionLiquidating {
		t.Fatalf("unexpected reject reason: %+v", order.RejectReason)
	}
	if len(ledger.reqs) != 1 {
		t.Fatalf("expected one release ledger posting, got %d", len(ledger.reqs))
	}
	if len(repo.fills) != 0 {
		t.Fatalf("expected no fills for rejected liquidating-position order, got %d", len(repo.fills))
	}
	if len(repo.events) != 1 || repo.events[0].EventType != "trade.order.rejected" {
		t.Fatalf("expected rejected event, got %+v", repo.events)
	}
}

func TestExecuteRestingOrders_UsesPairMakerFeeOverride(t *testing.T) {
	repo := newStubOrderRepo()
	repo.byOrderID["ord_limit"] = Order{
		OrderID:             "ord_limit",
		ClientOrderID:       "cli_limit",
		UserID:              7,
		SymbolID:            1,
		Symbol:              "BTC-PERP",
		Side:                "BUY",
		PositionEffect:      PositionEffectOpen,
		Type:                OrderTypeLimit,
		Qty:                 "1",
		FilledQty:           "0",
		AvgFillPrice:        "0",
		Leverage:            "10",
		Status:              OrderStatusResting,
		FrozenInitialMargin: "10.2",
		FrozenFee:           "0.0612",
		FrozenMargin:        "10.2612",
		Price:               strPtr("102"),
		CreatedAt:           time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
		UpdatedAt:           time.Date(2026, 3, 22, 0, 0, 0, 0, time.UTC),
	}
	repo.byClient["cli_limit"] = repo.byOrderID["ord_limit"]
	ledger := &stubLedger{}
	svc, err := NewService(
		testServiceConfig(),
		fakeClock{now: time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC)},
		&fakeIDGen{values: []string{"ldg_fill", "evt_fill", "fill_1", "pos_1"}},
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
			BestBid:               "100",
			BestAsk:               "101",
			InitialMarginRate:     "0.1",
			MaintenanceMarginRate: "0.05",
			SnapshotTS:            time.Date(2026, 3, 22, 0, 1, 0, 0, time.UTC),
		}},
		repo,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetRuntimeConfigProvider(stubOrderRuntimeProvider{bySymbol: map[string]RuntimeConfig{
		"BTC-PERP": {
			MakerFeeRate: "0.0001",
		},
	}})

	executed, err := svc.ExecuteRestingOrders(context.Background(), 10)
	if err != nil {
		t.Fatalf("execute resting orders: %v", err)
	}
	if executed != 1 {
		t.Fatalf("expected 1 executed order, got %d", executed)
	}
	if len(repo.fills) != 1 || repo.fills[0].FeeAmount != "0.0101" {
		t.Fatalf("expected maker fee override to be applied, fills=%+v", repo.fills)
	}
}

func strPtr(v string) *string { return &v }
