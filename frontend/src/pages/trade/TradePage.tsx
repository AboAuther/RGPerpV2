import { Alert, Card, Col, Row, Space, Spin, Table, Typography } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../../shared/api';
import { EmptyStateCard, ErrorAlert, PageIntro, StatusTag, TwoColumnRow } from '../../shared/components';
import type { OrderItem, PositionItem, SymbolItem, TickerItem } from '../../shared/domain';
import { formatDateTime, formatSignedUsd, formatUsd } from '../../shared/format';

const { Paragraph, Text, Title } = Typography;

interface TradeState {
  symbols: SymbolItem[];
  tickers: TickerItem[];
  orders: OrderItem[];
  positions: PositionItem[];
}

export function TradePage() {
  const [state, setState] = useState<TradeState | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError(null);

      try {
        const [symbols, tickers, orders, positions] = await Promise.all([
          api.market.getSymbols(),
          api.market.getTickers(),
          api.orders.getOrders(),
          api.positions.getPositions(),
        ]);

        if (active) {
          setState({ symbols, tickers, orders, positions });
        }
      } catch (loadError) {
        if (active) {
          setError(loadError);
        }
      } finally {
        if (active) {
          setLoading(false);
        }
      }
    }

    void load();
    return () => {
      active = false;
    };
  }, []);

  const marketRows = useMemo(() => {
    if (!state) {
      return [];
    }

    return state.symbols.map((symbol) => ({
      ...symbol,
      ticker: state.tickers.find((item) => item.symbol === symbol.symbol),
    }));
  }, [state]);

  return (
    <Space direction="vertical" size={20} style={{ width: '100%' }}>
      <PageIntro
        eyebrow="Trading"
        title="Trade Shell"
        description="交易页基础壳已连通行情、订单、仓位读模型。正式下单流属于 Milestone 3，当前先把状态展示与页面骨架按规范落地。"
      />

      <Alert
        showIcon
        type="warning"
        message="当前页面范围"
        description="当前版本只完成交易页骨架、行情展示与历史预览。下单、撤单、触发单和 reduce-only 交互会在 Milestone 3 继续实现。"
      />

      {loading ? <Spin size="large" /> : null}
      <ErrorAlert error={error} />

      {state ? (
        <>
          <Row gutter={[16, 16]}>
            {marketRows.map((row) => (
              <Col xs={24} md={12} xl={8} key={row.symbol}>
                <Card className="surface-card">
                  <Space direction="vertical" size={10}>
                    <Space>
                      <Title level={4} style={{ margin: 0 }}>
                        {row.symbol}
                      </Title>
                      <StatusTag value={row.status} />
                    </Space>
                    <Text type="secondary">{row.asset_class}</Text>
                    <Text>Index: {formatUsd(row.ticker?.index_price)}</Text>
                    <Text>Mark: {formatUsd(row.ticker?.mark_price)}</Text>
                    <Text>Bid / Ask: {formatUsd(row.ticker?.best_bid)} / {formatUsd(row.ticker?.best_ask)}</Text>
                    <Text type="secondary">ts: {formatDateTime(row.ticker?.ts)}</Text>
                  </Space>
                </Card>
              </Col>
            ))}
          </Row>

          <TwoColumnRow
            left={
              <EmptyStateCard
                title="Order Entry"
                description="下单面板将在 Milestone 3 接入实际风控校验、价格保护、reduce-only 和订单状态机。当前先保留布局入口，避免前端在后端未就绪时制造错误真相。 "
                action={<StatusTag value="PENDING" />}
              />
            }
            right={
              <Card className="surface-card" title="Current Exposure">
                <Space direction="vertical" size={10}>
                  {state.positions.map((position) => (
                    <div key={position.position_id}>
                      <Space>
                        <Text strong>{position.symbol}</Text>
                        <StatusTag value={position.side} />
                      </Space>
                      <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                        qty {position.qty} / uPnL {formatSignedUsd(position.unrealized_pnl)}
                      </Paragraph>
                    </div>
                  ))}
                </Space>
              </Card>
            }
          />

          <Card className="table-card" title="Open Orders">
            <Table
              rowKey="order_id"
              dataSource={state.orders}
              pagination={false}
              scroll={{ x: 980 }}
              columns={[
                { title: 'Symbol', dataIndex: 'symbol' },
                { title: 'Side', dataIndex: 'side', render: (value: string) => <StatusTag value={value} /> },
                { title: 'Type', dataIndex: 'type' },
                { title: 'Qty', dataIndex: 'qty', align: 'right' },
                { title: 'Filled', dataIndex: 'filled_qty', align: 'right' },
                { title: 'Avg Fill', dataIndex: 'avg_fill_price', align: 'right', render: (value: string) => formatUsd(value) },
                { title: 'Status', dataIndex: 'status', render: (value: string) => <StatusTag value={value} /> },
              ]}
            />
          </Card>
        </>
      ) : null}
    </Space>
  );
}
