package order

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type ServiceConfig struct {
	Asset                 string
	TakerFeeRate          string
	MakerFeeRate          string
	DefaultMaxSlippageBps int
	MaxMarketDataAge      time.Duration
}

type Service struct {
	cfg      ServiceConfig
	clock    Clock
	idgen    IDGenerator
	txm      TxManager
	accounts AccountResolver
	balances BalanceRepository
	ledger   LedgerPoster
	markets  MarketRepository
	repo     OrderRepository
}

func NewService(
	cfg ServiceConfig,
	clock Clock,
	idgen IDGenerator,
	txm TxManager,
	accounts AccountResolver,
	balances BalanceRepository,
	ledger LedgerPoster,
	markets MarketRepository,
	repo OrderRepository,
) (*Service, error) {
	if cfg.Asset == "" || cfg.TakerFeeRate == "" || cfg.MakerFeeRate == "" || cfg.DefaultMaxSlippageBps <= 0 || cfg.MaxMarketDataAge <= 0 {
		return nil, fmt.Errorf("%w: invalid order service config", errorsx.ErrInvalidArgument)
	}
	if clock == nil || idgen == nil || txm == nil || accounts == nil || balances == nil || ledger == nil || markets == nil || repo == nil {
		return nil, fmt.Errorf("%w: missing order dependency", errorsx.ErrInvalidArgument)
	}
	return &Service{
		cfg:      cfg,
		clock:    clock,
		idgen:    idgen,
		txm:      txm,
		accounts: accounts,
		balances: balances,
		ledger:   ledger,
		markets:  markets,
		repo:     repo,
	}, nil
}

