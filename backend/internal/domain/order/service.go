package order

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	ledgerdomain "github.com/xiaobao/rgperp/backend/internal/domain/ledger"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/exposurex"
	"github.com/xiaobao/rgperp/backend/internal/pkg/marketsession"
	"github.com/xiaobao/rgperp/backend/internal/pkg/positionrisk"
)

const (
	rejectReasonImmediateLiquidation = "WOULD_ENTER_LIQUIDATION_IMMEDIATELY"
	rejectReasonPositionLiquidating  = "POSITION_IS_LIQUIDATING"
)

type ServiceConfig struct {
	Asset                  string
	TakerFeeRate           string
	MakerFeeRate           string
	DefaultMaxSlippageBps  int
	MaxMarketDataAge       time.Duration
	NetExposureHardLimit   string
	MaxExposureSlippageBps int
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
	risk     PostTradeRiskProcessor
	runtime  RuntimeConfigProvider
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
	if cfg.Asset == "" || cfg.TakerFeeRate == "" || cfg.MakerFeeRate == "" || cfg.DefaultMaxSlippageBps <= 0 || cfg.MaxMarketDataAge <= 0 || cfg.NetExposureHardLimit == "" || cfg.MaxExposureSlippageBps < 0 {
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

func (s *Service) SetPostTradeRiskProcessor(processor PostTradeRiskProcessor) {
	s.risk = processor
}

func (s *Service) SetRuntimeConfigProvider(provider RuntimeConfigProvider) {
	s.runtime = provider
}

func (s *Service) CreateOrder(ctx context.Context, input CreateOrderInput) (Order, error) {
	if input.UserID == 0 || input.ClientOrderID == "" || input.Symbol == "" || input.Side == "" || input.PositionEffect == "" || input.Type == "" || input.Qty == "" {
		return Order{}, fmt.Errorf("%w: missing order fields", errorsx.ErrInvalidArgument)
	}
	input.MarginMode = normalizeMarginMode(input.MarginMode)
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = input.ClientOrderID
	}
	if existing, err := s.repo.GetByUserClientOrderID(ctx, input.UserID, input.ClientOrderID); err == nil {
		if existing.Symbol == "" {
			existing.Symbol = input.Symbol
		}
		return existing, nil
	} else if err != nil && !isNotFound(err) {
		return Order{}, err
	}

	market, err := s.markets.GetTradableSymbol(ctx, input.Symbol)
	if err != nil {
		return Order{}, err
	}
	runtimeCfg := s.currentRuntimeConfig(input.Symbol)
	positionEffect := normalizePositionEffect(input.PositionEffect, input.ReduceOnly)
	if positionEffect == "" {
		return Order{}, fmt.Errorf("%w: unsupported position_effect", errorsx.ErrInvalidArgument)
	}
	if runtimeCfg.GlobalReadOnly {
		return Order{}, fmt.Errorf("%w: system is read only", errorsx.ErrForbidden)
	}
	effectiveStatus := s.effectiveTradableStatus(market, runtimeCfg, s.clock.Now().UTC())
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
	orderType := normalizeOrderType(input.Type)
	if orderType == "" {
		return Order{}, fmt.Errorf("%w: unsupported order type", errorsx.ErrInvalidArgument)
	}

	maxSlippageBps := input.MaxSlippageBps
	if maxSlippageBps <= 0 {
		maxSlippageBps = runtimeCfg.DefaultMaxSlippageBps
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
	var triggerPrice *decimalx.Decimal
	if input.TriggerPrice != nil {
		parsed, err := decimalx.NewFromString(*input.TriggerPrice)
		if err != nil {
			return Order{}, err
		}
		if !parsed.GreaterThan(decimalx.MustFromString("0")) {
			return Order{}, fmt.Errorf("%w: trigger price must be positive", errorsx.ErrInvalidArgument)
		}
		if !isStepAligned(parsed, decimalx.MustFromString(market.TickSize)) {
			return Order{}, fmt.Errorf("%w: trigger price does not match tick size", errorsx.ErrInvalidArgument)
		}
		triggerPrice = &parsed
	}
	if isTriggerOrderType(orderType) && triggerPrice == nil {
		return Order{}, fmt.Errorf("%w: trigger price is required", errorsx.ErrInvalidArgument)
	}
	if !isTriggerOrderType(orderType) && triggerPrice != nil {
		return Order{}, fmt.Errorf("%w: trigger price is only allowed for trigger orders", errorsx.ErrInvalidArgument)
	}

	orderID := s.idgen.NewID("ord")
	now := s.clock.Now().UTC()

	order := Order{
		OrderID:             orderID,
		ClientOrderID:       input.ClientOrderID,
		UserID:              input.UserID,
		SymbolID:            market.SymbolID,
		Symbol:              market.Symbol,
		Side:                side,
		PositionEffect:      positionEffect,
		Type:                orderType,
		TimeInForce:         normalizeTimeInForce(input.TimeInForce),
		Qty:                 qty.String(),
		FilledQty:           "0",
		AvgFillPrice:        "0",
		Leverage:            "1",
		MarginMode:          input.MarginMode,
		ReduceOnly:          input.ReduceOnly,
		MaxSlippageBps:      maxSlippageBps,
		Status:              OrderStatusResting,
		FrozenInitialMargin: "0",
		FrozenFee:           "0",
		FrozenMargin:        "0",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if input.Price != nil {
		price := limitPrice.String()
		order.Price = &price
	}
	if input.TriggerPrice != nil {
		trigger := triggerPrice.String()
		order.TriggerPrice = &trigger
	}

	err = s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		tradeAccounts, err := s.accounts.ResolveTradeAccounts(txCtx, input.UserID, s.cfg.Asset)
		if err != nil {
			return err
		}
		if err := s.repo.LockSymbolForUpdate(txCtx, market.SymbolID); err != nil {
			return err
		}

		if positionEffect == PositionEffectOpen {
			availableBalance, err := s.lockOpenTradeBalances(txCtx, tradeAccounts)
			if err != nil {
				return err
			}
			if err := s.assertCanOpenRisk(txCtx, input.UserID); err != nil {
				return err
			}
			exposure, err := s.repo.GetSymbolExposureForUpdate(txCtx, market.SymbolID)
			if err != nil {
				return err
			}
			currentPosition, err := s.loadPosition(txCtx, input.UserID, market.SymbolID, orderSideToPositionSide(side), input.MarginMode)
			if err != nil {
				return err
			}
			if err := ensurePositionCanAcceptOpenRisk(currentPosition); err != nil {
				return err
			}
			_, executionPrice, leverage, holdInitialMargin, holdFee, err := s.resolveOpenRiskPricing(market, exposure, currentPosition, side, orderType, limitPrice, triggerPrice, qty, input.Leverage, maxSlippageBps, runtimeCfg)
			if err != nil {
				return err
			}
			order.Leverage = leverage.String()
			feeRate := decimalx.MustFromString(runtimeCfg.TakerFeeRate)
			if err := s.ensureSufficientAvailableBalance(availableBalance, holdInitialMargin.Add(holdFee)); err != nil {
				return err
			}
			created, err := s.createOpenOrder(txCtx, input, market, order, currentPosition, exposure, qty, executionPrice, holdInitialMargin, holdFee, feeRate, runtimeCfg, now, tradeAccounts.UserWalletAccountID, tradeAccounts.UserOrderMarginAccountID, tradeAccounts.UserPositionMarginAccountID, tradeAccounts.TradingFeeAccountID)
			if err != nil {
				return err
			}
			order = created
			return nil
		}
		referencePrice, executionPrice, err := s.resolvePrices(market, side, orderType, limitPrice, triggerPrice, maxSlippageBps, 0)
		if err != nil {
			return err
		}
		notional := qty.Mul(referencePrice).Mul(decimalx.MustFromString(market.ContractMultiplier))
		if notional.LessThan(decimalx.MustFromString(market.MinNotional)) {
			return fmt.Errorf("%w: notional below minimum", errorsx.ErrInvalidArgument)
		}
		feeRate := decimalx.MustFromString(runtimeCfg.TakerFeeRate)
		created, err := s.createCloseOrder(txCtx, input, market, order, qty, executionPrice, feeRate, runtimeCfg, now, tradeAccounts.UserWalletAccountID, tradeAccounts.UserPositionMarginAccountID, tradeAccounts.SystemPoolAccountID, tradeAccounts.TradingFeeAccountID)
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
	currentPosition Position,
	exposure SymbolExposure,
	qty decimalx.Decimal,
	executionPrice decimalx.Decimal,
	holdInitialMargin decimalx.Decimal,
	holdFee decimalx.Decimal,
	feeRate decimalx.Decimal,
	runtimeCfg RuntimeConfig,
	now time.Time,
	walletAccountID uint64,
	orderMarginAccountID uint64,
	positionMarginAccountID uint64,
	tradingFeeAccountID uint64,
) (Order, error) {
	if err := ensurePositionCanAcceptOpenRisk(currentPosition); err != nil {
		return Order{}, err
	}
	if order.Type == OrderTypeLimit {
		fillPrice, executable, err := s.resolveOpenLimitFillPrice(market, exposure, order.Side, order.Qty, decimalx.MustFromString(derefString(order.Price)), runtimeCfg)
		if err != nil {
			return Order{}, err
		}
		if executable {
			if s.wouldOpenPositionLiquidateImmediately(currentPosition, input.UserID, market, order.Side, order.MarginMode, qty, fillPrice, holdInitialMargin, runtimeCfg, now) {
				return Order{}, fmt.Errorf("%w: order would enter liquidation immediately", errorsx.ErrInvalidArgument)
			}
			return s.fillNewOpenOrder(txCtx, input, market, order, currentPosition, qty, fillPrice, holdInitialMargin, feeRate, holdInitialMargin.Add(holdFee), runtimeCfg, now, walletAccountID, orderMarginAccountID, positionMarginAccountID, tradingFeeAccountID)
		}
		holdAmount := holdInitialMargin.Add(holdFee)
		if err := s.postOpenOrderHold(txCtx, input, order, holdAmount, now, walletAccountID, orderMarginAccountID); err != nil {
			return Order{}, err
		}
		order.FrozenInitialMargin = holdInitialMargin.String()
		order.FrozenFee = holdFee.String()
		order.FrozenMargin = holdAmount.String()
		if err := s.repo.CreateOrder(txCtx, order); err != nil {
			return Order{}, err
		}
		if err := s.publishOrderAccepted(txCtx, order, now); err != nil {
			return Order{}, err
		}
		return order, nil
	}

	if isTriggerOrderType(order.Type) {
		holdAmount := holdInitialMargin.Add(holdFee)
		if err := s.postOpenOrderHold(txCtx, input, order, holdAmount, now, walletAccountID, orderMarginAccountID); err != nil {
			return Order{}, err
		}
		order.FrozenInitialMargin = holdInitialMargin.String()
		order.FrozenFee = holdFee.String()
		order.FrozenMargin = holdAmount.String()
		order.Status = OrderStatusTriggerWait
		if err := s.repo.CreateOrder(txCtx, order); err != nil {
			return Order{}, err
		}
		if err := s.publishOrderAccepted(txCtx, order, now); err != nil {
			return Order{}, err
		}
		return order, nil
	}

	if s.wouldOpenPositionLiquidateImmediately(currentPosition, input.UserID, market, order.Side, order.MarginMode, qty, executionPrice, holdInitialMargin, runtimeCfg, now) {
		return Order{}, fmt.Errorf("%w: order would enter liquidation immediately", errorsx.ErrInvalidArgument)
	}
	return s.fillNewOpenOrder(txCtx, input, market, order, currentPosition, qty, executionPrice, holdInitialMargin, feeRate, holdInitialMargin.Add(holdFee), runtimeCfg, now, walletAccountID, orderMarginAccountID, positionMarginAccountID, tradingFeeAccountID)
}

func (s *Service) fillNewOpenOrder(
	txCtx context.Context,
	input CreateOrderInput,
	market TradableSymbol,
	order Order,
	currentPosition Position,
	fillQty decimalx.Decimal,
	fillPrice decimalx.Decimal,
	fillMargin decimalx.Decimal,
	feeRate decimalx.Decimal,
	holdAmount decimalx.Decimal,
	runtimeCfg RuntimeConfig,
	now time.Time,
	walletAccountID uint64,
	orderMarginAccountID uint64,
	positionMarginAccountID uint64,
	tradingFeeAccountID uint64,
) (Order, error) {
	if err := ensurePositionCanAcceptOpenRisk(currentPosition); err != nil {
		return Order{}, err
	}
	fillNotional := fillQty.Mul(fillPrice).Mul(decimalx.MustFromString(market.ContractMultiplier))
	fillFee := fillNotional.Mul(feeRate)
	refund := holdAmount.Sub(fillMargin.Add(fillFee))
	if refund.LessThan(decimalx.MustFromString("0")) {
		return Order{}, fmt.Errorf("%w: execution exceeded held margin", errorsx.ErrConflict)
	}

	if err := s.postOpenOrderHold(txCtx, input, order, holdAmount, now, walletAccountID, orderMarginAccountID); err != nil {
		return Order{}, err
	}

	order.Status = OrderStatusFilled
	order.FilledQty = fillQty.String()
	order.AvgFillPrice = fillPrice.String()
	order.FrozenInitialMargin = "0"
	order.FrozenFee = "0"
	order.FrozenMargin = "0"
	if err := s.repo.CreateOrder(txCtx, order); err != nil {
		return Order{}, err
	}
	if err := s.publishOrderAccepted(txCtx, order, now); err != nil {
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
	position, err := s.repo.GetPositionForUpdate(txCtx, input.UserID, market.SymbolID, positionSide, input.MarginMode)
	if err != nil && !isNotFound(err) {
		return Order{}, err
	}
	position = applyOpenFill(position, input.UserID, market, positionSide, input.MarginMode, fillQty, fillPrice, fillMargin, runtimeCfg, now, s.idgen)
	if err := s.repo.UpsertPosition(txCtx, position); err != nil {
		return Order{}, err
	}
	if err := s.publishExecutionEvents(txCtx, order, fill, position, market.Symbol, now); err != nil {
		return Order{}, err
	}
	if err := s.handlePostTradeRisk(txCtx, input.UserID, input.TraceID, order.OrderID); err != nil {
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
	runtimeCfg RuntimeConfig,
	now time.Time,
	walletAccountID uint64,
	positionMarginAccountID uint64,
	systemPoolAccountID uint64,
	tradingFeeAccountID uint64,
) (Order, error) {
	targetPositionSide := closingTargetPositionSide(order.Side)
	position, err := s.repo.GetPositionForUpdate(txCtx, input.UserID, market.SymbolID, targetPositionSide, input.MarginMode)
	if err != nil {
		return Order{}, err
	}
	if err := ensurePositionCanBeUserManaged(position); err != nil {
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
	order.Qty = closeQty.String()

	if isTriggerOrderType(order.Type) {
		order.Status = OrderStatusTriggerWait
		if err := s.repo.CreateOrder(txCtx, order); err != nil {
			return Order{}, err
		}
		if err := s.publishOrderAccepted(txCtx, order, now); err != nil {
			return Order{}, err
		}
		return order, nil
	}
	if order.Type != OrderTypeMarket {
		return Order{}, fmt.Errorf("%w: close/reduce currently only supports MARKET", errorsx.ErrInvalidArgument)
	}

	entryPrice := decimalx.MustFromString(position.AvgEntryPrice)
	multiplier := decimalx.MustFromString(market.ContractMultiplier)
	closeNotional := closeQty.Mul(executionPrice).Mul(multiplier)
	closeFee := closeNotional.Mul(feeRate)
	releasedMargin := closeMarginRelease(position, market, closeQty)
	realizedPnL := realizedPnLDelta(targetPositionSide, closeQty, executionPrice, entryPrice, multiplier)
	userWalletDelta := releasedMargin.Add(realizedPnL).Sub(closeFee)

	order.Status = OrderStatusFilled
	order.FilledQty = closeQty.String()
	order.AvgFillPrice = executionPrice.String()
	order.FrozenInitialMargin = "0"
	order.FrozenFee = "0"
	order.FrozenMargin = "0"
	if err := s.repo.CreateOrder(txCtx, order); err != nil {
		return Order{}, err
	}
	if err := s.publishOrderAccepted(txCtx, order, now); err != nil {
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

	position = applyCloseFill(position, market, closeQty, executionPrice, releasedMargin, realizedPnL, runtimeCfg, now)
	if err := s.repo.UpsertPosition(txCtx, position); err != nil {
		return Order{}, err
	}
	if err := s.publishExecutionEvents(txCtx, order, fill, position, market.Symbol, now); err != nil {
		return Order{}, err
	}
	if err := s.handlePostTradeRisk(txCtx, input.UserID, input.TraceID, order.OrderID); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Service) CancelOrder(ctx context.Context, input CancelOrderInput) error {
	if input.UserID == 0 || input.OrderID == "" {
		return fmt.Errorf("%w: missing cancel order fields", errorsx.ErrInvalidArgument)
	}
	if s.currentRuntimeConfig("").GlobalReadOnly {
		return fmt.Errorf("%w: system is read only", errorsx.ErrForbidden)
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = input.OrderID
	}
	now := s.clock.Now().UTC()
	return s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
		tradeAccounts, err := s.accounts.ResolveTradeAccounts(txCtx, input.UserID, s.cfg.Asset)
		if err != nil {
			return err
		}
		if _, err := s.lockOpenTradeBalances(txCtx, tradeAccounts); err != nil {
			return err
		}
		order, err := s.repo.GetByUserOrderIDForUpdate(txCtx, input.UserID, input.OrderID)
		if err != nil {
			return err
		}
		if order.Status == OrderStatusCanceled {
			return nil
		}
		if order.Status != OrderStatusResting && order.Status != OrderStatusTriggerWait {
			return fmt.Errorf("%w: order is not cancelable", errorsx.ErrConflict)
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
		order.FrozenInitialMargin = "0"
		order.FrozenFee = "0"
		order.FrozenMargin = "0"
		order.UpdatedAt = now
		if err := s.repo.UpdateOrder(txCtx, order); err != nil {
			return err
		}
		return s.publishOrderCanceled(txCtx, order, now)
	})
}

func (s *Service) ExecuteTriggerOrders(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		return 0, fmt.Errorf("%w: invalid trigger execution batch", errorsx.ErrInvalidArgument)
	}
	orders, err := s.repo.ListTriggerWaitingOrders(ctx, batchSize)
	if err != nil {
		return 0, err
	}
	executed := 0
	for _, candidate := range orders {
		runtimeCfg := s.currentRuntimeConfig(candidate.Symbol)
		market, err := s.markets.GetTradableSymbol(ctx, candidate.Symbol)
		if err != nil {
			return executed, err
		}
		if market.SnapshotTS.IsZero() || s.clock.Now().UTC().Sub(market.SnapshotTS) > runtimeCfg.MaxMarketDataAge {
			continue
		}
		if !triggerSatisfied(candidate.Type, candidate.Side, decimalx.MustFromString(derefString(candidate.TriggerPrice)), decimalx.MustFromString(market.MarkPrice)) {
			continue
		}

		err = s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
			order, err := s.repo.GetByUserOrderIDForUpdate(txCtx, candidate.UserID, candidate.OrderID)
			if err != nil {
				return err
			}
			if order.Status != OrderStatusTriggerWait || !isTriggerOrderType(order.Type) {
				return nil
			}
			if order.Symbol == "" {
				order.Symbol = candidate.Symbol
			}
			if err := s.repo.LockSymbolForUpdate(txCtx, order.SymbolID); err != nil {
				return err
			}
			lockedMarket, err := s.markets.GetTradableSymbol(txCtx, order.Symbol)
			if err != nil {
				return err
			}
			orderRuntimeCfg := s.currentRuntimeConfig(order.Symbol)
			if lockedMarket.SnapshotTS.IsZero() || s.clock.Now().UTC().Sub(lockedMarket.SnapshotTS) > orderRuntimeCfg.MaxMarketDataAge {
				return nil
			}
			lockedStatus := s.effectiveTradableStatus(lockedMarket, orderRuntimeCfg, s.clock.Now().UTC())
			if !canTradeUnderSymbolStatus(lockedStatus, order.PositionEffect) {
				return nil
			}
			if !triggerSatisfied(order.Type, order.Side, decimalx.MustFromString(derefString(order.TriggerPrice)), decimalx.MustFromString(lockedMarket.MarkPrice)) {
				return nil
			}
			if order.PositionEffect == PositionEffectOpen {
				return s.executeTriggeredOpenOrder(txCtx, order, lockedMarket, s.clock.Now().UTC(), &executed)
			}
			return s.executeTriggeredCloseOrder(txCtx, order, lockedMarket, s.clock.Now().UTC(), &executed)
		})
		if err != nil {
			return executed, err
		}
	}
	return executed, nil
}

func (s *Service) ExecuteRestingOrders(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		return 0, fmt.Errorf("%w: invalid resting execution batch", errorsx.ErrInvalidArgument)
	}
	orders, err := s.repo.ListRestingOpenLimitOrders(ctx, batchSize)
	if err != nil {
		return 0, err
	}
	executed := 0
	for _, candidate := range orders {
		runtimeCfg := s.currentRuntimeConfig(candidate.Symbol)
		market, err := s.markets.GetTradableSymbol(ctx, candidate.Symbol)
		if err != nil {
			return executed, err
		}
		effectiveStatus := s.effectiveTradableStatus(market, runtimeCfg, s.clock.Now().UTC())
		if !canTradeUnderSymbolStatus(effectiveStatus, candidate.PositionEffect) {
			continue
		}
		limitPrice := decimalx.MustFromString(derefString(candidate.Price))
		if _, ok := executableLimitPrice(candidate.Side, limitPrice, decimalx.MustFromString(market.BestBid), decimalx.MustFromString(market.BestAsk)); !ok {
			continue
		}

		err = s.txm.WithinTransaction(ctx, func(txCtx context.Context) error {
			order, err := s.repo.GetByUserOrderIDForUpdate(txCtx, candidate.UserID, candidate.OrderID)
			if err != nil {
				return err
			}
			if order.Status != OrderStatusResting || order.Type != OrderTypeLimit || order.PositionEffect != PositionEffectOpen {
				return nil
			}
			if order.Symbol == "" {
				order.Symbol = candidate.Symbol
			}
			if err := s.repo.LockSymbolForUpdate(txCtx, order.SymbolID); err != nil {
				return err
			}
			lockedMarket, err := s.markets.GetTradableSymbol(txCtx, order.Symbol)
			if err != nil {
				return err
			}
			orderRuntimeCfg := s.currentRuntimeConfig(order.Symbol)
			lockedStatus := s.effectiveTradableStatus(lockedMarket, orderRuntimeCfg, s.clock.Now().UTC())
			if !canTradeUnderSymbolStatus(lockedStatus, order.PositionEffect) {
				return nil
			}
			tradeAccounts, err := s.accounts.ResolveTradeAccounts(txCtx, order.UserID, s.cfg.Asset)
			if err != nil {
				return err
			}
			if _, err := s.lockOpenTradeBalances(txCtx, tradeAccounts); err != nil {
				return err
			}
			if err := s.assertCanOpenRisk(txCtx, order.UserID); err != nil {
				if errors.Is(err, errorsx.ErrForbidden) {
					return nil
				}
				return err
			}
			exposure, err := s.repo.GetSymbolExposureForUpdate(txCtx, lockedMarket.SymbolID)
			if err != nil {
				return err
			}
			currentLimit := decimalx.MustFromString(derefString(order.Price))
			fillPrice, executable, err := s.resolveOpenLimitFillPrice(lockedMarket, exposure, order.Side, order.Qty, currentLimit, orderRuntimeCfg)
			if err != nil {
				return err
			}
			if !executable {
				return nil
			}
			qty := decimalx.MustFromString(order.Qty)
			currentPosition, err := s.loadPosition(txCtx, order.UserID, lockedMarket.SymbolID, orderSideToPositionSide(order.Side), order.MarginMode)
			if err != nil {
				return err
			}
			feeRate := decimalx.MustFromString(orderRuntimeCfg.MakerFeeRate)
			updated, err := s.executeExistingOpenLimitOrder(txCtx, order, lockedMarket, currentPosition, qty, fillPrice, feeRate, orderRuntimeCfg, s.clock.Now().UTC(), tradeAccounts)
			if err != nil {
				return err
			}
			if updated.OrderID != "" && updated.Status == OrderStatusFilled {
				executed++
			}
			return nil
		})
		if err != nil {
			return executed, err
		}
	}
	return executed, nil
}

func (s *Service) resolveOpenRiskPricing(market TradableSymbol, exposure SymbolExposure, currentPosition Position, side string, orderType string, limitPrice *decimalx.Decimal, triggerPrice *decimalx.Decimal, qty decimalx.Decimal, requestedLeverage *string, maxSlippageBps int, runtimeCfg RuntimeConfig) (decimalx.Decimal, decimalx.Decimal, decimalx.Decimal, decimalx.Decimal, decimalx.Decimal, error) {
	referencePrice, executionPrice, err := s.resolvePrices(market, side, orderType, limitPrice, triggerPrice, maxSlippageBps, 0)
	if err != nil {
		return decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, err
	}
	currentSigned := exposurex.SignedNetNotional(exposure.LongQty, exposure.ShortQty, market.MarkPrice, market.ContractMultiplier)
	deltaSigned := exposurex.SignedDeltaNotional(side, qty.String(), referencePrice.String(), market.ContractMultiplier)
	if exposurex.ExceedsHardLimit(currentSigned, deltaSigned, runtimeCfg.NetExposureHardLimit) {
		return decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, fmt.Errorf("%w: symbol net exposure hard limit reached", errorsx.ErrForbidden)
	}
	adjustmentBps := exposurex.DirectionAdjustmentBps(currentSigned, side, runtimeCfg.NetExposureHardLimit, runtimeCfg.MaxExposureSlippageBps)
	referencePrice, executionPrice, err = s.resolvePrices(market, side, orderType, limitPrice, triggerPrice, maxSlippageBps, adjustmentBps)
	if err != nil {
		return decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, err
	}
	notional := qty.Mul(referencePrice).Mul(decimalx.MustFromString(market.ContractMultiplier))
	if notional.LessThan(decimalx.MustFromString(market.MinNotional)) {
		return decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, fmt.Errorf("%w: notional below minimum", errorsx.ErrInvalidArgument)
	}
	feeRate := decimalx.MustFromString(runtimeCfg.TakerFeeRate)
	leverage, err := resolveOrderLeverage(market, notional, requestedLeverage, runtimeCfg.MaxLeverage)
	if err != nil {
		return decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, decimalx.Decimal{}, err
	}
	holdInitialMargin := requiredInitialMarginForLeverage(notional, leverage)
	holdFee := notional.Mul(feeRate)
	return referencePrice, executionPrice, leverage, holdInitialMargin, holdFee, nil
}

func (s *Service) resolvePrices(market TradableSymbol, side string, orderType string, limitPrice *decimalx.Decimal, triggerPrice *decimalx.Decimal, maxSlippageBps int, exposureAdjustmentBps int) (decimalx.Decimal, decimalx.Decimal, error) {
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
		execution = adjustExecutionPrice(side, execution, exposureAdjustmentBps)
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
	case OrderTypeStopMarket, OrderTypeTakeProfitMarket:
		if triggerPrice == nil {
			return decimalx.Decimal{}, decimalx.Decimal{}, fmt.Errorf("%w: trigger price is required", errorsx.ErrInvalidArgument)
		}
		highest := maxDecimal(*triggerPrice, mark, bestBid, bestAsk)
		slippageBuffer := highest.Mul(decimalx.MustFromString(fmt.Sprintf("%d", maxSlippageBps))).Div(decimalx.MustFromString("10000"))
		return highest.Add(slippageBuffer), decimalx.Decimal{}, nil
	default:
		return decimalx.Decimal{}, decimalx.Decimal{}, fmt.Errorf("%w: unsupported order type", errorsx.ErrInvalidArgument)
	}
}

func (s *Service) executeExistingOpenLimitOrder(
	txCtx context.Context,
	order Order,
	market TradableSymbol,
	currentPosition Position,
	fillQty decimalx.Decimal,
	fillPrice decimalx.Decimal,
	feeRate decimalx.Decimal,
	runtimeCfg RuntimeConfig,
	now time.Time,
	tradeAccounts TradeAccounts,
) (Order, error) {
	holdInitialMargin, holdFee := orderFrozenHoldComponents(order, feeRate)
	holdAmount := holdInitialMargin.Add(holdFee)
	if err := ensurePositionCanAcceptOpenRisk(currentPosition); err != nil {
		return s.rejectOpenOrder(txCtx, order, tradeAccounts, rejectReasonPositionLiquidating, now)
	}
	fillNotional := fillQty.Mul(fillPrice).Mul(decimalx.MustFromString(market.ContractMultiplier))
	fillFee := fillNotional.Mul(feeRate)
	fillMargin := holdInitialMargin
	refund := holdAmount.Sub(fillMargin.Add(fillFee))
	if refund.LessThan(decimalx.MustFromString("0")) {
		return Order{}, fmt.Errorf("%w: execution exceeded held margin", errorsx.ErrConflict)
	}
	if s.wouldOpenPositionLiquidateImmediately(currentPosition, order.UserID, market, order.Side, order.MarginMode, fillQty, fillPrice, fillMargin, runtimeCfg, now) {
		return s.rejectOpenOrder(txCtx, order, tradeAccounts, rejectReasonImmediateLiquidation, now)
	}

	order.Status = OrderStatusFilled
	order.FilledQty = fillQty.String()
	order.AvgFillPrice = fillPrice.String()
	order.FrozenInitialMargin = "0"
	order.FrozenFee = "0"
	order.FrozenMargin = "0"
	order.UpdatedAt = now
	if err := s.repo.UpdateOrder(txCtx, order); err != nil {
		return Order{}, err
	}

	fillLedgerTxID := s.idgen.NewID("ldg")
	entries := []ledgerdomain.LedgerEntry{
		{AccountID: tradeAccounts.UserOrderMarginAccountID, UserID: uint64Ptr(order.UserID), Asset: s.cfg.Asset, Amount: fillMargin.Add(fillFee).Neg().String(), EntryType: "TRADE_OPEN_EXECUTION"},
		{AccountID: tradeAccounts.UserPositionMarginAccountID, UserID: uint64Ptr(order.UserID), Asset: s.cfg.Asset, Amount: fillMargin.String(), EntryType: "TRADE_POSITION_MARGIN"},
		{AccountID: tradeAccounts.TradingFeeAccountID, Asset: s.cfg.Asset, Amount: fillFee.String(), EntryType: "TRADE_FEE"},
	}
	if refund.GreaterThan(decimalx.MustFromString("0")) {
		entries[0].Amount = holdAmount.Neg().String()
		entries = append(entries, ledgerdomain.LedgerEntry{
			AccountID: tradeAccounts.UserWalletAccountID,
			UserID:    uint64Ptr(order.UserID),
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
			IdempotencyKey: order.OrderID + ":resting-fill",
			OperatorType:   "USER",
			OperatorID:     fmt.Sprintf("%d", order.UserID),
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
		UserID:     order.UserID,
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
	position, err := s.repo.GetPositionForUpdate(txCtx, order.UserID, market.SymbolID, positionSide, order.MarginMode)
	if err != nil && !isNotFound(err) {
		return Order{}, err
	}
	position = applyOpenFill(position, order.UserID, market, positionSide, order.MarginMode, fillQty, fillPrice, fillMargin, runtimeCfg, now, s.idgen)
	if err := s.repo.UpsertPosition(txCtx, position); err != nil {
		return Order{}, err
	}
	if err := s.publishExecutionEvents(txCtx, order, fill, position, market.Symbol, now); err != nil {
		return Order{}, err
	}
	if err := s.handlePostTradeRisk(txCtx, order.UserID, "", order.OrderID); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Service) executeTriggeredOpenOrder(txCtx context.Context, order Order, market TradableSymbol, now time.Time, executed *int) error {
	if order.Type != OrderTypeStopMarket && order.Type != OrderTypeTakeProfitMarket {
		return fmt.Errorf("%w: unsupported trigger open order type", errorsx.ErrInvalidArgument)
	}
	tradeAccounts, err := s.accounts.ResolveTradeAccounts(txCtx, order.UserID, s.cfg.Asset)
	if err != nil {
		return err
	}
	if _, err := s.lockOpenTradeBalances(txCtx, tradeAccounts); err != nil {
		return err
	}
	if err := s.assertCanOpenRisk(txCtx, order.UserID); err != nil {
		return err
	}
	exposure, err := s.repo.GetSymbolExposureForUpdate(txCtx, market.SymbolID)
	if err != nil {
		return err
	}
	currentPosition, err := s.loadPosition(txCtx, order.UserID, market.SymbolID, orderSideToPositionSide(order.Side), order.MarginMode)
	if err != nil {
		return err
	}
	if err := ensurePositionCanAcceptOpenRisk(currentPosition); err != nil {
		updatedOrder, rejectErr := s.rejectOpenOrder(txCtx, order, tradeAccounts, rejectReasonPositionLiquidating, now)
		if rejectErr != nil {
			return rejectErr
		}
		if updatedOrder.Status == OrderStatusFilled {
			*executed = *executed + 1
		}
		return nil
	}
	runtimeCfg := s.currentRuntimeConfig(market.Symbol)
	_, executionPrice, _, _, _, err := s.resolveOpenRiskPricing(market, exposure, currentPosition, order.Side, OrderTypeMarket, nil, nil, decimalx.MustFromString(order.Qty), stringPtr(order.Leverage), order.MaxSlippageBps, runtimeCfg)
	if err != nil {
		return err
	}
	qty := decimalx.MustFromString(order.Qty)
	feeRate := decimalx.MustFromString(runtimeCfg.TakerFeeRate)
	updatedOrder, err := s.executeExistingOpenLimitOrder(txCtx, order, market, currentPosition, qty, executionPrice, feeRate, runtimeCfg, now, tradeAccounts)
	if err != nil {
		return err
	}
	if updatedOrder.Status == OrderStatusFilled {
		*executed = *executed + 1
	}
	return nil
}

func (s *Service) executeTriggeredCloseOrder(txCtx context.Context, order Order, market TradableSymbol, now time.Time, executed *int) error {
	if order.Type != OrderTypeStopMarket && order.Type != OrderTypeTakeProfitMarket {
		return fmt.Errorf("%w: unsupported trigger close order type", errorsx.ErrInvalidArgument)
	}
	targetPositionSide := closingTargetPositionSide(order.Side)
	position, err := s.repo.GetPositionForUpdate(txCtx, order.UserID, market.SymbolID, targetPositionSide, order.MarginMode)
	if err != nil {
		if isNotFound(err) {
			order.Status = OrderStatusCanceled
			order.UpdatedAt = now
			return s.repo.UpdateOrder(txCtx, order)
		}
		return err
	}
	if err := ensurePositionCanBeUserManaged(position); err != nil {
		order.Status = OrderStatusCanceled
		order.UpdatedAt = now
		return s.repo.UpdateOrder(txCtx, order)
	}
	currentQty := decimalx.MustFromString(position.Qty)
	if !currentQty.GreaterThan(decimalx.MustFromString("0")) {
		order.Status = OrderStatusCanceled
		order.UpdatedAt = now
		return s.repo.UpdateOrder(txCtx, order)
	}
	closeQty := decimalx.MustFromString(order.Qty)
	if closeQty.GreaterThan(currentQty) {
		closeQty = currentQty
	}
	runtimeCfg := s.currentRuntimeConfig(market.Symbol)
	_, executionPrice, err := s.resolvePrices(market, order.Side, OrderTypeMarket, nil, nil, order.MaxSlippageBps, 0)
	if err != nil {
		return err
	}
	entryPrice := decimalx.MustFromString(position.AvgEntryPrice)
	multiplier := decimalx.MustFromString(market.ContractMultiplier)
	closeNotional := closeQty.Mul(executionPrice).Mul(multiplier)
	closeFee := closeNotional.Mul(decimalx.MustFromString(runtimeCfg.TakerFeeRate))
	releasedMargin := closeMarginRelease(position, market, closeQty)
	realizedPnL := realizedPnLDelta(targetPositionSide, closeQty, executionPrice, entryPrice, multiplier)

	order.Status = OrderStatusFilled
	order.FilledQty = closeQty.String()
	order.AvgFillPrice = executionPrice.String()
	order.FrozenInitialMargin = "0"
	order.FrozenFee = "0"
	order.FrozenMargin = "0"
	order.UpdatedAt = now
	if err := s.repo.UpdateOrder(txCtx, order); err != nil {
		return err
	}

	tradeAccounts, err := s.accounts.ResolveTradeAccounts(txCtx, order.UserID, s.cfg.Asset)
	if err != nil {
		return err
	}
	fillLedgerTxID := s.idgen.NewID("ldg")
	userWalletDelta := releasedMargin.Add(realizedPnL).Sub(closeFee)
	entries := []ledgerdomain.LedgerEntry{
		{AccountID: tradeAccounts.UserPositionMarginAccountID, UserID: uint64Ptr(order.UserID), Asset: s.cfg.Asset, Amount: releasedMargin.Neg().String(), EntryType: "TRADE_CLOSE_MARGIN_RELEASE"},
		{AccountID: tradeAccounts.UserWalletAccountID, UserID: uint64Ptr(order.UserID), Asset: s.cfg.Asset, Amount: userWalletDelta.String(), EntryType: "TRADE_CLOSE_SETTLEMENT"},
		{AccountID: tradeAccounts.TradingFeeAccountID, Asset: s.cfg.Asset, Amount: closeFee.String(), EntryType: "TRADE_FEE"},
	}
	if realizedPnL.GreaterThan(decimalx.MustFromString("0")) {
		entries = append(entries, ledgerdomain.LedgerEntry{
			AccountID: tradeAccounts.SystemPoolAccountID,
			Asset:     s.cfg.Asset,
			Amount:    realizedPnL.Neg().String(),
			EntryType: "TRADE_REALIZED_PNL",
		})
	} else if realizedPnL.LessThan(decimalx.MustFromString("0")) {
		entries = append(entries, ledgerdomain.LedgerEntry{
			AccountID: tradeAccounts.SystemPoolAccountID,
			Asset:     s.cfg.Asset,
			Amount:    realizedPnL.Abs().String(),
			EntryType: "TRADE_REALIZED_LOSS",
		})
	}
	if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
		LedgerTx: ledgerdomain.LedgerTx{
			ID:             fillLedgerTxID,
			EventID:        s.idgen.NewID("evt"),
			BizType:        "trade.fill.close.trigger",
			BizRefID:       order.OrderID,
			Asset:          s.cfg.Asset,
			IdempotencyKey: order.OrderID + ":trigger-fill",
			OperatorType:   "USER",
			OperatorID:     fmt.Sprintf("%d", order.UserID),
			Status:         "COMMITTED",
			CreatedAt:      now,
		},
		Entries: entries,
	}); err != nil {
		return err
	}
	fill := Fill{
		FillID:     s.idgen.NewID("fill"),
		OrderID:    order.OrderID,
		UserID:     order.UserID,
		SymbolID:   market.SymbolID,
		Side:       order.Side,
		Qty:        closeQty.String(),
		Price:      executionPrice.String(),
		FeeAmount:  closeFee.String(),
		LedgerTxID: fillLedgerTxID,
		CreatedAt:  now,
	}
	if err := s.repo.CreateFill(txCtx, fill); err != nil {
		return err
	}
	position = applyCloseFill(position, market, closeQty, executionPrice, releasedMargin, realizedPnL, runtimeCfg, now)
	if err := s.repo.UpsertPosition(txCtx, position); err != nil {
		return err
	}
	if err := s.publishExecutionEvents(txCtx, order, fill, position, market.Symbol, now); err != nil {
		return err
	}
	if err := s.handlePostTradeRisk(txCtx, order.UserID, "", order.OrderID); err != nil {
		return err
	}
	*executed = *executed + 1
	return nil
}

func (s *Service) publishOrderAccepted(ctx context.Context, order Order, now time.Time) error {
	payload := map[string]any{
		"order_id":              order.OrderID,
		"client_order_id":       order.ClientOrderID,
		"user_id":               order.UserID,
		"asset":                 s.cfg.Asset,
		"symbol":                order.Symbol,
		"side":                  order.Side,
		"type":                  order.Type,
		"position_effect":       order.PositionEffect,
		"qty":                   order.Qty,
		"frozen_initial_margin": order.FrozenInitialMargin,
		"frozen_fee":            order.FrozenFee,
		"frozen_margin":         order.FrozenMargin,
		"status":                order.Status,
		"reduce_only":           order.ReduceOnly,
	}
	if order.Price != nil {
		payload["price"] = *order.Price
	}
	if order.TriggerPrice != nil {
		payload["trigger_price"] = *order.TriggerPrice
	}
	return s.repo.CreateEvent(ctx, Event{
		EventID:       s.idgen.NewID("evt"),
		AggregateType: "order",
		AggregateID:   order.OrderID,
		EventType:     "trade.order.accepted",
		Payload:       payload,
		CreatedAt:     now,
	})
}

func (s *Service) publishOrderCanceled(ctx context.Context, order Order, now time.Time) error {
	return s.repo.CreateEvent(ctx, Event{
		EventID:       s.idgen.NewID("evt"),
		AggregateType: "order",
		AggregateID:   order.OrderID,
		EventType:     "trade.order.canceled",
		Payload: map[string]any{
			"order_id":              order.OrderID,
			"client_order_id":       order.ClientOrderID,
			"user_id":               order.UserID,
			"asset":                 s.cfg.Asset,
			"symbol":                order.Symbol,
			"status":                order.Status,
			"frozen_initial_margin": order.FrozenInitialMargin,
			"frozen_fee":            order.FrozenFee,
			"frozen_margin":         order.FrozenMargin,
		},
		CreatedAt: now,
	})
}

func (s *Service) publishOrderRejected(ctx context.Context, order Order, now time.Time) error {
	payload := map[string]any{
		"order_id":              order.OrderID,
		"client_order_id":       order.ClientOrderID,
		"user_id":               order.UserID,
		"asset":                 s.cfg.Asset,
		"symbol":                order.Symbol,
		"status":                order.Status,
		"frozen_initial_margin": order.FrozenInitialMargin,
		"frozen_fee":            order.FrozenFee,
		"frozen_margin":         order.FrozenMargin,
	}
	if order.RejectReason != nil {
		payload["reject_reason"] = *order.RejectReason
	}
	return s.repo.CreateEvent(ctx, Event{
		EventID:       s.idgen.NewID("evt"),
		AggregateType: "order",
		AggregateID:   order.OrderID,
		EventType:     "trade.order.rejected",
		Payload:       payload,
		CreatedAt:     now,
	})
}

func (s *Service) publishExecutionEvents(ctx context.Context, order Order, fill Fill, position Position, symbol string, now time.Time) error {
	if err := s.repo.CreateEvent(ctx, Event{
		EventID:       s.idgen.NewID("evt"),
		AggregateType: "fill",
		AggregateID:   fill.FillID,
		EventType:     "trade.fill.created",
		Payload: map[string]any{
			"fill_id":      fill.FillID,
			"order_id":     fill.OrderID,
			"user_id":      fill.UserID,
			"asset":        s.cfg.Asset,
			"symbol":       symbol,
			"side":         fill.Side,
			"qty":          fill.Qty,
			"price":        fill.Price,
			"fee_amount":   fill.FeeAmount,
			"position_id":  position.PositionID,
			"ledger_tx_id": fill.LedgerTxID,
		},
		CreatedAt: now,
	}); err != nil {
		return err
	}
	return s.repo.CreateEvent(ctx, Event{
		EventID:       s.idgen.NewID("evt"),
		AggregateType: "position",
		AggregateID:   position.PositionID,
		EventType:     "trade.position.updated",
		Payload: map[string]any{
			"order_id":           order.OrderID,
			"fill_id":            fill.FillID,
			"position_id":        position.PositionID,
			"user_id":            position.UserID,
			"asset":              s.cfg.Asset,
			"symbol":             symbol,
			"side":               position.Side,
			"qty":                position.Qty,
			"avg_entry_price":    position.AvgEntryPrice,
			"mark_price":         position.MarkPrice,
			"initial_margin":     position.InitialMargin,
			"maintenance_margin": position.MaintenanceMargin,
			"unrealized_pnl":     position.UnrealizedPnL,
			"status":             position.Status,
		},
		CreatedAt: now,
	})
}

func applyOpenFill(current Position, userID uint64, market TradableSymbol, positionSide string, marginMode string, fillQty decimalx.Decimal, fillPrice decimalx.Decimal, fillMargin decimalx.Decimal, runtimeCfg RuntimeConfig, now time.Time, idgen IDGenerator) Position {
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
			MarginMode:        marginMode,
			Qty:               "0",
			AvgEntryPrice:     "0",
			MarkPrice:         market.MarkPrice,
			Notional:          "0",
			Leverage:          "0",
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
	current.MarginMode = marginMode
	current.Qty = newQty.String()
	current.AvgEntryPrice = currentAvg.String()
	current.MarkPrice = mark.String()
	current.Notional = notional.String()
	current.InitialMargin = decimalx.MustFromString(current.InitialMargin).Add(fillMargin).String()
	current.Leverage = effectivePositionLeverage(newQty, currentAvg, decimalx.MustFromString(market.ContractMultiplier), decimalx.MustFromString(current.InitialMargin)).String()
	current.MaintenanceMargin = requiredMaintenanceMargin(market, notional, runtimeCfg.MaintenanceMarginUpliftRatio).String()
	current.UnrealizedPnL = unrealized.String()
	tier := selectOrderRiskTier(market, notional)
	maintenanceRate := effectiveMaintenanceRate(tier.MaintenanceRate, runtimeCfg.MaintenanceMarginUpliftRatio)
	liquidationPenaltyRate := effectiveLiquidationPenaltyRate(runtimeCfg, tier.LiquidationFeeRate)
	current.LiquidationPrice, current.BankruptcyPrice = positionrisk.ComputeDisplayPrices(
		current.Side,
		current.Qty,
		current.AvgEntryPrice,
		current.InitialMargin,
		maintenanceRate,
		liquidationPenaltyRate,
		market.ContractMultiplier,
		runtimeCfg.LiquidationExtraSlippageBps,
	)
	current.Status = PositionStatusOpen
	current.UpdatedAt = now
	if current.CreatedAt.IsZero() {
		current.CreatedAt = now
	}
	return current
}

func applyCloseFill(current Position, market TradableSymbol, closeQty decimalx.Decimal, closePrice decimalx.Decimal, releasedMargin decimalx.Decimal, realizedPnL decimalx.Decimal, runtimeCfg RuntimeConfig, now time.Time) Position {
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
		current.Leverage = "0"
		current.MaintenanceMargin = "0"
		current.UnrealizedPnL = "0"
		current.LiquidationPrice = "0"
		current.BankruptcyPrice = "0"
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
	current.Leverage = effectivePositionLeverage(remainingQty, entryPrice, multiplier, decimalx.MustFromString(current.InitialMargin)).String()
	current.MaintenanceMargin = requiredMaintenanceMargin(market, notional, runtimeCfg.MaintenanceMarginUpliftRatio).String()
	current.UnrealizedPnL = unrealized.String()
	tier := selectOrderRiskTier(market, notional)
	maintenanceRate := effectiveMaintenanceRate(tier.MaintenanceRate, runtimeCfg.MaintenanceMarginUpliftRatio)
	liquidationPenaltyRate := effectiveLiquidationPenaltyRate(runtimeCfg, tier.LiquidationFeeRate)
	current.LiquidationPrice, current.BankruptcyPrice = positionrisk.ComputeDisplayPrices(
		current.Side,
		current.Qty,
		current.AvgEntryPrice,
		current.InitialMargin,
		maintenanceRate,
		liquidationPenaltyRate,
		market.ContractMultiplier,
		runtimeCfg.LiquidationExtraSlippageBps,
	)
	current.Status = PositionStatusOpen
	current.UpdatedAt = now
	return current
}

func (s *Service) loadPosition(ctx context.Context, userID uint64, symbolID uint64, side string, marginMode string) (Position, error) {
	position, err := s.repo.GetPositionForUpdate(ctx, userID, symbolID, side, marginMode)
	if err != nil {
		if isNotFound(err) {
			return Position{}, nil
		}
		return Position{}, err
	}
	return position, nil
}

func normalizeMarginMode(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case MarginModeIsolated:
		return MarginModeIsolated
	default:
		return MarginModeCross
	}
}

func (s *Service) handlePostTradeRisk(ctx context.Context, userID uint64, traceID string, orderID string) error {
	if s.risk == nil || userID == 0 {
		return nil
	}
	if strings.TrimSpace(traceID) == "" {
		traceID = "trade_fill:" + orderID
	}
	return s.risk.RecalculateAfterTrade(ctx, userID, traceID)
}

func closeMarginRelease(current Position, market TradableSymbol, closeQty decimalx.Decimal) decimalx.Decimal {
	currentQty := decimalx.MustFromString(defaultDecimalString(current.Qty))
	currentInitialMargin := decimalx.MustFromString(defaultDecimalString(current.InitialMargin))
	if !currentQty.GreaterThan(decimalx.MustFromString("0")) {
		return decimalx.MustFromString("0")
	}
	if closeQty.GreaterThanOrEqual(currentQty) {
		return currentInitialMargin
	}
	return currentInitialMargin.Mul(closeQty).Div(currentQty)
}

func requiredInitialMargin(market TradableSymbol, notional decimalx.Decimal) decimalx.Decimal {
	tier := selectOrderRiskTier(market, notional)
	return notional.Mul(decimalx.MustFromString(tier.InitialMarginRate))
}

func requiredInitialMarginForLeverage(notional decimalx.Decimal, leverage decimalx.Decimal) decimalx.Decimal {
	if !leverage.GreaterThan(decimalx.MustFromString("0")) {
		return decimalx.MustFromString("0")
	}
	return notional.Div(leverage)
}

func requiredMaintenanceMargin(market TradableSymbol, notional decimalx.Decimal, upliftRatio string) decimalx.Decimal {
	tier := selectOrderRiskTier(market, notional)
	return notional.Mul(decimalx.MustFromString(effectiveMaintenanceRate(tier.MaintenanceRate, upliftRatio)))
}

func resolveOrderLeverage(market TradableSymbol, notional decimalx.Decimal, requested *string, runtimeMaxLeverage string) (decimalx.Decimal, error) {
	tier := selectOrderRiskTier(market, notional)
	maxLeverage := decimalx.MustFromString(defaultDecimalString(tier.MaxLeverage))
	if !maxLeverage.GreaterThan(decimalx.MustFromString("0")) {
		imr := decimalx.MustFromString(defaultDecimalString(tier.InitialMarginRate))
		if !imr.GreaterThan(decimalx.MustFromString("0")) {
			return decimalx.Decimal{}, fmt.Errorf("%w: invalid leverage config", errorsx.ErrConflict)
		}
		maxLeverage = decimalx.MustFromString("1").Div(imr)
	}
	if strings.TrimSpace(runtimeMaxLeverage) != "" {
		override := decimalx.MustFromString(runtimeMaxLeverage)
		if override.GreaterThan(decimalx.MustFromString("0")) {
			maxLeverage = override
		}
	}
	if requested == nil || strings.TrimSpace(*requested) == "" {
		return maxLeverage, nil
	}
	leverage, err := decimalx.NewFromString(strings.TrimSpace(*requested))
	if err != nil {
		return decimalx.Decimal{}, fmt.Errorf("%w: invalid leverage", errorsx.ErrInvalidArgument)
	}
	if !leverage.GreaterThan(decimalx.MustFromString("0")) {
		return decimalx.Decimal{}, fmt.Errorf("%w: leverage must be positive", errorsx.ErrInvalidArgument)
	}
	if leverage.GreaterThan(maxLeverage) {
		return decimalx.Decimal{}, fmt.Errorf("%w: leverage exceeds max %s", errorsx.ErrInvalidArgument, maxLeverage.String())
	}
	return leverage, nil
}

func effectivePositionLeverage(qty decimalx.Decimal, entryPrice decimalx.Decimal, contractMultiplier decimalx.Decimal, initialMargin decimalx.Decimal) decimalx.Decimal {
	if !initialMargin.GreaterThan(decimalx.MustFromString("0")) {
		return decimalx.MustFromString("0")
	}
	return qty.Abs().Mul(entryPrice).Mul(contractMultiplier).Div(initialMargin)
}

func selectOrderRiskTier(market TradableSymbol, notional decimalx.Decimal) RiskTier {
	if len(market.RiskTiers) == 0 {
		maxLeverage := "0"
		if decimalx.MustFromString(defaultDecimalString(market.InitialMarginRate)).GreaterThan(decimalx.MustFromString("0")) {
			maxLeverage = decimalx.MustFromString("1").Div(decimalx.MustFromString(defaultDecimalString(market.InitialMarginRate))).String()
		}
		return RiskTier{
			TierLevel:         1,
			MaxNotional:       "0",
			MaxLeverage:       maxLeverage,
			InitialMarginRate: market.InitialMarginRate,
			MaintenanceRate:   market.MaintenanceMarginRate,
		}
	}
	target := notional.Abs()
	for _, tier := range market.RiskTiers {
		maxNotional := decimalx.MustFromString(tier.MaxNotional)
		if !maxNotional.GreaterThan(decimalx.MustFromString("0")) || target.LessThanOrEqual(maxNotional) {
			return tier
		}
	}
	return market.RiskTiers[len(market.RiskTiers)-1]
}

func defaultDecimalString(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return value
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

func stringPtr(value string) *string {
	return &value
}

func normalizeOrderType(orderType string) string {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case OrderTypeMarket:
		return OrderTypeMarket
	case OrderTypeLimit:
		return OrderTypeLimit
	case OrderTypeStopMarket:
		return OrderTypeStopMarket
	case OrderTypeTakeProfitMarket:
		return OrderTypeTakeProfitMarket
	default:
		return ""
	}
}

func isTriggerOrderType(orderType string) bool {
	return orderType == OrderTypeStopMarket || orderType == OrderTypeTakeProfitMarket
}

func triggerSatisfied(orderType string, side string, triggerPrice decimalx.Decimal, markPrice decimalx.Decimal) bool {
	switch orderType {
	case OrderTypeStopMarket:
		if side == "BUY" {
			return markPrice.GreaterThanOrEqual(triggerPrice)
		}
		return markPrice.LessThanOrEqual(triggerPrice)
	case OrderTypeTakeProfitMarket:
		if side == "BUY" {
			return markPrice.LessThanOrEqual(triggerPrice)
		}
		return markPrice.GreaterThanOrEqual(triggerPrice)
	default:
		return false
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

func (s *Service) effectiveTradableStatus(market TradableSymbol, runtimeCfg RuntimeConfig, now time.Time) string {
	status := market.Status
	if runtimeCfg.GlobalReduceOnly {
		status = degradeTradableStatus(status)
	}
	if market.SnapshotTS.IsZero() || now.UTC().Sub(market.SnapshotTS) > runtimeCfg.MaxMarketDataAge {
		status = degradeTradableStatus(status)
	}
	sessionOpen, err := sessionAllowsOpen(market, runtimeCfg, now)
	if err != nil || !sessionOpen {
		status = degradeTradableStatus(status)
	}
	return status
}

func degradeTradableStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "", "TRADING", "REDUCE_ONLY":
		return "REDUCE_ONLY"
	default:
		return status
	}
}

func sessionAllowsOpen(market TradableSymbol, runtimeCfg RuntimeConfig, now time.Time) (bool, error) {
	policy := strings.TrimSpace(runtimeCfg.SessionPolicy)
	if policy == "" {
		policy = market.SessionPolicy
	}
	return marketsession.AllowsOpenAt(policy, now)
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

func executableLimitPrice(side string, limitPrice decimalx.Decimal, bestBid decimalx.Decimal, bestAsk decimalx.Decimal) (decimalx.Decimal, bool) {
	if side == "BUY" {
		if bestAsk.GreaterThan(decimalx.MustFromString("0")) && bestAsk.LessThanOrEqual(limitPrice) {
			return bestAsk, true
		}
		return decimalx.Decimal{}, false
	}
	if bestBid.GreaterThan(decimalx.MustFromString("0")) && bestBid.GreaterThanOrEqual(limitPrice) {
		return bestBid, true
	}
	return decimalx.Decimal{}, false
}

func (s *Service) resolveOpenLimitFillPrice(market TradableSymbol, exposure SymbolExposure, side string, qty string, limitPrice decimalx.Decimal, runtimeCfg RuntimeConfig) (decimalx.Decimal, bool, error) {
	basePrice, executable := executableLimitPrice(side, limitPrice, decimalx.MustFromString(market.BestBid), decimalx.MustFromString(market.BestAsk))
	if !executable {
		return decimalx.Decimal{}, false, nil
	}
	currentSigned := exposurex.SignedNetNotional(exposure.LongQty, exposure.ShortQty, market.MarkPrice, market.ContractMultiplier)
	adjustmentBps := exposurex.DirectionAdjustmentBps(currentSigned, side, runtimeCfg.NetExposureHardLimit, runtimeCfg.MaxExposureSlippageBps)
	adjusted := adjustExecutionPrice(side, basePrice, adjustmentBps)
	if side == "BUY" && adjusted.GreaterThan(limitPrice) {
		adjusted = limitPrice
	}
	if side == "SELL" && adjusted.LessThan(limitPrice) {
		adjusted = limitPrice
	}
	deltaSigned := exposurex.SignedDeltaNotional(side, qty, adjusted.String(), market.ContractMultiplier)
	if exposurex.ExceedsHardLimit(currentSigned, deltaSigned, runtimeCfg.NetExposureHardLimit) {
		return decimalx.Decimal{}, false, nil
	}
	return adjusted, true, nil
}

func orderFrozenHoldComponents(order Order, feeRate decimalx.Decimal) (decimalx.Decimal, decimalx.Decimal) {
	frozenInitialMargin := decimalx.MustFromString(defaultDecimal(order.FrozenInitialMargin))
	frozenFee := decimalx.MustFromString(defaultDecimal(order.FrozenFee))
	if frozenInitialMargin.GreaterThan(decimalx.MustFromString("0")) || frozenFee.GreaterThan(decimalx.MustFromString("0")) {
		return frozenInitialMargin, frozenFee
	}
	legacyTotal := decimalx.MustFromString(defaultDecimal(order.FrozenMargin))
	if !legacyTotal.GreaterThan(decimalx.MustFromString("0")) {
		return decimalx.MustFromString("0"), decimalx.MustFromString("0")
	}
	if !feeRate.GreaterThan(decimalx.MustFromString("0")) {
		return legacyTotal, decimalx.MustFromString("0")
	}
	return legacyTotal, decimalx.MustFromString("0")
}

func (s *Service) postOpenOrderHold(txCtx context.Context, input CreateOrderInput, order Order, holdAmount decimalx.Decimal, now time.Time, walletAccountID uint64, orderMarginAccountID uint64) error {
	return s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
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
	})
}

