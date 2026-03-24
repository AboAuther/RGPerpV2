import {
  Alert,
  Button,
  Card,
  Col,
  Descriptions,
  Input,
  InputNumber,
  Row,
  Segmented,
  Select,
  Slider,
  Space,
  Spin,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd';
import { useEffect, useMemo, useRef, useState } from 'react';
import KlineChart from '../../components/trading/KlineChart';
import { api } from '../../shared/api';
import { EmptyStateCard, ErrorAlert, LoginRequiredCard, PageIntro, StatusTag } from '../../shared/components';
import { useAuth } from '../../shared/auth';
import type { AccountSummary, FillItem, FundingQuoteItem, OrderCreateRequest, OrderItem, PositionItem, RiskSnapshot, SymbolItem, TickerItem } from '../../shared/domain';
import { formatDateTime, formatDecimal, formatDecimalAdaptive, formatPercent, formatSignedUsd, formatSignedUsdAdaptive, formatUsd } from '../../shared/format';
import { useWindowRefetch } from '../../shared/refetch';

const { Paragraph, Text, Title } = Typography;

type ChartInterval = '1m' | '5m' | '15m' | '1h' | '1d';
type EntryMarginMode = 'isolated' | 'cross';
type EntryTimeInForce = 'GTC';
type EntryOrderType = 'MARKET' | 'LIMIT' | 'STOP_MARKET' | 'TAKE_PROFIT_MARKET';
type ActivityTabKey = 'positions' | 'open-orders' | 'trade-history' | 'order-history';

// The decimal helpers deliberately avoid floating-point drift for order sizing,
// trigger pricing, and step alignment. That keeps the UI consistent with the
// backend's stricter decimal validation rules.
function decimalScale(input: string): number {
  const normalized = input.trim();
  if (!normalized) {
    return 0;
  }
  const [, fraction = ''] = normalized.split('.');
  return fraction.length;
}

function decimalToScaledBigInt(input: string, scale: number): bigint | null {
  const normalized = input.trim();
  if (!/^\d+(\.\d+)?$/.test(normalized)) {
    return null;
  }
  const [integerPart, fractionPart = ''] = normalized.split('.');
  const paddedFraction = `${fractionPart}${'0'.repeat(scale)}`.slice(0, scale);
  return BigInt(`${integerPart}${paddedFraction}`);
}

function scaledBigIntToDecimal(value: bigint, scale: number): string {
  if (scale <= 0) {
    return value.toString();
  }
  const negative = value < 0n;
  const absolute = negative ? -value : value;
  const digits = absolute.toString().padStart(scale + 1, '0');
  const integerPart = digits.slice(0, digits.length - scale) || '0';
  const fractionPart = digits.slice(digits.length - scale).replace(/0+$/, '');
  const prefix = negative ? '-' : '';
  return fractionPart ? `${prefix}${integerPart}.${fractionPart}` : `${prefix}${integerPart}`;
}

function isAlignedToStep(value: string, step: string): boolean {
  const scale = Math.max(decimalScale(value), decimalScale(step));
  const scaledValue = decimalToScaledBigInt(value, scale);
  const scaledStep = decimalToScaledBigInt(step, scale);
  if (scaledValue === null || scaledStep === null || scaledStep <= 0n) {
    return false;
  }
  return scaledValue % scaledStep === 0n;
}

function alignDownToStep(value: string, step: string): string {
  const scale = Math.max(decimalScale(value), decimalScale(step));
  const scaledValue = decimalToScaledBigInt(value, scale);
  const scaledStep = decimalToScaledBigInt(step, scale);
  if (scaledValue === null || scaledStep === null || scaledStep <= 0n) {
    return value;
  }
  const aligned = (scaledValue / scaledStep) * scaledStep;
  return scaledBigIntToDecimal(aligned, scale);
}

function compareDecimalStrings(left: string, right: string): number {
  const scale = Math.max(decimalScale(left), decimalScale(right));
  const scaledLeft = decimalToScaledBigInt(left, scale);
  const scaledRight = decimalToScaledBigInt(right, scale);
  if (scaledLeft === null || scaledRight === null) {
    return 0;
  }
  if (scaledLeft < scaledRight) {
    return -1;
  }
  if (scaledLeft > scaledRight) {
    return 1;
  }
  return 0;
}

function deriveDefaultReduceQty(positionQty: string, step: string): string {
  const scale = Math.max(decimalScale(positionQty), decimalScale(step));
  const scaledQty = decimalToScaledBigInt(positionQty, scale);
  const scaledStep = decimalToScaledBigInt(step, scale);
  if (scaledQty === null || scaledStep === null || scaledQty <= 0n || scaledStep <= 0n) {
    return positionQty;
  }
  const halfAligned = ((scaledQty / 2n) / scaledStep) * scaledStep;
  if (halfAligned > 0n) {
    return scaledBigIntToDecimal(halfAligned, scale);
  }
  const minimum = scaledQty < scaledStep ? scaledQty : scaledStep;
  return scaledBigIntToDecimal(minimum, scale);
}

function adjustReduceQty(currentQty: string, step: string, positionQty: string, direction: 'up' | 'down'): string {
  const scale = Math.max(decimalScale(currentQty), decimalScale(step), decimalScale(positionQty));
  const scaledCurrent = decimalToScaledBigInt(currentQty, scale);
  const scaledStep = decimalToScaledBigInt(step, scale);
  const scaledPosition = decimalToScaledBigInt(positionQty, scale);
  if (scaledCurrent === null || scaledStep === null || scaledPosition === null || scaledStep <= 0n || scaledPosition <= 0n) {
    return currentQty;
  }
  const minimum = scaledPosition < scaledStep ? scaledPosition : scaledStep;
  let next = direction === 'up' ? scaledCurrent + scaledStep : scaledCurrent - scaledStep;
  if (next < minimum) {
    next = minimum;
  }
  if (next > scaledPosition) {
    next = scaledPosition;
  }
  return scaledBigIntToDecimal(next, scale);
}

function toNumber(input: string | number | null | undefined): number {
  if (input == null) {
    return 0;
  }
  const value = typeof input === 'number' ? input : Number(input);
  return Number.isFinite(value) ? value : 0;
}

function formatOrderTypeLabel(value: string): string {
  switch (value) {
    case 'STOP_MARKET':
      return 'Stop Market';
    case 'TAKE_PROFIT_MARKET':
      return 'TP Market';
    case 'LIMIT':
      return 'Limit';
    case 'MARKET':
      return 'Market';
    default:
      return value || '--';
  }
}

function formatOrderDirection(order: Pick<OrderItem, 'side' | 'position_effect'>): string {
  const side = order.side === 'BUY' ? 'Buy' : order.side === 'SELL' ? 'Sell' : order.side;
  const effect = order.position_effect ? ` / ${order.position_effect}` : '';
  return `${side}${effect}`;
}

function getTriggerCondition(order: Pick<OrderItem, 'type' | 'side' | 'trigger_price'>): string {
  if (!order.trigger_price) {
    return '--';
  }
  if (order.type === 'STOP_MARKET') {
    return order.side === 'BUY' ? `Mark >= ${formatUsd(order.trigger_price)}` : `Mark <= ${formatUsd(order.trigger_price)}`;
  }
  if (order.type === 'TAKE_PROFIT_MARKET') {
    return order.side === 'BUY' ? `Mark <= ${formatUsd(order.trigger_price)}` : `Mark >= ${formatUsd(order.trigger_price)}`;
  }
  return formatUsd(order.trigger_price);
}

function getOrderDisplayPrice(order: Pick<OrderItem, 'price' | 'trigger_price' | 'avg_fill_price'>): string {
  if (order.price) {
    return formatUsd(order.price);
  }
  if (order.trigger_price) {
    return formatUsd(order.trigger_price);
  }
  if (toNumber(order.avg_fill_price) > 0) {
    return formatUsd(order.avg_fill_price);
  }
  return '--';
}

interface TradeState {
  symbols: SymbolItem[];
  tickers: TickerItem[];
  fundingQuotes: FundingQuoteItem[];
  orders: OrderItem[];
  fills: FillItem[];
  positions: PositionItem[];
  summary: AccountSummary | null;
  risk: RiskSnapshot | null;
}

const intervalOptions: ChartInterval[] = ['1m', '5m', '15m', '1h', '1d'];
const MARKET_TICKER_POLL_MS = 1000;
const PRIVATE_DATA_POLL_MS = 5000;

function getPriceTone(current: number, previous: number): 'up' | 'down' | 'flat' {
  if (!Number.isFinite(current) || !Number.isFinite(previous) || previous <= 0) {
    return 'flat';
  }
  if (current > previous) {
    return 'up';
  }
  if (current < previous) {
    return 'down';
  }
  return 'flat';
}

function formatCountdown(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return '00:00:00';
  }
  const total = Math.floor(seconds);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secs = total % 60;
  return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
}