func (s *Service) CreateOrder(ctx context.Context, input CreateOrderInput) (Order, error) {
	if input.UserID == 0 || input.ClientOrderID == "" || input.Symbol == "" || input.Side == "" || input.PositionEffect == "" || input.Type == "" || input.Qty == "" {
		return Order{}, fmt.Errorf("%w: missing order fields", errorsx.ErrInvalidArgument)
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = input.ClientOrderID
	}
	if existing, err := s.repo.GetByUserClientOrderID(ctx, input.UserID, input.ClientOrderID); err == nil {
		return existing, nil
	} else if err != nil && !isNotFound(err) {
		return Order{}, err
	}

	market, err := s.markets.GetTradableSymbol(ctx, input.Symbol)
	if err != nil {
		return Order{}, err
	}
	positionEffect := normalizePositionEffect(input.PositionEffect, input.ReduceOnly)
	if positionEffect == "" {
		return Order{}, fmt.Errorf("%w: unsupported position_effect", errorsx.ErrInvalidArgument)
	}
	effectiveStatus := market.Status
	if market.SnapshotTS.IsZero() || s.clock.Now().UTC().Sub(market.SnapshotTS) > s.cfg.MaxMarketDataAge {
		effectiveStatus = "REDUCE_ONLY"
	}
	if !canTradeUnderSymbolStatus(effectiveStatus, positionEffect) {
		return Order{}, fmt.Errorf("%w: symbol %s is not tradable for %s", errorsx.ErrForbidden, input.Symbol, positionEffect)
	}

	qty, err := decimalx.NewFromString(input.Qty)
	if err != nil {
		return Order{}, err
	}
	if !qty.GreaterThan(decimalx.MustFromString("0")) {
		return Order{}, fmt.Errorf("%w: qty must be positive", errorsx.ErrInvalidArgument)
	}
	if !isStepAligned(qty, decimalx.MustFromString(market.StepSize)) {
		return Order{}, fmt.Errorf("%w: qty does not match step size", errorsx.ErrInvalidArgument)
	}

	side := normalizeSide(input.Side)
	if side == "" {
		return Order{}, fmt.Errorf("%w: unsupported side", errorsx.ErrInvalidArgument)
	}

	maxSlippageBps := input.MaxSlippageBps
	if maxSlippageBps <= 0 {
		maxSlippageBps = s.cfg.DefaultMaxSlippageBps
	}

	var limitPrice *decimalx.Decimal
	if input.Price != nil {
		parsed, err := decimalx.NewFromString(*input.Price)
		if err != nil {
			return Order{}, err
		}
		if !parsed.GreaterThan(decimalx.MustFromString("0")) {
			return Order{}, fmt.Errorf("%w: price must be positive", errorsx.ErrInvalidArgument)
		}
		if !isStepAligned(parsed, decimalx.MustFromString(market.TickSize)) {
			return Order{}, fmt.Errorf("%w: price does not match tick size", errorsx.ErrInvalidArgument)
		}
		limitPrice = &parsed
	}

	referencePrice, executionPrice, err := s.resolvePrices(market, side, strings.ToUpper(strings.TrimSpace(input.Type)), limitPrice, maxSlippageBps)
	if err != nil {
		return Order{}, err
	}
	notional := qty.Mul(referencePrice).Mul(decimalx.MustFromString(market.ContractMultiplier))
	if notional.LessThan(decimalx.MustFromString(market.MinNotional)) {
		return Order{}, fmt.Errorf("%w: notional below minimum", errorsx.ErrInvalidArgument)
	}

	imr := decimalx.MustFromString(market.InitialMarginRate)
	marginRequired := notional.Mul(imr)
	feeRate := decimalx.MustFromString(s.cfg.TakerFeeRate)
	feeEstimate := notional.Mul(feeRate)
	holdAmount := marginRequired.Add(feeEstimate)

	orderID := s.idgen.NewID("ord")
	now := s.clock.Now().UTC()

	order := Order{
		OrderID:        orderID,
		ClientOrderID:  input.ClientOrderID,
		UserID:         input.UserID,
		SymbolID:       market.SymbolID,
		Symbol:         market.Symbol,
		Side:           side,
		PositionEffect: positionEffect,
		Type:           strings.ToUpper(strings.TrimSpace(input.Type)),
		TimeInForce:    normalizeTimeInForce(input.TimeInForce),
		Qty:            qty.String(),
		FilledQty:      "0",
		AvgFillPrice:   "0",
		ReduceOnly:     input.ReduceOnly,
		MaxSlippageBps: maxSlippageBps,
		Status:         OrderStatusResting,
		FrozenMargin:   holdAmount.String(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if input.Price != nil {
		price := limitPrice.String()
		order.Price = &price
	}
	if input.TriggerPrice != nil {
		trigger := strings.TrimSpace(*input.TriggerPrice)
		order.TriggerPrice = &trigger
	}

	err = s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		tradeAccounts, err := s.accounts.ResolveTradeAccounts(txCtx, input.UserID, s.cfg.Asset)
		if err != nil {
			return err
		}

		if positionEffect == PositionEffectOpen {
			created, err := s.createOpenOrder(txCtx, input, market, order, qty, executionPrice, holdAmount, imr, feeRate, now, tradeAccounts.UserWalletAccountID, tradeAccounts.UserOrderMarginAccountID, tradeAccounts.UserPositionMarginAccountID, tradeAccounts.TradingFeeAccountID)
			if err != nil {
				return err
			}
			order = created
			return nil
		}
		created, err := s.createCloseOrder(txCtx, input, market, order, qty, executionPrice, feeRate, now, tradeAccounts.UserWalletAccountID, tradeAccounts.UserPositionMarginAccountID, tradeAccounts.SystemPoolAccountID, tradeAccounts.TradingFeeAccountID)
		if err != nil {
			return err
		}
		order = created
		return nil
	})
	if err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Service) createOpenOrder(
	txCtx context.Context,
	input CreateOrderInput,
	market TradableSymbol,
	order Order,
	qty decimalx.Decimal,
	executionPrice decimalx.Decimal,
	holdAmount decimalx.Decimal,
	imr decimalx.Decimal,
	feeRate decimalx.Decimal,
	now time.Time,
	walletAccountID uint64,
	orderMarginAccountID uint64,
	positionMarginAccountID uint64,
	tradingFeeAccountID uint64,
) (Order, error) {
	available, err := s.balances.GetAccountBalanceForUpdate(txCtx, walletAccountID, s.cfg.Asset)
	if err != nil {
		return Order{}, err
	}
	if decimalx.MustFromString(available).LessThan(holdAmount) {
		return Order{}, fmt.Errorf("%w: insufficient available balance", errorsx.ErrConflict)
	}

	if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
		LedgerTx: ledgerdomain.LedgerTx{
			ID:             s.idgen.NewID("ldg"),
			EventID:        s.idgen.NewID("evt"),
			BizType:        "trade.order.hold",
			BizRefID:       order.OrderID,
			Asset:          s.cfg.Asset,
			IdempotencyKey: input.IdempotencyKey + ":hold",
			OperatorType:   "USER",
			OperatorID:     fmt.Sprintf("%d", input.UserID),
			TraceID:        input.TraceID,
			Status:         "COMMITTED",
			CreatedAt:      now,
		},
		Entries: []ledgerdomain.LedgerEntry{
			{AccountID: walletAccountID, UserID: uint64Ptr(input.UserID), Asset: s.cfg.Asset, Amount: holdAmount.Neg().String(), EntryType: "TRADE_ORDER_HOLD"},
			{AccountID: orderMarginAccountID, UserID: uint64Ptr(input.UserID), Asset: s.cfg.Asset, Amount: holdAmount.String(), EntryType: "TRADE_ORDER_HOLD"},
		},
	}); err != nil {
		return Order{}, err
	}

	if order.Type == OrderTypeLimit {
		return order, s.repo.CreateOrder(txCtx, order)
	}

	fillQty := qty
	fillPrice := executionPrice
	fillNotional := fillQty.Mul(fillPrice).Mul(decimalx.MustFromString(market.ContractMultiplier))
	fillFee := fillNotional.Mul(feeRate)
	fillMargin := fillNotional.Mul(imr)
	refund := holdAmount.Sub(fillMargin.Add(fillFee))
	if refund.LessThan(decimalx.MustFromString("0")) {
		return Order{}, fmt.Errorf("%w: execution exceeded held margin", errorsx.ErrConflict)
	}

	order.Status = OrderStatusFilled
	order.FilledQty = fillQty.String()
	order.AvgFillPrice = fillPrice.String()
	order.FrozenMargin = "0"
	if err := s.repo.CreateOrder(txCtx, order); err != nil {
		return Order{}, err
	}

	fillLedgerTxID := s.idgen.NewID("ldg")
	entries := []ledgerdomain.LedgerEntry{
		{AccountID: orderMarginAccountID, UserID: uint64Ptr(input.UserID), Asset: s.cfg.Asset, Amount: fillMargin.Add(fillFee).Neg().String(), EntryType: "TRADE_OPEN_EXECUTION"},
		{AccountID: positionMarginAccountID, UserID: uint64Ptr(input.UserID), Asset: s.cfg.Asset, Amount: fillMargin.String(), EntryType: "TRADE_POSITION_MARGIN"},
		{AccountID: tradingFeeAccountID, Asset: s.cfg.Asset, Amount: fillFee.String(), EntryType: "TRADE_FEE"},
	}
	if refund.GreaterThan(decimalx.MustFromString("0")) {
		entries[0].Amount = holdAmount.Neg().String()
		entries = append(entries, ledgerdomain.LedgerEntry{
			AccountID: walletAccountID,
			UserID:    uint64Ptr(input.UserID),
			Asset:     s.cfg.Asset,
			Amount:    refund.String(),
			EntryType: "TRADE_ORDER_RELEASE",
		})
	}
	if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
		LedgerTx: ledgerdomain.LedgerTx{
			ID:             fillLedgerTxID,
			EventID:        s.idgen.NewID("evt"),
			BizType:        "trade.fill",
			BizRefID:       order.OrderID,
			Asset:          s.cfg.Asset,
			IdempotencyKey: input.IdempotencyKey + ":fill",
			OperatorType:   "USER",
			OperatorID:     fmt.Sprintf("%d", input.UserID),
			TraceID:        input.TraceID,
			Status:         "COMMITTED",
			CreatedAt:      now,
		},
		Entries: entries,
	}); err != nil {
		return Order{}, err
	}

	fill := Fill{
		FillID:     s.idgen.NewID("fill"),
		OrderID:    order.OrderID,
		UserID:     input.UserID,
		SymbolID:   market.SymbolID,
		Side:       order.Side,
		Qty:        fillQty.String(),
		Price:      fillPrice.String(),
		FeeAmount:  fillFee.String(),
		LedgerTxID: fillLedgerTxID,
		CreatedAt:  now,
	}
	if err := s.repo.CreateFill(txCtx, fill); err != nil {
		return Order{}, err
	}

	positionSide := orderSideToPositionSide(order.Side)
	position, err := s.repo.GetPositionForUpdate(txCtx, input.UserID, market.SymbolID, positionSide)
	if err != nil && !isNotFound(err) {
		return Order{}, err
	}
	position = applyOpenFill(position, input.UserID, market, positionSide, fillQty, fillPrice, fillMargin, now, s.idgen)
	if err := s.repo.UpsertPosition(txCtx, position); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Service) createCloseOrder(
	txCtx context.Context,
	input CreateOrderInput,
	market TradableSymbol,
	order Order,
	requestQty decimalx.Decimal,
	executionPrice decimalx.Decimal,
	feeRate decimalx.Decimal,
	now time.Time,
	walletAccountID uint64,
	positionMarginAccountID uint64,
	systemPoolAccountID uint64,
	tradingFeeAccountID uint64,
) (Order, error) {
	if order.Type != OrderTypeMarket {
		return Order{}, fmt.Errorf("%w: close/reduce currently only supports MARKET", errorsx.ErrInvalidArgument)
	}
	targetPositionSide := closingTargetPositionSide(order.Side)
	position, err := s.repo.GetPositionForUpdate(txCtx, input.UserID, market.SymbolID, targetPositionSide)
	if err != nil {
		return Order{}, err
	}
	currentQty := decimalx.MustFromString(position.Qty)
	if !currentQty.GreaterThan(decimalx.MustFromString("0")) {
		return Order{}, fmt.Errorf("%w: no position to reduce", errorsx.ErrConflict)
	}
	closeQty := requestQty
	if closeQty.GreaterThan(currentQty) {
		closeQty = currentQty
	}
	if !closeQty.GreaterThan(decimalx.MustFromString("0")) {
		return Order{}, fmt.Errorf("%w: close qty must be positive", errorsx.ErrInvalidArgument)
	}

	entryPrice := decimalx.MustFromString(position.AvgEntryPrice)
	multiplier := decimalx.MustFromString(market.ContractMultiplier)
	closeNotional := closeQty.Mul(executionPrice).Mul(multiplier)
	closeFee := closeNotional.Mul(feeRate)
	currentInitialMargin := decimalx.MustFromString(position.InitialMargin)
	releasedMargin := proportionalAmount(currentInitialMargin, closeQty, currentQty)
	realizedPnL := realizedPnLDelta(targetPositionSide, closeQty, executionPrice, entryPrice, multiplier)
	userWalletDelta := releasedMargin.Add(realizedPnL).Sub(closeFee)

	order.Status = OrderStatusFilled
	order.FilledQty = closeQty.String()
	order.AvgFillPrice = executionPrice.String()
	order.FrozenMargin = "0"
	if err := s.repo.CreateOrder(txCtx, order); err != nil {
		return Order{}, err
	}

	fillLedgerTxID := s.idgen.NewID("ldg")
	entries := []ledgerdomain.LedgerEntry{
		{AccountID: positionMarginAccountID, UserID: uint64Ptr(input.UserID), Asset: s.cfg.Asset, Amount: releasedMargin.Neg().String(), EntryType: "TRADE_CLOSE_MARGIN_RELEASE"},
		{AccountID: walletAccountID, UserID: uint64Ptr(input.UserID), Asset: s.cfg.Asset, Amount: userWalletDelta.String(), EntryType: "TRADE_CLOSE_SETTLEMENT"},
		{AccountID: tradingFeeAccountID, Asset: s.cfg.Asset, Amount: closeFee.String(), EntryType: "TRADE_FEE"},
	}
	if realizedPnL.GreaterThan(decimalx.MustFromString("0")) {
		entries = append(entries, ledgerdomain.LedgerEntry{
			AccountID: systemPoolAccountID,
			Asset:     s.cfg.Asset,
			Amount:    realizedPnL.Neg().String(),
			EntryType: "TRADE_REALIZED_PNL",
		})
	} else if realizedPnL.LessThan(decimalx.MustFromString("0")) {
		entries = append(entries, ledgerdomain.LedgerEntry{
			AccountID: systemPoolAccountID,
			Asset:     s.cfg.Asset,
			Amount:    realizedPnL.Abs().String(),
			EntryType: "TRADE_REALIZED_LOSS",
		})
	}
	if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
		LedgerTx: ledgerdomain.LedgerTx{
			ID:             fillLedgerTxID,
			EventID:        s.idgen.NewID("evt"),
			BizType:        "trade.fill.close",
			BizRefID:       order.OrderID,
			Asset:          s.cfg.Asset,
			IdempotencyKey: input.IdempotencyKey + ":fill",
			OperatorType:   "USER",
			OperatorID:     fmt.Sprintf("%d", input.UserID),
			TraceID:        input.TraceID,
			Status:         "COMMITTED",
			CreatedAt:      now,
		},
		Entries: entries,
	}); err != nil {
		return Order{}, err
	}

	fill := Fill{
		FillID:     s.idgen.NewID("fill"),
		OrderID:    order.OrderID,
		UserID:     input.UserID,
		SymbolID:   market.SymbolID,
		Side:       order.Side,
		Qty:        closeQty.String(),
		Price:      executionPrice.String(),
		FeeAmount:  closeFee.String(),
		LedgerTxID: fillLedgerTxID,
		CreatedAt:  now,
	}
	if err := s.repo.CreateFill(txCtx, fill); err != nil {
		return Order{}, err
	}

	position = applyCloseFill(position, market, closeQty, executionPrice, releasedMargin, realizedPnL, now)
	if err := s.repo.UpsertPosition(txCtx, position); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Service) CancelOrder(ctx context.Context, input CancelOrderInput) error {
	if input.UserID == 0 || input.OrderID == "" {
		return fmt.Errorf("%w: missing cancel order fields", errorsx.ErrInvalidArgument)
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = input.OrderID
	}
	now := s.clock.Now().UTC()
	return s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		order, err := s.repo.GetByUserOrderIDForUpdate(txCtx, input.UserID, input.OrderID)
		if err != nil {
			return err
		}
		if order.Status == OrderStatusCanceled {
			return nil
		}
		if order.Status != OrderStatusResting {
			return fmt.Errorf("%w: order is not cancelable", errorsx.ErrConflict)
		}

		tradeAccounts, err := s.accounts.ResolveTradeAccounts(txCtx, input.UserID, s.cfg.Asset)
		if err != nil {
			return err
		}

		releaseAmount := decimalx.MustFromString(order.FrozenMargin)
		if releaseAmount.GreaterThan(decimalx.MustFromString("0")) {
			if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
				LedgerTx: ledgerdomain.LedgerTx{
					ID:             s.idgen.NewID("ldg"),
					EventID:        s.idgen.NewID("evt"),
					BizType:        "trade.order.cancel",
					BizRefID:       order.OrderID,
					Asset:          s.cfg.Asset,
					IdempotencyKey: input.IdempotencyKey + ":release",
					OperatorType:   "USER",
					OperatorID:     fmt.Sprintf("%d", input.UserID),
					TraceID:        input.TraceID,
					Status:         "COMMITTED",
					CreatedAt:      now,
				},
				Entries: []ledgerdomain.LedgerEntry{
					{AccountID: tradeAccounts.UserOrderMarginAccountID, UserID: uint64Ptr(input.UserID), Asset: s.cfg.Asset, Amount: releaseAmount.Neg().String(), EntryType: "TRADE_ORDER_RELEASE"},
					{AccountID: tradeAccounts.UserWalletAccountID, UserID: uint64Ptr(input.UserID), Asset: s.cfg.Asset, Amount: releaseAmount.String(), EntryType: "TRADE_ORDER_RELEASE"},
				},
			}); err != nil {
				return err
			}
		}

		order.Status = OrderStatusCanceled
		order.FrozenMargin = "0"
		order.UpdatedAt = now
		return s.repo.UpdateOrder(txCtx, order)
	})
}

