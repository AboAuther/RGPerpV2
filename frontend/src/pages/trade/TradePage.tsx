import { Alert, Button, Card, Col, Empty, Input, Row, Segmented, Select, Space, Spin, Switch, Table, Tag, Typography, message } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import KlineChart from '../../components/trading/KlineChart';
import { api } from '../../shared/api';
import { EmptyStateCard, ErrorAlert, LoginRequiredCard, PageIntro, StatusTag } from '../../shared/components';
import { useAuth } from '../../shared/auth';
import type { AccountSummary, FillItem, OrderCreateRequest, OrderItem, PositionItem, RiskSnapshot, SymbolItem, TickerItem } from '../../shared/domain';
import { formatDateTime, formatDecimal, formatPercent, formatSignedUsd, formatUsd } from '../../shared/format';
import { useWindowRefetch } from '../../shared/refetch';

const { Paragraph, Text, Title } = Typography;

type ChartInterval = '1m' | '5m' | '15m' | '1h' | '1d';

interface TradeState {
  symbols: SymbolItem[];
  tickers: TickerItem[];
  orders: OrderItem[];
  fills: FillItem[];
  positions: PositionItem[];
  summary: AccountSummary | null;
  risk: RiskSnapshot | null;
}

const intervalOptions: ChartInterval[] = ['1m', '5m', '15m', '1h', '1d'];