func (s *Service) rejectOpenOrder(txCtx context.Context, order Order, tradeAccounts TradeAccounts, reason string, now time.Time) (Order, error) {
	releaseAmount := decimalx.MustFromString(defaultDecimal(order.FrozenMargin))
	if releaseAmount.GreaterThan(decimalx.MustFromString("0")) {
		if err := s.ledger.Post(txCtx, ledgerdomain.PostingRequest{
			LedgerTx: ledgerdomain.LedgerTx{
				ID:             s.idgen.NewID("ldg"),
				EventID:        s.idgen.NewID("evt"),
				BizType:        "trade.order.reject",
				BizRefID:       order.OrderID,
				Asset:          s.cfg.Asset,
				IdempotencyKey: order.OrderID + ":reject-release",
				OperatorType:   "SYSTEM",
				OperatorID:     "order-engine",
				Status:         "COMMITTED",
				CreatedAt:      now,
			},
			Entries: []ledgerdomain.LedgerEntry{
				{AccountID: tradeAccounts.UserOrderMarginAccountID, UserID: uint64Ptr(order.UserID), Asset: s.cfg.Asset, Amount: releaseAmount.Neg().String(), EntryType: "TRADE_ORDER_RELEASE"},
				{AccountID: tradeAccounts.UserWalletAccountID, UserID: uint64Ptr(order.UserID), Asset: s.cfg.Asset, Amount: releaseAmount.String(), EntryType: "TRADE_ORDER_RELEASE"},
			},
		}); err != nil {
			return Order{}, err
		}
	}
	order.Status = OrderStatusRejected
	order.RejectReason = stringPtr(reason)
	order.FrozenInitialMargin = "0"
	order.FrozenFee = "0"
	order.FrozenMargin = "0"
	order.UpdatedAt = now
	if err := s.repo.UpdateOrder(txCtx, order); err != nil {
		return Order{}, err
	}
	if err := s.publishOrderRejected(txCtx, order, now); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Service) wouldOpenPositionLiquidateImmediately(current Position, userID uint64, market TradableSymbol, side string, marginMode string, fillQty decimalx.Decimal, fillPrice decimalx.Decimal, fillMargin decimalx.Decimal, runtimeCfg RuntimeConfig, now time.Time) bool {
	if marginMode != MarginModeIsolated {
		return false
	}
	position := applyOpenFill(current, userID, market, orderSideToPositionSide(side), marginMode, fillQty, fillPrice, fillMargin, runtimeCfg, now, previewIDGenerator{})
	return positionrisk.IsAtOrBeyondLiquidation(position.Side, position.MarkPrice, position.LiquidationPrice)
}