func (s *Service) resolvePrices(market TradableSymbol, side string, orderType string, limitPrice *decimalx.Decimal, maxSlippageBps int) (decimalx.Decimal, decimalx.Decimal, error) {
	mark := decimalx.MustFromString(market.MarkPrice)
	bestBid := decimalx.MustFromString(market.BestBid)
	bestAsk := decimalx.MustFromString(market.BestAsk)
	switch orderType {
	case OrderTypeMarket:
		var execution decimalx.Decimal
		if side == "BUY" {
			execution = bestAsk
		} else {
			execution = bestBid
		}
		if !execution.GreaterThan(decimalx.MustFromString("0")) || !mark.GreaterThan(decimalx.MustFromString("0")) {
			return decimalx.Decimal{}, decimalx.Decimal{}, fmt.Errorf("%w: market data unavailable", errorsx.ErrConflict)
		}
		slippageLimit := mark.Mul(decimalx.MustFromString(fmt.Sprintf("%d", maxSlippageBps))).Div(decimalx.MustFromString("10000"))
		if side == "BUY" && execution.GreaterThan(mark.Add(slippageLimit)) {
			return decimalx.Decimal{}, decimalx.Decimal{}, fmt.Errorf("%w: market buy execution exceeds slippage limit", errorsx.ErrConflict)
		}
		if side == "SELL" && execution.LessThan(mark.Sub(slippageLimit)) {
			return decimalx.Decimal{}, decimalx.Decimal{}, fmt.Errorf("%w: market sell execution exceeds slippage limit", errorsx.ErrConflict)
		}
		return execution, execution, nil
	case OrderTypeLimit:
		if limitPrice == nil {
			return decimalx.Decimal{}, decimalx.Decimal{}, fmt.Errorf("%w: limit price is required", errorsx.ErrInvalidArgument)
		}
		return *limitPrice, decimalx.Decimal{}, nil
	default:
		return decimalx.Decimal{}, decimalx.Decimal{}, fmt.Errorf("%w: unsupported order type", errorsx.ErrInvalidArgument)
	}
}