export function TradePage() {
  const { session } = useAuth();
  const [msgApi, contextHolder] = message.useMessage();
  const [state, setState] = useState<TradeState | null>(null);
  const [selectedSymbol, setSelectedSymbol] = useState('BTC-PERP');
  const [interval, setInterval] = useState<ChartInterval>('15m');
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [submitting, setSubmitting] = useState(false);
  const [entrySide, setEntrySide] = useState<'BUY' | 'SELL'>('BUY');
  const [entryType, setEntryType] = useState<'MARKET' | 'LIMIT'>('MARKET');
  const [entryEffect, setEntryEffect] = useState<'OPEN' | 'REDUCE' | 'CLOSE'>('OPEN');
  const [entryQty, setEntryQty] = useState('0.001');
  const [entryPrice, setEntryPrice] = useState('');
  const [entryReduceOnly, setEntryReduceOnly] = useState(false);

  async function loadData(background = false) {
    if (background && state) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);

    try {
      const [symbols, tickers, orders, fills, positions, summary, risk] = await Promise.all([
        api.market.getSymbols(),
        api.market.getTickers(),
        session ? api.orders.getOrders() : Promise.resolve([]),
        session ? api.fills.getFills() : Promise.resolve([]),
        session ? api.positions.getPositions() : Promise.resolve([]),
        session ? api.account.getSummary() : Promise.resolve(null),
        session ? api.account.getRisk() : Promise.resolve(null),
      ]);

      setState({
        symbols,
        tickers,
        orders,
        fills,
        positions,
        summary,
        risk,
      });
    } catch (loadError) {
      setError(loadError);
    } finally {
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
    };
  }, [selectedSymbol, state]);

  const selectedOrders = useMemo(() => state?.orders.filter((item) => item.symbol === selectedSymbol) || [], [selectedSymbol, state?.orders]);
  const selectedFills = useMemo(() => state?.fills.filter((item) => item.symbol === selectedSymbol) || [], [selectedSymbol, state?.fills]);
  const selectedPositions = useMemo(
    () => state?.positions.filter((item) => item.symbol === selectedSymbol && item.status === 'OPEN') || [],
    [selectedSymbol, state?.positions],
  );
  const latestRestingOrder = useMemo(
    () => selectedOrders.find((item) => item.status === 'RESTING') ?? null,
    [selectedOrders],
  );

  const markPriceValue = Number(selectedMarket?.ticker?.mark_price ?? 0);
  const indexPriceValue = Number(selectedMarket?.ticker?.index_price ?? 0);
  const priceDelta = Number.isFinite(markPriceValue) && Number.isFinite(indexPriceValue) ? markPriceValue - indexPriceValue : 0;
  const priceDeltaPercent =
    Number.isFinite(markPriceValue) && Number.isFinite(indexPriceValue) && indexPriceValue > 0 ? priceDelta / indexPriceValue : 0;
  const priceTone = priceDelta > 0 ? 'up' : priceDelta < 0 ? 'down' : 'flat';

  const marketOptions = useMemo(
    () =>
      (state?.symbols || []).map((item) => ({
        label: `${item.symbol} · ${item.asset_class}`,
        value: item.symbol,
      })),
    [state?.symbols],
  );

  const orderEntryDisabled =
    !session ||
    !selectedMarket ||
    submitting ||
    selectedMarket.status === 'PAUSED' ||
    (entryEffect === 'OPEN'
      ? selectedMarket.status !== 'TRADING' || (state?.risk ? !state.risk.can_open_risk : false)
      : !['TRADING', 'REDUCE_ONLY'].includes(selectedMarket.status));

  const referencePrice = entryType === 'LIMIT' && entryPrice ? Number(entryPrice) : markPriceValue;
  const estimatedNotional = Number(entryQty || 0) * (Number.isFinite(referencePrice) ? referencePrice : 0);

  useEffect(() => {
    if (entryEffect === 'OPEN') {
      setEntryReduceOnly(false);
      return;
    }
    setEntryReduceOnly(true);
  }, [entryEffect]);

  async function submitOrder(input: OrderCreateRequest, successText: string) {
    try {
      setSubmitting(true);
      await api.orders.createOrder(input);
      await loadData(true);
      await msgApi.success(successText);
    } catch (submitError) {
      const errorText = submitError instanceof Error ? submitError.message : '请求失败';
      await msgApi.error(errorText);
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
    if (!qty) {
      void msgApi.error('请输入数量');
      return;
    }
    if (entryType === 'LIMIT' && !price) {
      void msgApi.error('请输入限价');
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
        price: entryType === 'LIMIT' ? price : null,
        reduce_only: entryReduceOnly,
        time_in_force: entryType === 'LIMIT' ? 'GTC' : undefined,
      },
      entryType === 'MARKET' ? '市价订单已提交' : '限价订单已提交',
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
        reduce_only: true,
      },
      `${position.symbol} 市价平仓已提交`,
    );
  }

  async function handleCancelOrder(orderId: string) {
    try {
      setSubmitting(true);
      await api.orders.cancelOrder(orderId);
      await loadData(true);
      await msgApi.success('撤单成功');
    } catch (cancelError) {
      const errorText = cancelError instanceof Error ? cancelError.message : '撤单失败';
      await msgApi.error(errorText);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rg-app-page rg-app-page--trade">
      {contextHolder}
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow="Trading"
          title="Trade Console"
          description="Trade 页面现在只接入后端已实现的读接口：symbol、ticker、订单、成交、持仓、账户摘要和风险快照。下单、撤单与 WebSocket 增量更新会在后续里程碑继续接入。"
          titleEffect="glitch"
          descriptionEffect="proximity"
          extra={
            <Button onClick={() => void loadData(true)} loading={refreshing}>
              刷新交易数据
            </Button>
          }
        />

        <Alert
          showIcon
          type="info"
          message="当前交易页边界"
          description="图表与行情可公开浏览；账户摘要、风险、挂单、成交和持仓属于登录后私有读模型。当前页面不伪造深度盘口、不伪造下单成功，也不在前端推导已成交状态。"
        />

        {loading ? <Spin size="large" /> : null}
        <ErrorAlert error={error} />

        {!loading && !state?.symbols.length ? (
          <EmptyStateCard title="暂无可交易 symbol" description="后端尚未返回 symbol 元数据。待市场配置完成后，这里会自动展示可交易合约。" />
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
                    <Text className="trade-quote-stat__value">{formatUsd(selectedMarket.ticker?.index_price)}</Text>
                  </div>
                  <div className="trade-quote-stat">
                    <Text className="trade-quote-stat__label">Best Bid</Text>
                    <Text className="trade-quote-stat__value">{formatUsd(selectedMarket.ticker?.best_bid)}</Text>
                  </div>
                  <div className="trade-quote-stat">
                    <Text className="trade-quote-stat__label">Best Ask</Text>
                    <Text className="trade-quote-stat__value">{formatUsd(selectedMarket.ticker?.best_ask)}</Text>
                  </div>
                  <div className="trade-quote-stat">
                    <Text className="trade-quote-stat__label">Updated</Text>
                    <Text className="trade-quote-stat__value">{formatDateTime(selectedMarket.ticker?.ts)}</Text>
                  </div>
                  <div className="trade-quote-stat trade-quote-stat--reserved">
                    <Text className="trade-quote-stat__label">Funding / Countdown</Text>
                    <Text className="trade-quote-stat__value trade-quote-stat__value--muted">-- / --</Text>
                    <Text className="trade-quote-stat__note">预留 funding rate 展示位</Text>
                  </div>
                </div>
              </div>
            </Card>

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
                    <Card className="surface-card" title={`Order Entry · ${selectedMarket.symbol}`}>
                      <Space direction="vertical" size={12} style={{ width: '100%' }}>
                        <Segmented
                          block
                          value={entrySide}
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
                          ]}
                          onChange={(value) => setEntryType(value as 'MARKET' | 'LIMIT')}
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
                        <Input
                          value={entryQty}
                          onChange={(event) => setEntryQty(event.target.value)}
                          placeholder={`数量，step ${selectedMarket.step_size}`}
                          disabled={orderEntryDisabled}
                        />
                        {entryType === 'LIMIT' ? (
                          <Input
                            value={entryPrice}
                            onChange={(event) => setEntryPrice(event.target.value)}
                            placeholder={`限价，tick ${selectedMarket.tick_size}`}
                            disabled={orderEntryDisabled}
                            addonAfter={
                              <Space size={4}>
                                <Button size="small" type="text" onClick={() => setEntryPrice(String(selectedMarket.ticker?.best_bid || ''))}>
                                  买一
                                </Button>
                                <Button size="small" type="text" onClick={() => setEntryPrice(String(selectedMarket.ticker?.best_ask || ''))}>
                                  卖一
                                </Button>
                                <Button size="small" type="text" onClick={() => setEntryPrice(String(selectedMarket.ticker?.mark_price || ''))}>
                                  标记价
                                </Button>
                              </Space>
                            }
                          />
                        ) : null}
                        <div className="trade-toggle-row">
                          <Text type="secondary">Reduce-Only</Text>
                          <Switch checked={entryReduceOnly} onChange={setEntryReduceOnly} disabled={orderEntryDisabled || entryEffect !== 'OPEN'} />
                        </div>
                        <Card size="small" className="trade-order-summary">
                          <div className="trade-meta-row">
                            <Text type="secondary">Reference Price</Text>
                            <Text strong>{formatUsd(referencePrice)}</Text>
                          </div>
                          <div className="trade-meta-row">
                            <Text type="secondary">Estimated Notional</Text>
                            <Text strong>{formatUsd(estimatedNotional)}</Text>
                          </div>
                          <div className="trade-meta-row">
                            <Text type="secondary">Routing</Text>
                            <Text strong>{entryType === 'LIMIT' ? 'RESTING / GTC' : 'IMMEDIATE'}</Text>
                          </div>
                        </Card>
                        {state?.risk && !state.risk.can_open_risk && entryEffect === 'OPEN' ? (
                          <Alert showIcon type="warning" message="当前账户不可新增风险，只允许减仓或平仓。" />
                        ) : null}
                        <Button type="primary" onClick={() => void handleOpenOrder()} loading={submitting} disabled={orderEntryDisabled}>
                          {entryType === 'MARKET' ? '提交市价单' : '提交限价单'}
                        </Button>
                        <Text type="secondary">
                          当前直接接后端真实订单接口。支持 `MARKET / LIMIT`，并显式传递 `position_effect`、`reduce_only` 和 `time_in_force`。
                        </Text>
                      </Space>
                    </Card>
                  ) : (
                    <LoginRequiredCard
                      title="登录后下单"
                      description="下单模块已经接入真实后端接口。连接钱包登录后，才能提交订单、撤单和执行平仓。"
                    />
                  )}

                  {session && state?.summary && state.risk ? (
                    <Card className="surface-card" title="Account Risk">
                      <Space direction="vertical" size={12} style={{ width: '100%' }}>
                        <div className="trade-meta-row">
                          <Text type="secondary">Equity</Text>
                          <Text strong>{formatUsd(state.summary.equity)}</Text>
                        </div>
                        <div className="trade-meta-row">
                          <Text type="secondary">Available</Text>
                          <Text strong>{formatUsd(state.summary.available_balance)}</Text>
                        </div>
                        <div className="trade-meta-row">
                          <Text type="secondary">Margin Ratio</Text>
                          <Text strong>{formatPercent(state.summary.margin_ratio)}</Text>
                        </div>
                        <Space wrap size={[8, 8]}>
                          <StatusTag value={state.risk.account_status} />
                          <StatusTag value={state.risk.risk_state} />
                          <Tag color={state.risk.can_open_risk ? 'success' : 'error'}>
                            {state.risk.can_open_risk ? 'CAN_OPEN_RISK' : 'NO_NEW_RISK'}
                          </Tag>
                        </Space>
                        {state.risk.notes.length ? (
                          <Space direction="vertical" size={6}>
                            {state.risk.notes.map((note) => (
                              <Text type="secondary" key={note}>
                                {note}
                              </Text>
                            ))}
                          </Space>
                        ) : null}
                      </Space>
                    </Card>
                  ) : (
                    <LoginRequiredCard
                      title="登录后查看账户风险"
                      description="Trade 页的账户权益、保证金占用和风险状态来自私有读模型；未登录时只展示公共市场信息与图表。"
                    />
                  )}

                </Space>
              </Col>
            </Row>

            {session ? (
              <>
                <Row gutter={[20, 20]}>
                  <Col xs={24} xl={12}>
                    <Card className="table-card" title={`Positions · ${selectedMarket.symbol}`}>
                      {selectedPositions.length ? (
                        <Table
                          rowKey="position_id"
                          dataSource={selectedPositions}
                          pagination={false}
                          locale={{ emptyText: '当前 symbol 没有持仓' }}
                          scroll={{ x: 920 }}
                          columns={[
                            { title: 'Side', dataIndex: 'side', render: (value: string) => <StatusTag value={value} /> },
                            { title: 'Qty', dataIndex: 'qty', align: 'right', render: (value: string) => formatDecimal(value, 4) },
                            { title: 'Entry', dataIndex: 'avg_entry_price', align: 'right', render: (value: string) => formatUsd(value) },
                            { title: 'Mark', dataIndex: 'mark_price', align: 'right', render: (value: string) => formatUsd(value) },
                            { title: 'uPnL', dataIndex: 'unrealized_pnl', align: 'right', render: (value: string) => formatSignedUsd(value) },
                            { title: 'Funding', dataIndex: 'funding_accrual', align: 'right', render: (value: string) => formatSignedUsd(value) },
                            { title: 'Liq', dataIndex: 'liquidation_price', align: 'right', render: (value: string) => formatUsd(value) },
                            {
                              title: 'Action',
                              render: (_, record) => (
                                <Button
                                  size="small"
                                  onClick={() => void handleClosePosition(record)}
                                  loading={submitting}
                                  disabled={!['TRADING', 'REDUCE_ONLY'].includes(selectedMarket.status)}
                                >
                                  市价全平
                                </Button>
                              ),
                            },
                          ]}
                        />
                      ) : (
                        <Empty description="当前 symbol 没有持仓" image={Empty.PRESENTED_IMAGE_SIMPLE} />
                      )}
                    </Card>
                  </Col>

                  <Col xs={24} xl={12}>
                    <Space direction="vertical" size={20} style={{ width: '100%' }}>
                      {latestRestingOrder ? (
                        <Card className="surface-card" title="Resting Order Actions">
                          <Space direction="vertical" size={12} style={{ width: '100%' }}>
                            <div className="trade-meta-row">
                              <Text type="secondary">Latest Resting Order</Text>
                              <Text strong>
                                {latestRestingOrder.side} / {latestRestingOrder.type} / {latestRestingOrder.position_effect}
                              </Text>
                            </div>
                            <div className="trade-meta-row">
                              <Text type="secondary">Qty / Price</Text>
                              <Text strong>
                                {formatDecimal(latestRestingOrder.qty, 4)} /{' '}
                                {latestRestingOrder.price ? formatUsd(latestRestingOrder.price) : '--'}
                              </Text>
                            </div>
                            <Button onClick={() => void handleCancelOrder(latestRestingOrder.order_id)} loading={submitting}>
                              取消最近挂单
                            </Button>
                          </Space>
                        </Card>
                      ) : null}

                      <Card className="table-card" title={`Open Orders · ${selectedMarket.symbol}`}>
                      <Table
                        rowKey="order_id"
                        dataSource={selectedOrders}
                        pagination={false}
                        locale={{ emptyText: '当前 symbol 暂无挂单' }}
                        scroll={{ x: 860 }}
                        columns={[
                          { title: 'Side', dataIndex: 'side', render: (value: string) => <StatusTag value={value} /> },
                          { title: 'Type', dataIndex: 'type' },
                          { title: 'Effect', dataIndex: 'position_effect' },
                          { title: 'Qty', dataIndex: 'qty', align: 'right', render: (value: string) => formatDecimal(value, 4) },
                          {
                            title: 'Price / Trigger',
                            render: (_, record) => (
                              <Text type="secondary">{record.price ? formatUsd(record.price) : record.trigger_price ? formatUsd(record.trigger_price) : '--'}</Text>
                            ),
                          },
                          { title: 'Filled', dataIndex: 'filled_qty', align: 'right', render: (value: string) => formatDecimal(value, 4) },
                          { title: 'Status', dataIndex: 'status', render: (value: string) => <StatusTag value={value} /> },
                          {
                            title: 'Action',
                            render: (_, record) =>
                              record.status === 'RESTING' ? (
                                <Button size="small" onClick={() => void handleCancelOrder(record.order_id)} loading={submitting}>
                                  撤单
                                </Button>
                              ) : (
                                <Text type="secondary">--</Text>
                              ),
                          },
                        ]}
                      />
                      </Card>
                    </Space>
                  </Col>
                </Row>

                <Row gutter={[20, 20]}>
                  <Col xs={24}>
                    <Card className="table-card" title={`Recent Fills · ${selectedMarket.symbol}`}>
                      <Table
                        rowKey="fill_id"
                        dataSource={selectedFills}
                        pagination={false}
                        locale={{ emptyText: '当前 symbol 暂无成交' }}
                        scroll={{ x: 860 }}
                        columns={[
                          { title: 'Time', dataIndex: 'created_at', render: (value: string) => formatDateTime(value), width: 180 },
                          { title: 'Side', dataIndex: 'side', render: (value: string) => <StatusTag value={value} /> },
                          { title: 'Qty', dataIndex: 'qty', align: 'right', render: (value: string) => formatDecimal(value, 4) },
                          { title: 'Price', dataIndex: 'price', align: 'right', render: (value: string) => formatUsd(value) },
                          { title: 'Fee', dataIndex: 'fee_amount', align: 'right', render: (value: string) => formatUsd(value) },
                        ]}
                      />
                    </Card>
                  </Col>
                </Row>
              </>
            ) : null}
          </>
        ) : null}
      </Space>
    </div>
  );
}