export function TradePage() {
  const { session } = useAuth();
  const [msgApi, contextHolder] = message.useMessage();
  const [state, setState] = useState<TradeState | null>(null);
  const [previousTickers, setPreviousTickers] = useState<Record<string, TickerItem>>({});
  const [selectedSymbol, setSelectedSymbol] = useState('BTC-USDC');
  const [interval, setInterval] = useState<ChartInterval>('15m');
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [clockNowMs, setClockNowMs] = useState(() => Date.now());
  const [error, setError] = useState<unknown>(null);
  const [submitting, setSubmitting] = useState(false);
  const [activityTab, setActivityTab] = useState<ActivityTabKey>('positions');
  const [entrySide, setEntrySide] = useState<'BUY' | 'SELL'>('BUY');
  const [entryType, setEntryType] = useState<EntryOrderType>('MARKET');
  const [entryEffect, setEntryEffect] = useState<'OPEN' | 'REDUCE' | 'CLOSE'>('OPEN');
  const [entryQty, setEntryQty] = useState('0.001');
  const [entryPrice, setEntryPrice] = useState('');
  const [entryTriggerPrice, setEntryTriggerPrice] = useState('');
  const [entryReduceOnly, setEntryReduceOnly] = useState(false);
  const [entryMarginMode, setEntryMarginMode] = useState<EntryMarginMode>('isolated');
  const [entryLeverage, setEntryLeverage] = useState(10);
  const [entryTimeInForce, setEntryTimeInForce] = useState<EntryTimeInForce>('GTC');
  const [entryMaxSlippageBps, setEntryMaxSlippageBps] = useState(100);
  const [reduceQtyByPosition, setReduceQtyByPosition] = useState<Record<string, string>>({});

  // Public market data is polled more aggressively than private state so the
  // page stays responsive without pushing unnecessary authenticated traffic.
  const orderEntryCardRef = useRef<HTMLDivElement | null>(null);
  const stateRef = useRef<TradeState | null>(null);
  const loadingRef = useRef(false);
  const marketPollingRef = useRef(false);
  const privatePollingRef = useRef(false);

  useEffect(() => {
    stateRef.current = state;
  }, [state]);

  function snapshotPreviousTickers() {
    const currentTickers = stateRef.current?.tickers || [];
    setPreviousTickers(Object.fromEntries(currentTickers.map((item) => [item.symbol, item])) as Record<string, TickerItem>);
  }

  async function loadMarketTickers() {
    if (marketPollingRef.current) {
      return;
    }
    marketPollingRef.current = true;
    try {
      const [symbols, tickers, fundingQuotes] = await Promise.all([api.market.getSymbols(), api.market.getTickers(), api.market.getFundingQuotes()]);
      snapshotPreviousTickers();
      setState((current) => (current ? { ...current, symbols, tickers, fundingQuotes } : current));
    } catch (loadError) {
      setError(loadError);
    } finally {
      marketPollingRef.current = false;
    }
  }

  async function loadPrivateData() {
    if (!session || privatePollingRef.current) {
      return;
    }
    privatePollingRef.current = true;
    try {
      const [orders, fills, positions, summary, risk] = await Promise.all([
        api.orders.getOrders(),
        api.fills.getFills(),
        api.positions.getPositions(),
        api.account.getSummary(),
        api.account.getRisk(),
      ]);
      setState((current) =>
        current
          ? {
              ...current,
              orders,
              fills,
              positions,
              summary,
              risk,
            }
          : current,
      );
    } catch (loadError) {
      setError(loadError);
    } finally {
      privatePollingRef.current = false;
    }
  }

  async function loadData(background = false) {
    if (loadingRef.current) {
      return;
    }
    loadingRef.current = true;
    if (background && state) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);

    try {
      const [symbols, tickers, fundingQuotes, orders, fills, positions, summary, risk] = await Promise.all([
        api.market.getSymbols(),
        api.market.getTickers(),
        api.market.getFundingQuotes(),
        session ? api.orders.getOrders() : Promise.resolve([]),
        session ? api.fills.getFills() : Promise.resolve([]),
        session ? api.positions.getPositions() : Promise.resolve([]),
        session ? api.account.getSummary() : Promise.resolve(null),
        session ? api.account.getRisk() : Promise.resolve(null),
      ]);

      snapshotPreviousTickers();

      setState({
        symbols,
        tickers,
        fundingQuotes,
        orders,
        fills,
        positions,
        summary,
        risk,
      });
    } catch (loadError) {
      setError(loadError);
    } finally {
      loadingRef.current = false;
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    void loadData();
  }, [session]);

  useWindowRefetch(() => {
    void loadData(true);
  }, !!state);

  useEffect(() => {
    if (!state) {
      return;
    }
    const timer = window.setInterval(() => {
      if (document.visibilityState !== 'visible') {
        return;
      }
      void loadMarketTickers();
    }, MARKET_TICKER_POLL_MS);
    return () => window.clearInterval(timer);
  }, [state]);

  useEffect(() => {
    const timer = window.setInterval(() => setClockNowMs(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    if (!state || !session) {
      return;
    }
    const timer = window.setInterval(() => {
      if (document.visibilityState !== 'visible') {
        return;
      }
      void loadPrivateData();
    }, PRIVATE_DATA_POLL_MS);
    return () => window.clearInterval(timer);
  }, [session, state]);

  useEffect(() => {
    if (!state?.symbols.length) {
      return;
    }
    if (!state.symbols.some((item) => item.symbol === selectedSymbol)) {
      setSelectedSymbol(state.symbols[0].symbol);
    }
  }, [selectedSymbol, state?.symbols]);

  const selectedMarket = useMemo(() => {
    if (!state) {
      return null;
    }
    const symbol = state.symbols.find((item) => item.symbol === selectedSymbol) || state.symbols[0];
    if (!symbol) {
      return null;
    }
    return {
      ...symbol,
      ticker: state.tickers.find((item) => item.symbol === symbol.symbol) || null,
      fundingQuote: state.fundingQuotes.find((item) => item.symbol === symbol.symbol) || null,
    };
  }, [selectedSymbol, state]);

  const accountOpenOrders = useMemo(
    () => state?.orders.filter((item) => item.status === 'RESTING' || item.status === 'TRIGGER_WAIT') || [],
    [state?.orders],
  );
  const accountOrderHistory = useMemo(() => state?.orders || [], [state?.orders]);
  const accountTradeHistory = useMemo(() => state?.fills || [], [state?.fills]);
  const accountActivePositions = useMemo(
    () => state?.positions.filter((item) => item.status === 'OPEN') || [],
    [state?.positions],
  );
  const latestCancelableOrder = useMemo(
    () => accountOpenOrders[0] ?? null,
    [accountOpenOrders],
  );
  const accountLiquidating = state?.risk?.risk_state === 'LIQUIDATING';
  const noTicker = !selectedMarket?.ticker;

  useEffect(() => {
    if (!selectedMarket) {
      return;
    }
    const nextDefault = Number(selectedMarket.default_max_slippage_bps || 100);
    setEntryMaxSlippageBps(Number.isFinite(nextDefault) && nextDefault > 0 ? nextDefault : 100);
  }, [selectedSymbol, selectedMarket?.default_max_slippage_bps]);
  const tickerStale = selectedMarket?.ticker?.stale ?? false;
  const marketPaused = selectedMarket?.status === 'PAUSED';
  const marketReduceOnly = selectedMarket?.status === 'REDUCE_ONLY';
  const canOpenRisk = state?.risk ? state.risk.can_open_risk : true;

  const currentTicker = selectedMarket?.ticker ?? null;
  const currentFundingQuote = selectedMarket?.fundingQuote ?? null;
  const previousTicker = selectedMarket ? previousTickers[selectedMarket.symbol] ?? null : null;
  const markPriceValue = Number(currentTicker?.mark_price ?? 0);
  const previousMarkPriceValue = Number(previousTicker?.mark_price ?? 0);
  const indexPriceValue = Number(currentTicker?.index_price ?? 0);
  const previousIndexPriceValue = Number(previousTicker?.index_price ?? 0);
  const bestBidValue = Number(currentTicker?.best_bid ?? 0);
  const previousBestBidValue = Number(previousTicker?.best_bid ?? 0);
  const bestAskValue = Number(currentTicker?.best_ask ?? 0);
  const previousBestAskValue = Number(previousTicker?.best_ask ?? 0);
  const priceDelta = Number.isFinite(markPriceValue) && Number.isFinite(previousMarkPriceValue) ? markPriceValue - previousMarkPriceValue : 0;
  const priceDeltaPercent =
    Number.isFinite(markPriceValue) && Number.isFinite(previousMarkPriceValue) && previousMarkPriceValue > 0 ? priceDelta / previousMarkPriceValue : 0;
  const priceTone = getPriceTone(markPriceValue, previousMarkPriceValue);
  const indexTone = getPriceTone(indexPriceValue, previousIndexPriceValue);
  const bestBidTone = getPriceTone(bestBidValue, previousBestBidValue);
  const bestAskTone = getPriceTone(bestAskValue, previousBestAskValue);
  const fundingNotApplicable = currentFundingQuote?.status === 'NOT_APPLICABLE';
  const fundingRateLabel = fundingNotApplicable ? 'N/A' : (currentFundingQuote?.estimated_rate ? formatPercent(currentFundingQuote.estimated_rate, 4) : '--');
  const countdownSeconds = currentFundingQuote && !fundingNotApplicable
    ? Math.max(0, Math.floor((new Date(currentFundingQuote.next_funding_at).getTime() - clockNowMs) / 1000))
    : 0;
  const fundingCountdownLabel = fundingNotApplicable ? 'N/A' : (currentFundingQuote ? formatCountdown(countdownSeconds) : '--');

  const marketOptions = useMemo(
    () =>
      (state?.symbols || []).map((item) => ({
        label: `${item.symbol} · ${item.asset_class}`,
        value: item.symbol,
      })),
    [state?.symbols],
  );
  const marketBySymbol = useMemo(() => new Map((state?.symbols || []).map((item) => [item.symbol, item])), [state?.symbols]);
  const tickerBySymbol = useMemo(() => new Map((state?.tickers || []).map((item) => [item.symbol, item])), [state?.tickers]);
  const orderById = useMemo(() => new Map((state?.orders || []).map((item) => [item.order_id, item])), [state?.orders]);

  useEffect(() => {
    setReduceQtyByPosition((current) => {
      const next: Record<string, string> = {};
      for (const position of accountActivePositions) {
        const market = marketBySymbol.get(position.symbol);
        if (!market) {
          continue;
        }
        const existing = current[position.position_id];
        if (!existing) {
          next[position.position_id] = deriveDefaultReduceQty(position.qty, market.step_size);
          continue;
        }
        let normalized = existing.trim();
        if (!normalized) {
          normalized = deriveDefaultReduceQty(position.qty, market.step_size);
        } else if (compareDecimalStrings(normalized, position.qty) > 0) {
          normalized = position.qty;
        }
        next[position.position_id] = normalized;
      }
      return next;
    });
  }, [accountActivePositions, marketBySymbol]);

  const orderEntryDisabled =
    !session ||
    !selectedMarket ||
    submitting ||
    noTicker ||
    tickerStale ||
    accountLiquidating ||
    marketPaused ||
    (entryEffect === 'OPEN'
      ? selectedMarket.status !== 'TRADING' || !canOpenRisk
      : !['TRADING', 'REDUCE_ONLY'].includes(selectedMarket.status));

  const isLimitOrder = entryType === 'LIMIT';
  const isTriggerOrder = entryType === 'STOP_MARKET' || entryType === 'TAKE_PROFIT_MARKET';
  const referencePrice =
    isLimitOrder && entryPrice
      ? Number(entryPrice)
      : isTriggerOrder && entryTriggerPrice
        ? Number(entryTriggerPrice)
        : markPriceValue;
  const estimatedNotional = Number(entryQty || 0) * (Number.isFinite(referencePrice) ? referencePrice : 0);
  const leverageMax = Math.max(1, Math.floor(Number(selectedMarket?.max_leverage || '1') || 1));
  const estimatedMargin = entryEffect === 'OPEN' && estimatedNotional > 0 && entryLeverage > 0 ? estimatedNotional / entryLeverage : 0;
  const estimatedReserve = (isLimitOrder || isTriggerOrder) && entryEffect === 'OPEN' ? estimatedMargin : 0;
  const marketProtectText = `${entryMaxSlippageBps} bps`;
  const quoteHintPrice = isLimitOrder ? Number(entryPrice || 0) : isTriggerOrder ? Number(entryTriggerPrice || 0) : referencePrice;
  const primaryTriggerLabel = entryType === 'STOP_MARKET' ? '止损' : '止盈';

  useEffect(() => {
    if (entryEffect === 'OPEN') {
      setEntryReduceOnly(false);
      return;
    }
    setEntryReduceOnly(true);
  }, [entryEffect]);

  useEffect(() => {
    if (entryLeverage > leverageMax) {
      setEntryLeverage(leverageMax);
    }
  }, [entryLeverage, leverageMax]);

  async function submitOrder(input: OrderCreateRequest, successText: string) {
    try {
      setSubmitting(true);
      await api.orders.createOrder(input);
      await loadData(true);
      void msgApi.success(successText);
    } catch (submitError) {
      const errorText = submitError instanceof Error ? submitError.message : '请求失败';
      void msgApi.error(errorText);
    } finally {
      setSubmitting(false);
    }
  }

  async function handleOpenOrder() {
    if (!selectedMarket) {
      return;
    }
    const qty = entryQty.trim();
    const price = entryPrice.trim();
    const triggerPrice = entryTriggerPrice.trim();
    const qtyValue = Number(qty);
    const priceValue = Number(price);
    const triggerPriceValue = Number(triggerPrice);
    if (!qty) {
      void msgApi.error('请输入数量');
      return;
    }
    if (!Number.isFinite(qtyValue) || qtyValue <= 0) {
      void msgApi.error('数量必须为正数');
      return;
    }
    if (!isAlignedToStep(qty, selectedMarket.step_size)) {
      void msgApi.error(`数量必须按 step_size 对齐，当前 step 为 ${selectedMarket.step_size}`);
      return;
    }
    if (isLimitOrder && !price) {
      void msgApi.error('请输入限价');
      return;
    }
    if (isLimitOrder && (!Number.isFinite(priceValue) || priceValue <= 0)) {
      void msgApi.error('限价必须为正数');
      return;
    }
    if (isLimitOrder && !isAlignedToStep(price, selectedMarket.tick_size)) {
      void msgApi.error(`限价必须按 tick_size 对齐，当前 tick 为 ${selectedMarket.tick_size}`);
      return;
    }
    if (isTriggerOrder && !triggerPrice) {
      void msgApi.error('请输入触发价');
      return;
    }
    if (isTriggerOrder && (!Number.isFinite(triggerPriceValue) || triggerPriceValue <= 0)) {
      void msgApi.error('触发价必须为正数');
      return;
    }
    if (isTriggerOrder && !isAlignedToStep(triggerPrice, selectedMarket.tick_size)) {
      void msgApi.error(`触发价必须按 tick_size 对齐，当前 tick 为 ${selectedMarket.tick_size}`);
      return;
    }
    if (!Number.isFinite(entryMaxSlippageBps) || entryMaxSlippageBps <= 0) {
      void msgApi.error('市价保护必须大于 0 bps');
      return;
    }
    if (entryEffect === 'OPEN' && (!Number.isFinite(entryLeverage) || entryLeverage <= 0 || entryLeverage > leverageMax)) {
      void msgApi.error(`杠杆必须在 1 到 ${leverageMax}x 之间`);
      return;
    }
    await submitOrder(
      {
        client_order_id: `cli_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`,
        symbol: selectedMarket.symbol,
        side: entrySide,
        position_effect: entryEffect,
        type: entryType,
        qty,
        leverage: entryEffect === 'OPEN' ? String(entryLeverage) : null,
        margin_mode: entryMarginMode === 'isolated' ? 'ISOLATED' : 'CROSS',
        price: isLimitOrder ? price : null,
        trigger_price: isTriggerOrder ? triggerPrice : null,
        reduce_only: entryReduceOnly,
        time_in_force: isLimitOrder ? entryTimeInForce : undefined,
        max_slippage_bps: entryMaxSlippageBps,
      },
      entryType === 'MARKET'
        ? '市价订单已提交'
        : entryType === 'LIMIT'
          ? '限价订单已提交'
          : `${primaryTriggerLabel}触发单已提交`,
    );
  }

  async function handleClosePosition(position: PositionItem) {
    const closeSide: 'BUY' | 'SELL' = position.side === 'LONG' ? 'SELL' : 'BUY';
    await submitOrder(
      {
        client_order_id: `cli_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`,
        symbol: position.symbol,
        side: closeSide,
        position_effect: 'CLOSE',
        type: 'MARKET',
        qty: position.qty,
        margin_mode: position.margin_mode === 'ISOLATED' ? 'ISOLATED' : 'CROSS',
        reduce_only: true,
      },
      `${position.symbol} 市价平仓已提交`,
    );
  }

  async function handleReducePosition(position: PositionItem, qtyInput: string) {
    const market = marketBySymbol.get(position.symbol);
    if (!market) {
      void msgApi.error(`未找到 ${position.symbol} 的交易规则`);
      return;
    }
    const qty = qtyInput.trim();
    const qtyValue = Number(qty);
    if (!qty) {
      void msgApi.error('请输入减仓数量');
      return;
    }
    if (!Number.isFinite(qtyValue) || qtyValue <= 0) {
      void msgApi.error('减仓数量必须为正数');
      return;
    }
    if (!isAlignedToStep(qty, market.step_size)) {
      void msgApi.error(`减仓数量必须按 step_size 对齐，当前 step 为 ${market.step_size}`);
      return;
    }
    if (compareDecimalStrings(qty, position.qty) > 0) {
      void msgApi.error('减仓数量不能超过当前仓位');
      return;
    }
    const reduceSide: 'BUY' | 'SELL' = position.side === 'LONG' ? 'SELL' : 'BUY';
    await submitOrder(
      {
        client_order_id: `cli_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`,
        symbol: position.symbol,
        side: reduceSide,
        position_effect: 'REDUCE',
        type: 'MARKET',
        qty,
        margin_mode: position.margin_mode === 'ISOLATED' ? 'ISOLATED' : 'CROSS',
        reduce_only: true,
      },
      `${position.symbol} 市价减仓已提交`,
    );
  }

  function handleReduceQtyInput(position: PositionItem, nextValue: string) {
    setReduceQtyByPosition((current) => ({
      ...current,
      [position.position_id]: nextValue,
    }));
  }

  function handleAdjustReduceQty(position: PositionItem, direction: 'up' | 'down') {
    const market = marketBySymbol.get(position.symbol);
    if (!market) {
      return;
    }
    const currentQty = reduceQtyByPosition[position.position_id] || deriveDefaultReduceQty(position.qty, market.step_size);
    const nextQty = adjustReduceQty(currentQty, market.step_size, position.qty, direction);
    setReduceQtyByPosition((current) => ({
      ...current,
      [position.position_id]: nextQty,
    }));
  }

  async function handleCancelOrder(orderId: string) {
    try {
      setSubmitting(true);
      await api.orders.cancelOrder(orderId);
      await loadData(true);
      void msgApi.success('撤单成功');
    } catch (cancelError) {
      const errorText = cancelError instanceof Error ? cancelError.message : '撤单失败';
      void msgApi.error(errorText);
    } finally {
      setSubmitting(false);
    }
  }

  const positionsColumns = useMemo(
    () => [
      {
        title: 'Coin',
        render: (_: unknown, record: PositionItem) => (
          <Space size={[8, 8]} wrap>
            <Text strong>{record.symbol}</Text>
            <StatusTag value={record.side} />
            <Tag color={record.margin_mode === 'ISOLATED' ? 'blue' : 'gold'}>
              {record.margin_mode === 'ISOLATED' ? '逐仓' : '全仓'}
            </Tag>
          </Space>
        ),
      },
      { title: 'Size', dataIndex: 'qty', align: 'right' as const, render: (value: string) => formatDecimalAdaptive(value, 8) },
      {
        title: 'Position Value',
        align: 'right' as const,
        render: (_: unknown, record: PositionItem) => formatUsd(toNumber(record.qty) * toNumber(record.mark_price), 8),
      },
      { title: 'Entry Price', dataIndex: 'avg_entry_price', align: 'right' as const, render: (value: string) => formatUsd(value, 8) },
      { title: 'Mark Price', dataIndex: 'mark_price', align: 'right' as const, render: (value: string) => formatUsd(value, 8) },
      {
        title: 'PNL (ROE %)',
        align: 'right' as const,
        render: (_: unknown, record: PositionItem) => {
          const margin = toNumber(record.initial_margin);
          const roe = margin > 0 ? toNumber(record.unrealized_pnl) / margin : 0;
          return (
            <Space direction="vertical" size={0} style={{ width: '100%' }}>
              <Text strong>{formatSignedUsdAdaptive(record.unrealized_pnl)}</Text>
              <Text type="secondary">{`(${formatPercent(roe)})`}</Text>
            </Space>
          );
        },
      },
      {
        title: 'Liq. Price',
        dataIndex: 'liquidation_price',
        align: 'right' as const,
        render: (value: string) => (toNumber(value) > 0 ? formatUsd(value, 8) : '--'),
      },
      { title: 'Margin', dataIndex: 'initial_margin', align: 'right' as const, render: (value: string) => formatUsd(value, 8) },
      { title: 'Funding', dataIndex: 'funding_accrual', align: 'right' as const, render: (value: string) => formatSignedUsdAdaptive(value, 8) },
      {
        title: 'Action',
        render: (_: unknown, record: PositionItem) => {
          const rowMarket = marketBySymbol.get(record.symbol);
          const rowTicker = tickerBySymbol.get(record.symbol);
          const rowDisabled =
            submitting ||
            accountLiquidating ||
            !rowMarket ||
            record.status === 'LIQUIDATING' ||
            rowMarket.status === 'PAUSED' ||
            rowTicker?.stale ||
            !rowTicker ||
            !['TRADING', 'REDUCE_ONLY'].includes(rowMarket.status);
          const reduceQty = reduceQtyByPosition[record.position_id] || (rowMarket ? deriveDefaultReduceQty(record.qty, rowMarket.step_size) : record.qty);
          return (
            <Space size={8} wrap>
              <Button size="small" onClick={() => handleAdjustReduceQty(record, 'down')} disabled={rowDisabled}>
                -
              </Button>
              <Input
                size="small"
                value={reduceQty}
                onChange={(event) => handleReduceQtyInput(record, event.target.value)}
                disabled={rowDisabled}
                style={{ width: 92 }}
              />
              <Button size="small" onClick={() => handleAdjustReduceQty(record, 'up')} disabled={rowDisabled}>
                +
              </Button>
              <Button size="small" onClick={() => void handleReducePosition(record, reduceQty)} disabled={rowDisabled}>
                Reduce
              </Button>
              <Button size="small" onClick={() => void handleClosePosition(record)} loading={submitting} disabled={rowDisabled}>
                Close
              </Button>
            </Space>
          );
        },
      },
    ],
    [accountLiquidating, handleClosePosition, marketBySymbol, reduceQtyByPosition, submitting, tickerBySymbol],
  );

  const openOrdersColumns = useMemo(
    () => [
      { title: 'Time', dataIndex: 'created_at', render: (value: string) => formatDateTime(value), width: 180 },
      { title: 'Type', dataIndex: 'type', render: (value: string) => formatOrderTypeLabel(value) },
      { title: 'Coin', dataIndex: 'symbol', render: (value: string) => <Text strong>{value}</Text> },
      { title: 'Direction', render: (_: unknown, record: OrderItem) => formatOrderDirection(record) },
      {
        title: 'Size',
        align: 'right' as const,
        render: (_: unknown, record: OrderItem) => formatDecimalAdaptive(Math.max(0, toNumber(record.qty) - toNumber(record.filled_qty)), 8),
      },
      { title: 'Original Size', dataIndex: 'qty', align: 'right' as const, render: (value: string) => formatDecimalAdaptive(value, 8) },
      {
        title: 'Order Value',
        align: 'right' as const,
        render: (_: unknown, record: OrderItem) => {
          const ticker = tickerBySymbol.get(record.symbol);
          const referencePrice = toNumber(record.price ?? record.trigger_price ?? ticker?.mark_price);
          return referencePrice > 0 ? formatUsd(referencePrice * toNumber(record.qty), 8) : '--';
        },
      },
      { title: 'Price', render: (_: unknown, record: OrderItem) => getOrderDisplayPrice(record) },
      { title: 'Reduce Only', dataIndex: 'reduce_only', render: (value: boolean) => (value ? <Tag color="gold">Yes</Tag> : <Text type="secondary">No</Text>) },
      { title: 'Trigger Conditions', render: (_: unknown, record: OrderItem) => <Text type="secondary">{getTriggerCondition(record)}</Text> },
      {
        title: 'Action',
        render: (_: unknown, record: OrderItem) => (
          <Button size="small" onClick={() => void handleCancelOrder(record.order_id)} loading={submitting}>
            Cancel
          </Button>
        ),
      },
    ],
    [handleCancelOrder, submitting, tickerBySymbol],
  );

  const tradeHistoryColumns = useMemo(
    () => [
      { title: 'Time', dataIndex: 'created_at', render: (value: string) => formatDateTime(value), width: 180 },
      { title: 'Coin', dataIndex: 'symbol', render: (value: string) => <Text strong>{value}</Text> },
      {
        title: 'Direction',
        render: (_: unknown, record: FillItem) => {
          const order = orderById.get(record.order_id);
          return order ? formatOrderDirection(order) : record.side;
        },
      },
      { title: 'Price', dataIndex: 'price', align: 'right' as const, render: (value: string) => formatUsd(value, 8) },
      { title: 'Size', dataIndex: 'qty', align: 'right' as const, render: (value: string) => formatDecimalAdaptive(value, 8) },
      {
        title: 'Trade Value',
        align: 'right' as const,
        render: (_: unknown, record: FillItem) => formatUsd(toNumber(record.qty) * toNumber(record.price), 8),
      },
      { title: 'Fee', dataIndex: 'fee_amount', align: 'right' as const, render: (value: string) => formatUsd(value, 8) },
    ],
    [orderById],
  );

  const orderHistoryColumns = useMemo(
    () => [
      { title: 'Time', dataIndex: 'created_at', render: (value: string) => formatDateTime(value), width: 180 },
      { title: 'Type', dataIndex: 'type', render: (value: string) => formatOrderTypeLabel(value) },
      { title: 'Coin', dataIndex: 'symbol', render: (value: string) => <Text strong>{value}</Text> },
      { title: 'Direction', render: (_: unknown, record: OrderItem) => formatOrderDirection(record) },
      { title: 'Size', dataIndex: 'qty', align: 'right' as const, render: (value: string) => formatDecimalAdaptive(value, 8) },
      { title: 'Filled Size', dataIndex: 'filled_qty', align: 'right' as const, render: (value: string) => formatDecimalAdaptive(value, 8) },
      {
        title: 'Order Value',
        align: 'right' as const,
        render: (_: unknown, record: OrderItem) => {
          const referencePrice = toNumber(record.price ?? record.trigger_price ?? record.avg_fill_price);
          return referencePrice > 0 ? formatUsd(referencePrice * toNumber(record.qty), 8) : '--';
        },
      },
      { title: 'Price', render: (_: unknown, record: OrderItem) => getOrderDisplayPrice(record) },
      { title: 'Reduce Only', dataIndex: 'reduce_only', render: (value: boolean) => (value ? <Tag color="gold">Yes</Tag> : <Text type="secondary">No</Text>) },
      { title: 'Trigger Conditions', render: (_: unknown, record: OrderItem) => <Text type="secondary">{getTriggerCondition(record)}</Text> },
      { title: 'Status', dataIndex: 'status', render: (value: string) => <StatusTag value={value} /> },
    ],
    [],
  );

  const activityTabItems = [
    {
      key: 'positions',
      label: 'Positions',
      children: (
        <Table
          rowKey="position_id"
          dataSource={accountActivePositions}
          pagination={false}
          locale={{ emptyText: 'No active positions yet' }}
          scroll={{ x: 1380 }}
          columns={positionsColumns}
        />
      ),
    },
    {
      key: 'open-orders',
      label: 'Open Orders',
      children: (
        <Table
          rowKey="order_id"
          dataSource={accountOpenOrders}
          pagination={false}
          locale={{ emptyText: 'No open orders yet' }}
          scroll={{ x: 1600 }}
          columns={openOrdersColumns}
        />
      ),
    },
    {
      key: 'trade-history',
      label: 'Trade History',
      children: (
        <Table
          rowKey="fill_id"
          dataSource={accountTradeHistory}
          pagination={false}
          locale={{ emptyText: 'No trades yet' }}
          scroll={{ x: 1320 }}
          columns={tradeHistoryColumns}
        />
      ),
    },
    {
      key: 'order-history',
      label: 'Order History',
      children: (
        <Table
          rowKey="order_id"
          dataSource={accountOrderHistory}
          pagination={false}
          locale={{ emptyText: 'No historical orders yet' }}
          scroll={{ x: 1800 }}
          columns={orderHistoryColumns}
        />
      ),
    },
  ];

  return (
    <div className="rg-app-page rg-app-page--trade">
      {contextHolder}
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow="Trading"
          title="Trade Console"
          description="查看行情、提交订单、管理持仓，并跟踪账户风险与成交记录。"
          titleEffect="glitch"
          descriptionEffect="proximity"
          extra={
            <Button onClick={() => void loadData(true)} loading={refreshing}>
              刷新交易数据
            </Button>
          }
        />

        {loading ? <Spin size="large" /> : null}
        <ErrorAlert error={error} />

        {!loading && !state?.symbols.length ? (
          <EmptyStateCard title="暂无可交易合约" description="当前暂时没有可交易合约，请稍后再试。" />
        ) : null}

        {selectedMarket ? (
          <>
            <Card className="surface-card trade-quote-bar-card">
              <div className="trade-quote-bar">
                <div className="trade-quote-bar__identity">
                  <Text className="page-intro-eyebrow">Market</Text>
                  <Select value={selectedMarket.symbol} options={marketOptions} onChange={setSelectedSymbol} className="trade-symbol-select" />
                  <Space wrap size={[8, 8]}>
                    <StatusTag value={selectedMarket.status} />
                    <Tag color="blue">{selectedMarket.asset_class}</Tag>
                    <Tag color="cyan">TradingView</Tag>
                  </Space>
                </div>

                <div className="trade-quote-bar__hero">
                  <Text className="trade-quote-bar__hero-label">Mark Price</Text>
                  <div className={`trade-quote-bar__hero-price is-${priceTone}`}>
                    <span className={`trade-quote-bar__hero-dot is-${priceTone}`} />
                    {formatUsd(selectedMarket.ticker?.mark_price)}
                  </div>
                  <div className={`trade-quote-bar__hero-change is-${priceTone}`}>
                    {`${priceDelta > 0 ? '+' : ''}${formatUsd(priceDelta)} / ${priceDelta > 0 ? '+' : ''}${formatPercent(priceDeltaPercent)}`}
                  </div>
                </div>

                <div className="trade-quote-bar__stats">
                  <div className="trade-quote-stat">
                    <Text className="trade-quote-stat__label">Index</Text>
                    <Text className={`trade-quote-stat__value is-${indexTone}`}>{formatUsd(selectedMarket.ticker?.index_price)}</Text>
                  </div>
                  <div className="trade-quote-stat">
                    <Text className="trade-quote-stat__label">Best Bid</Text>
                    <Text className={`trade-quote-stat__value is-${bestBidTone}`}>{formatUsd(selectedMarket.ticker?.best_bid)}</Text>
                  </div>
                  <div className="trade-quote-stat">
                    <Text className="trade-quote-stat__label">Best Ask</Text>
                    <Text className={`trade-quote-stat__value is-${bestAskTone}`}>{formatUsd(selectedMarket.ticker?.best_ask)}</Text>
                  </div>
                  <div className="trade-quote-stat">
                    <Text className="trade-quote-stat__label">Updated</Text>
                    <Text className="trade-quote-stat__value">{formatDateTime(selectedMarket.ticker?.ts)}</Text>
                  </div>
                  <div className="trade-quote-stat trade-quote-stat--reserved">
                    <Text className="trade-quote-stat__label">Funding / Countdown</Text>
                    <Text className="trade-quote-stat__value trade-quote-stat__value--muted">{`${fundingRateLabel} / ${fundingCountdownLabel}`}</Text>
                  </div>
                </div>
              </div>
            </Card>

            {accountLiquidating ? (
              <Alert
                showIcon
                type="error"
                message="账户处于 LIQUIDATING（清算中）"
                description="当前账户已进入清算流程，请先关注仓位和账户状态。"
              />
            ) : null}
            {tickerStale ? (
              <Alert
                showIcon
                type="warning"
                message="行情异常或延迟"
                description="当前 MARK PRICE（标记价格）或买卖价更新延迟，请稍后再试。"
              />
            ) : null}
            {marketReduceOnly ? (
              <Alert
                showIcon
                type="warning"
                message="Symbol 处于 REDUCE_ONLY（只减仓）"
                description="当前仅支持 REDUCE / CLOSE（减仓 / 平仓）操作。"
              />
            ) : null}
            {marketPaused ? (
              <Alert
                showIcon
                type="error"
                message="Symbol 已暂停交易"
                description="该交易对暂时不可交易，请稍后再试。"
              />
            ) : null}

            <Row gutter={[20, 20]} align="stretch">
              <Col xs={24} xl={16}>
                <Card
                  className="surface-card trade-chart-card"
                  title={
                    <div className="trade-chart-title">
                      <Segmented
                        value={interval}
                        options={intervalOptions}
                        onChange={(value) => setInterval(value as ChartInterval)}
                        className="trade-interval-switch"
                      />
                    </div>
                  }
                  extra={
                    <Space wrap size={[8, 8]}>
                      <Tag color="cyan">Chart</Tag>
                      <Tag color="blue">Reference Feed</Tag>
                      <StatusTag value={selectedMarket.status} />
                    </Space>
                  }
                >
                  <div className="trade-chart-wrap">
                    <KlineChart symbol={selectedMarket.symbol} interval={interval} dark height={720} />
                  </div>
                  <div className="trade-chart-footnote">
                    <Text type="secondary">更新时间 {formatDateTime(selectedMarket.ticker?.ts)}</Text>
                    <Text type="secondary">图表周期 {interval}</Text>
                  </div>
                </Card>
              </Col>

              <Col xs={24} xl={8}>
                <Space direction="vertical" size={20} style={{ width: '100%' }}>
                  {session ? (
                    <div ref={orderEntryCardRef}>
                      <Card className="surface-card trade-order-entry-card" title={`Order Entry · ${selectedMarket.symbol}`}>
                      <Space direction="vertical" size={14} style={{ width: '100%' }}>
                        <Segmented
                          block
                          value={entrySide}
                          className={`rg-side-segmented rg-side-segmented--${entrySide === 'BUY' ? 'long' : 'short'}`}
                          options={[
                            { label: 'Buy / Long', value: 'BUY' },
                            { label: 'Sell / Short', value: 'SELL' },
                          ]}
                          onChange={(value) => setEntrySide(value as 'BUY' | 'SELL')}
                          disabled={orderEntryDisabled}
                        />
                        <Segmented
                          block
                          value={entryType}
                          options={[
                            { label: 'Market', value: 'MARKET' },
                            { label: 'Limit', value: 'LIMIT' },
                            { label: 'Stop Market', value: 'STOP_MARKET' },
                            { label: 'Take Profit', value: 'TAKE_PROFIT_MARKET' },
                          ]}
                          onChange={(value) => setEntryType(value as EntryOrderType)}
                          disabled={orderEntryDisabled}
                        />
                        <Segmented
                          block
                          value={entryEffect}
                          options={[
                            { label: 'Open', value: 'OPEN' },
                            { label: 'Reduce', value: 'REDUCE' },
                            { label: 'Close', value: 'CLOSE' },
                          ]}
                          onChange={(value) => setEntryEffect(value as 'OPEN' | 'REDUCE' | 'CLOSE')}
                          disabled={orderEntryDisabled}
                        />
                        <Segmented
                          block
                          value={entryMarginMode}
                          options={[
                            { label: '逐仓', value: 'isolated' },
                            { label: '全仓', value: 'cross' },
                          ]}
                          onChange={(value) => setEntryMarginMode(value as EntryMarginMode)}
                          disabled={orderEntryDisabled}
                        />
                        {isLimitOrder ? (
                          <Alert
                            showIcon
                            type="info"
                            message="限价单会进入挂单列表"
                            description="成交后会自动更新到订单、成交和持仓列表。"
                          />
                        ) : isTriggerOrder ? (
                          <Alert
                            showIcon
                            type="info"
                            message={`${primaryTriggerLabel}单按 MARK PRICE（标记价格）触发`}
                            description="触发单会先进入等待触发状态，命中触发价后按市价执行，并继续受滑点保护。"
                          />
                        ) : (
                          <Alert
                            showIcon
                            type="info"
                            message="市价单带滑点保护"
                            description={`你设置的保护阈值是 ${marketProtectText}，超出该范围时订单不会成交。`}
                          />
                        )}
                        {(isLimitOrder || isTriggerOrder) && entryReduceOnly ? (
                          <Alert
                            showIcon
                            type="warning"
                            message={`Reduce-only ${isTriggerOrder ? '触发单' : '限价单'}`}
                            description="该订单只会减少现有风险敞口，不会新开仓位。"
                          />
                        ) : null}
                        <Input
                          value={entryQty}
                          onChange={(event) => setEntryQty(event.target.value)}
                          placeholder={`数量，step ${selectedMarket.step_size}`}
                          disabled={orderEntryDisabled}
                        />
                        {isLimitOrder ? (
                          <>
                            <Input
                              value={entryPrice}
                              onChange={(event) => setEntryPrice(event.target.value)}
                              placeholder={`限价，tick ${selectedMarket.tick_size}`}
                              disabled={orderEntryDisabled}
                              addonAfter={
                                <Space size={4}>
                                  <Button
                                    size="small"
                                    type="text"
                                    onClick={() => setEntryPrice(alignDownToStep(String(selectedMarket.ticker?.best_bid || ''), selectedMarket.tick_size))}
                                  >
                                    买一
                                  </Button>
                                  <Button
                                    size="small"
                                    type="text"
                                    onClick={() => setEntryPrice(alignDownToStep(String(selectedMarket.ticker?.best_ask || ''), selectedMarket.tick_size))}
                                  >
                                    卖一
                                  </Button>
                                  <Button
                                    size="small"
                                    type="text"
                                    onClick={() => setEntryPrice(alignDownToStep(String(selectedMarket.ticker?.mark_price || ''), selectedMarket.tick_size))}
                                  >
                                    标记价
                                  </Button>
                                </Space>
                              }
                            />
                            <Segmented
                              block
                              value={entryTimeInForce}
                              options={[{ label: 'GTC', value: 'GTC' }]}
                              onChange={(value) => setEntryTimeInForce(value as EntryTimeInForce)}
                              disabled={orderEntryDisabled}
                            />
                          </>
                        ) : isTriggerOrder ? (
                          <>
                            <Input
                              value={entryTriggerPrice}
                              onChange={(event) => setEntryTriggerPrice(event.target.value)}
                              placeholder={`触发价，tick ${selectedMarket.tick_size}`}
                              disabled={orderEntryDisabled}
                              addonAfter={
                                <Space size={4}>
                                  <Button
                                    size="small"
                                    type="text"
                                    onClick={() =>
                                      setEntryTriggerPrice(alignDownToStep(String(selectedMarket.ticker?.best_bid || ''), selectedMarket.tick_size))
                                    }
                                  >
                                    买一
                                  </Button>
                                  <Button
                                    size="small"
                                    type="text"
                                    onClick={() =>
                                      setEntryTriggerPrice(alignDownToStep(String(selectedMarket.ticker?.best_ask || ''), selectedMarket.tick_size))
                                    }
                                  >
                                    卖一
                                  </Button>
                                  <Button
                                    size="small"
                                    type="text"
                                    onClick={() =>
                                      setEntryTriggerPrice(alignDownToStep(String(selectedMarket.ticker?.mark_price || ''), selectedMarket.tick_size))
                                    }
                                  >
                                    标记价
                                  </Button>
                                </Space>
                              }
                            />
                            <div className="trade-order-field">
                              <Text type="secondary">滑点保护</Text>
                              <InputNumber
                                min={1}
                                max={500}
                                value={entryMaxSlippageBps}
                                onChange={(value) => setEntryMaxSlippageBps(Number(value ?? 100))}
                                addonAfter="bps"
                                style={{ width: '100%' }}
                                disabled={orderEntryDisabled}
                              />
                            </div>
                          </>
                        ) : (
                          <div className="trade-order-field">
                            <Text type="secondary">滑点保护</Text>
                            <InputNumber
                              min={1}
                              max={500}
                              value={entryMaxSlippageBps}
                              onChange={(value) => setEntryMaxSlippageBps(Number(value ?? 100))}
                              addonAfter="bps"
                              style={{ width: '100%' }}
                              disabled={orderEntryDisabled}
                            />
                          </div>
                        )}
                        <div className="trade-order-field">
                          <Text type="secondary">杠杆</Text>
                          <InputNumber
                            min={1}
                            max={leverageMax}
                            value={entryLeverage}
                            onChange={(value) => setEntryLeverage(Number(value ?? 1))}
                            addonAfter="x"
                            style={{ width: '100%' }}
                            disabled={orderEntryDisabled}
                          />
                        </div>
                        <div className="trade-order-slider">
                          <div className="trade-meta-row">
                            <Text type="secondary">杠杆滑块 / 上限 {leverageMax}x</Text>
                            <Text strong>{entryLeverage}x</Text>
                          </div>
                          <Slider
                            min={1}
                            max={leverageMax}
                            value={entryLeverage}
                            onChange={(value) => setEntryLeverage(Number(value))}
                            disabled={orderEntryDisabled}
                          />
                        </div>
                        <div className="trade-toggle-row">
                          <Text type="secondary">Reduce-Only</Text>
                          <Switch checked={entryReduceOnly} onChange={setEntryReduceOnly} disabled={orderEntryDisabled || entryEffect !== 'OPEN'} />
                        </div>
                        <Card size="small" className="trade-order-summary">
                          <Descriptions size="small" column={1} colon={false}>
                            <Descriptions.Item label="方向">{entrySide === 'BUY' ? '做多 Long' : '做空 Short'}</Descriptions.Item>
                            <Descriptions.Item label="模式">{entryMarginMode === 'cross' ? '全仓 Cross' : '逐仓 Isolated'}</Descriptions.Item>
                            <Descriptions.Item label="价格参考">
                              {isLimitOrder
                                ? `${formatUsd(quoteHintPrice)} (限价)`
                                : isTriggerOrder
                                  ? `${formatUsd(quoteHintPrice)} (${primaryTriggerLabel}触发价)`
                                  : `${formatUsd(referencePrice)} (标记价)`}
                            </Descriptions.Item>
                            <Descriptions.Item label="名义价值">{formatUsd(estimatedNotional)}</Descriptions.Item>
                            <Descriptions.Item label="预估保证金">{entryEffect === 'OPEN' ? formatUsd(estimatedMargin) : '--'}</Descriptions.Item>
                            <Descriptions.Item label="路由">
                              {isLimitOrder
                                ? `RESTING / ${entryTimeInForce}`
                                : isTriggerOrder
                                  ? `TRIGGER_WAIT / ${primaryTriggerLabel} / ${marketProtectText}`
                                  : `IMMEDIATE / ${marketProtectText}`}
                            </Descriptions.Item>
                            <Descriptions.Item label="只减仓">{entryReduceOnly ? '开启' : '关闭'}</Descriptions.Item>
                            {(isLimitOrder || isTriggerOrder) && entryEffect === 'OPEN' ? (
                              <Descriptions.Item label={isTriggerOrder ? '触发冻结预算' : '挂单冻结预算'}>{formatUsd(estimatedReserve)}</Descriptions.Item>
                            ) : null}
                          </Descriptions>
                        </Card>
                        {state?.risk && !state.risk.can_open_risk && entryEffect === 'OPEN' ? (
                          <Alert showIcon type="warning" message="当前账户不可新增风险，只允许减仓或平仓。" />
                        ) : null}
                        <Button type="primary" onClick={() => void handleOpenOrder()} loading={submitting} disabled={orderEntryDisabled}>
                          {entryType === 'MARKET'
                            ? '提交市价单'
                            : entryType === 'LIMIT'
                              ? '提交限价单'
                              : `提交${primaryTriggerLabel}触发单`}
                        </Button>
                        <Text type="secondary">
                          请确认方向、数量、价格与触发条件后再提交订单。
                        </Text>
                      </Space>
                      </Card>
                    </div>
                  ) : (
                    <LoginRequiredCard
                      title="登录后下单"
                      description="连接钱包后即可提交订单、撤单和执行平仓。"
                    />
                  )}
                </Space>
              </Col>
            </Row>

            {session ? (
              <>
                <Card
                  className="table-card trade-activity-card"
                  title="Account Activity"
                  extra={
                    <Space size={[8, 8]} wrap>
                      <Tag color="cyan">All Symbols</Tag>
                      {latestCancelableOrder ? (
                        <Button size="small" onClick={() => void handleCancelOrder(latestCancelableOrder.order_id)} loading={submitting}>
                          Cancel Latest Open Order
                        </Button>
                      ) : null}
                    </Space>
                  }
                >
                  {state?.summary && state.risk ? (
                    <div className="trade-account-overview">
                      <div className="trade-account-overview__stats">
                        <div className="trade-account-stat">
                          <Text className="trade-account-stat__label">Equity</Text>
                          <Text className="trade-account-stat__value">{formatUsd(state.summary.equity)}</Text>
                        </div>
                        <div className="trade-account-stat">
                          <Text className="trade-account-stat__label">Available</Text>
                          <Text className="trade-account-stat__value">{formatUsd(state.summary.available_balance)}</Text>
                        </div>
                        <div className="trade-account-stat">
                          <Text className="trade-account-stat__label">Margin Ratio</Text>
                          <Text className="trade-account-stat__value">{formatPercent(state.summary.margin_ratio)}</Text>
                        </div>
                        <div className="trade-account-stat">
                          <Text className="trade-account-stat__label">Unrealized PnL</Text>
                          <Text className="trade-account-stat__value">{formatSignedUsd(state.summary.unrealized_pnl)}</Text>
                        </div>
                      </div>
                      <div className="trade-activity-summary">
                        <div className="trade-activity-summary__text">
                          <Text strong>Account Overview</Text>
                          <Text type="secondary">Track live balances, risk posture, working orders, and recent execution flow from one unified account view.</Text>
                        </div>
                        <Space wrap size={[8, 8]}>
                          <StatusTag value={state.risk.account_status} />
                          <StatusTag value={state.risk.risk_state} />
                          <Tag color={state.risk.can_open_risk ? 'success' : 'error'}>
                            {state.risk.can_open_risk ? 'OPEN RISK ENABLED' : 'REDUCE ONLY'}
                          </Tag>
                        </Space>
                      </div>
                    </div>
                  ) : null}
                  <Tabs
                    className="trade-activity-tabs"
                    activeKey={activityTab}
                    onChange={(key) => setActivityTab(key as ActivityTabKey)}
                    items={activityTabItems}
                  />
                </Card>
              </>
            ) : null}
          </>
        ) : null}
      </Space>
    </div>
  );
}