func applyOpenFill(current Position, userID uint64, market TradableSymbol, positionSide string, fillQty decimalx.Decimal, fillPrice decimalx.Decimal, fillMargin decimalx.Decimal, now time.Time, idgen IDGenerator) Position {
	if current.Qty == "" {
		current.Qty = "0"
	}
	if current.AvgEntryPrice == "" {
		current.AvgEntryPrice = "0"
	}
	if current.InitialMargin == "" {
		current.InitialMargin = "0"
	}
	if current.RealizedPnL == "" {
		current.RealizedPnL = "0"
	}
	if current.FundingAccrual == "" {
		current.FundingAccrual = "0"
	}
	currentQty := decimalx.MustFromString(current.Qty)
	currentAvg := decimalx.MustFromString(current.AvgEntryPrice)
	newQty := currentQty.Add(fillQty)
	if current.PositionID == "" {
		current = Position{
			PositionID:        idgen.NewID("pos"),
			UserID:            userID,
			SymbolID:          market.SymbolID,
			Side:              positionSide,
			Qty:               "0",
			AvgEntryPrice:     "0",
			MarkPrice:         market.MarkPrice,
			Notional:          "0",
			InitialMargin:     "0",
			MaintenanceMargin: "0",
			RealizedPnL:       "0",
			UnrealizedPnL:     "0",
			FundingAccrual:    "0",
			LiquidationPrice:  "0",
			BankruptcyPrice:   "0",
			Status:            PositionStatusOpen,
			CreatedAt:         now,
		}
	}

	if currentQty.IsZero() {
		currentAvg = fillPrice
	} else {
		currentAvg = currentQty.Mul(currentAvg).Add(fillQty.Mul(fillPrice)).Div(newQty)
	}
	sign := decimalx.MustFromString("1")
	if positionSide == PositionSideShort {
		sign = decimalx.MustFromString("-1")
	}
	mark := decimalx.MustFromString(market.MarkPrice)
	notional := newQty.Mul(mark).Mul(decimalx.MustFromString(market.ContractMultiplier))
	unrealized := sign.Mul(newQty).Mul(mark.Sub(currentAvg)).Mul(decimalx.MustFromString(market.ContractMultiplier))
	current.PositionID = current.PositionID
	current.UserID = userID
	current.SymbolID = market.SymbolID
	current.Side = positionSide
	current.Qty = newQty.String()
	current.AvgEntryPrice = currentAvg.String()
	current.MarkPrice = mark.String()
	current.Notional = notional.String()
	current.InitialMargin = decimalx.MustFromString(current.InitialMargin).Add(fillMargin).String()
	current.MaintenanceMargin = notional.Mul(decimalx.MustFromString(market.MaintenanceMarginRate)).String()
	current.UnrealizedPnL = unrealized.String()
	current.Status = PositionStatusOpen
	current.UpdatedAt = now
	if current.CreatedAt.IsZero() {
		current.CreatedAt = now
	}
	return current
}