type previewIDGenerator struct{}

func (previewIDGenerator) NewID(prefix string) string {
	return prefix + "_preview"
}

func defaultDecimal(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return value
}

func ensurePositionCanAcceptOpenRisk(position Position) error {
	if position.PositionID != "" && position.Status == PositionStatusLiquidating {
		return fmt.Errorf("%w: position is liquidating", errorsx.ErrForbidden)
	}
	return nil
}

func ensurePositionCanBeUserManaged(position Position) error {
	if position.PositionID != "" && position.Status == PositionStatusLiquidating {
		return fmt.Errorf("%w: position is in liquidation review", errorsx.ErrForbidden)
	}
	return nil
}

func adjustExecutionPrice(side string, price decimalx.Decimal, adjustmentBps int) decimalx.Decimal {
	if adjustmentBps == 0 {
		return price
	}
	adjustment := decimalx.MustFromString(fmt.Sprintf("%d", adjustmentBps)).Div(decimalx.MustFromString("10000"))
	if side == "BUY" {
		return price.Mul(decimalx.MustFromString("1").Add(adjustment))
	}
	return price.Mul(decimalx.MustFromString("1").Sub(adjustment))
}

func (s *Service) assertCanOpenRisk(ctx context.Context, userID uint64) error {
	riskLevel, err := s.repo.GetLatestRiskLevelForUpdate(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if riskLevel == "NO_NEW_RISK" || riskLevel == "LIQUIDATING" {
		return fmt.Errorf("%w: account is %s", errorsx.ErrForbidden, strings.ToLower(riskLevel))
	}
	return nil
}

func (s *Service) ensureSufficientAvailableBalance(available string, required decimalx.Decimal) error {
	if decimalx.MustFromString(available).LessThan(required) {
		return fmt.Errorf("%w: insufficient available balance", errorsx.ErrConflict)
	}
	return nil
}

func (s *Service) lockOpenTradeBalances(ctx context.Context, accounts TradeAccounts) (string, error) {
	accountIDs := []uint64{
		accounts.UserWalletAccountID,
		accounts.UserOrderMarginAccountID,
		accounts.UserPositionMarginAccountID,
	}
	sort.Slice(accountIDs, func(i, j int) bool { return accountIDs[i] < accountIDs[j] })
	balances, err := s.balances.GetAccountBalancesForUpdate(ctx, accountIDs, s.cfg.Asset)
	if err != nil {
		return "", err
	}
	return balances[accounts.UserWalletAccountID], nil
}

func maxDecimal(first decimalx.Decimal, rest ...decimalx.Decimal) decimalx.Decimal {
	maxValue := first
	for _, value := range rest {
		if value.GreaterThan(maxValue) {
			maxValue = value
		}
	}
	return maxValue
}

func proportionalAmount(total decimalx.Decimal, part decimalx.Decimal, whole decimalx.Decimal) decimalx.Decimal {
	if whole.IsZero() {
		return decimalx.MustFromString("0")
	}
	return total.Mul(part).Div(whole)
}

func (s *Service) currentRuntimeConfig(symbol string) RuntimeConfig {
	current := RuntimeConfig{
		MaxMarketDataAge:             s.cfg.MaxMarketDataAge,
		NetExposureHardLimit:         s.cfg.NetExposureHardLimit,
		MaxExposureSlippageBps:       s.cfg.MaxExposureSlippageBps,
		TakerFeeRate:                 s.cfg.TakerFeeRate,
		MakerFeeRate:                 s.cfg.MakerFeeRate,
		DefaultMaxSlippageBps:        s.cfg.DefaultMaxSlippageBps,
		LiquidationExtraSlippageBps:  0,
		MaintenanceMarginUpliftRatio: "",
	}
	if s.runtime == nil {
		return current
	}
	override := s.runtime.CurrentOrderRuntimeConfig(symbol)
	if override.MaxMarketDataAge > 0 {
		current.MaxMarketDataAge = override.MaxMarketDataAge
	}
	if strings.TrimSpace(override.NetExposureHardLimit) != "" {
		current.NetExposureHardLimit = override.NetExposureHardLimit
	}
	if override.MaxExposureSlippageBps >= 0 {
		current.MaxExposureSlippageBps = override.MaxExposureSlippageBps
	}
	if strings.TrimSpace(override.TakerFeeRate) != "" {
		current.TakerFeeRate = override.TakerFeeRate
	}
	if strings.TrimSpace(override.MakerFeeRate) != "" {
		current.MakerFeeRate = override.MakerFeeRate
	}
	if override.DefaultMaxSlippageBps > 0 {
		current.DefaultMaxSlippageBps = override.DefaultMaxSlippageBps
	}
	if strings.TrimSpace(override.MaxLeverage) != "" {
		current.MaxLeverage = override.MaxLeverage
	}
	if strings.TrimSpace(override.LiquidationPenaltyRate) != "" {
		current.LiquidationPenaltyRate = override.LiquidationPenaltyRate
	}
	if override.LiquidationExtraSlippageBps >= 0 {
		current.LiquidationExtraSlippageBps = override.LiquidationExtraSlippageBps
	}
	if strings.TrimSpace(override.MaintenanceMarginUpliftRatio) != "" {
		current.MaintenanceMarginUpliftRatio = override.MaintenanceMarginUpliftRatio
	}
	current.GlobalReadOnly = override.GlobalReadOnly
	current.GlobalReduceOnly = override.GlobalReduceOnly
	return current
}

func effectiveMaintenanceRate(baseRate string, upliftRatio string) string {
	rate := decimalx.MustFromString(defaultDecimalString(baseRate))
	uplift := decimalx.MustFromString(defaultDecimalString(upliftRatio))
	if !uplift.GreaterThan(decimalx.MustFromString("0")) {
		return rate.String()
	}
	return rate.Mul(decimalx.MustFromString("1").Add(uplift)).String()
}

func effectiveLiquidationPenaltyRate(runtimeCfg RuntimeConfig, fallback string) string {
	if strings.TrimSpace(runtimeCfg.LiquidationPenaltyRate) != "" {
		return runtimeCfg.LiquidationPenaltyRate
	}
	return fallback
}

func isNotFound(err error) bool {
	return errors.Is(err, errorsx.ErrNotFound)
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