func applyCloseFill(current Position, market TradableSymbol, closeQty decimalx.Decimal, closePrice decimalx.Decimal, releasedMargin decimalx.Decimal, realizedPnL decimalx.Decimal, now time.Time) Position {
	currentQty := decimalx.MustFromString(current.Qty)
	remainingQty := currentQty.Sub(closeQty)
	currentInitialMargin := decimalx.MustFromString(current.InitialMargin)
	currentRealized := decimalx.MustFromString(current.RealizedPnL)
	entryPrice := decimalx.MustFromString(current.AvgEntryPrice)
	mark := decimalx.MustFromString(market.MarkPrice)
	multiplier := decimalx.MustFromString(market.ContractMultiplier)
	if remainingQty.LessThan(decimalx.MustFromString("0")) {
		remainingQty = decimalx.MustFromString("0")
	}
	current.Qty = remainingQty.String()
	current.InitialMargin = currentInitialMargin.Sub(releasedMargin).String()
	current.RealizedPnL = currentRealized.Add(realizedPnL).String()
	current.MarkPrice = mark.String()
	if remainingQty.IsZero() {
		current.Notional = "0"
		current.MaintenanceMargin = "0"
		current.UnrealizedPnL = "0"
		current.Status = PositionStatusClosed
		current.UpdatedAt = now
		return current
	}
	sign := decimalx.MustFromString("1")
	if current.Side == PositionSideShort {
		sign = decimalx.MustFromString("-1")
	}
	notional := remainingQty.Mul(mark).Mul(multiplier)
	unrealized := sign.Mul(remainingQty).Mul(mark.Sub(entryPrice)).Mul(multiplier)
	current.Notional = notional.String()
	current.MaintenanceMargin = notional.Mul(decimalx.MustFromString(market.MaintenanceMarginRate)).String()
	current.UnrealizedPnL = unrealized.String()
	current.Status = PositionStatusOpen
	current.UpdatedAt = now
	return current
}

func isStepAligned(value decimalx.Decimal, step decimalx.Decimal) bool {
	if !step.GreaterThan(decimalx.MustFromString("0")) {
		return false
	}
	return value.Mod(step).IsZero()
}

func normalizeSide(side string) string {
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case "BUY":
		return "BUY"
	case "SELL":
		return "SELL"
	default:
		return ""
	}
}

func orderSideToPositionSide(side string) string {
	if side == "BUY" {
		return PositionSideLong
	}
	return PositionSideShort
}

func normalizeTimeInForce(tif string) string {
	switch strings.ToUpper(strings.TrimSpace(tif)) {
	case "", "GTC":
		return "GTC"
	default:
		return strings.ToUpper(strings.TrimSpace(tif))
	}
}

func normalizePositionEffect(positionEffect string, reduceOnly bool) string {
	if reduceOnly {
		return PositionEffectReduce
	}
	switch strings.ToUpper(strings.TrimSpace(positionEffect)) {
	case PositionEffectOpen:
		return PositionEffectOpen
	case PositionEffectReduce:
		return PositionEffectReduce
	case PositionEffectClose:
		return PositionEffectClose
	default:
		return ""
	}
}

func canTradeUnderSymbolStatus(symbolStatus string, positionEffect string) bool {
	switch symbolStatus {
	case "TRADING":
		return true
	case "REDUCE_ONLY":
		return positionEffect == PositionEffectReduce || positionEffect == PositionEffectClose
	default:
		return false
	}
}

func closingTargetPositionSide(orderSide string) string {
	if orderSide == "SELL" {
		return PositionSideLong
	}
	return PositionSideShort
}

func realizedPnLDelta(positionSide string, qty decimalx.Decimal, closePrice decimalx.Decimal, entryPrice decimalx.Decimal, multiplier decimalx.Decimal) decimalx.Decimal {
	sign := decimalx.MustFromString("1")
	if positionSide == PositionSideShort {
		sign = decimalx.MustFromString("-1")
	}
	return sign.Mul(qty).Mul(closePrice.Sub(entryPrice)).Mul(multiplier)
}

func proportionalAmount(total decimalx.Decimal, part decimalx.Decimal, whole decimalx.Decimal) decimalx.Decimal {
	if whole.IsZero() {
		return decimalx.MustFromString("0")
	}
	return total.Mul(part).Div(whole)
}

func isNotFound(err error) bool {
	return errors.Is(err, errorsx.ErrNotFound)
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}
