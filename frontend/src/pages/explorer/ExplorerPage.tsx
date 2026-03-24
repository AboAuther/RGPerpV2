import { Button, Card, Descriptions, Input, Space, Spin, Table, Tag, Typography } from 'antd';
import { useEffect, useState } from 'react';
import { api } from '../../shared/api';
import { ErrorAlert, LoginRequiredCard, PageIntro, StatusTag } from '../../shared/components';
import type { ExplorerEvent } from '../../shared/domain';
import { formatAddress, formatDateTime, formatDecimalAdaptive } from '../../shared/format';
import { hasAdminAccess, useAuth } from '../../shared/auth';

const { Paragraph, Text } = Typography;

const primarySummaryKeys = [
  'symbol',
  'status',
  'side',
  'type',
  'position_effect',
  'qty',
  'price',
  'avg_entry_price',
  'execution_price',
  'fee_amount',
  'gross_amount',
  'net_amount',
  'frozen_margin',
  'frozen_initial_margin',
  'frozen_fee',
  'liquidated_qty',
  'penalty_amount',
  'insurance_fund_used',
  'released_margin',
  'confirmations',
  'block_number',
  'block_height',
  'chain_id',
  'triggered_by',
  'kind',
] as const;

const detailLabels: Record<string, string> = {
  amount: '金额',
  asset: '资产',
  avg_entry_price: '开仓均价',
  block_height: '区块高度',
  block_number: '区块号',
  chain_id: '链 ID',
  client_order_id: '客户端订单号',
  confirmations: '确认数',
  event_id: '事件 ID',
  execution_price: '执行价',
  fee_amount: '手续费',
  fill_id: '成交 ID',
  frozen_fee: '冻结手续费',
  frozen_initial_margin: '冻结初始保证金',
  frozen_margin: '冻结保证金',
  funding_batch_id: '资金费批次',
  gross_amount: '毛额',
  insurance_fund_used: '保险基金使用',
  internal_seq: '内部序号',
  kind: '异常类型',
  ledger_tx_id: '账本交易',
  liquidation_id: '清算 ID',
  liquidated_qty: '清算数量',
  mark_price: '标记价格',
  net_amount: '到账净额',
  order_id: '订单 ID',
  penalty_amount: '罚金',
  position_effect: '仓位动作',
  position_id: '仓位 ID',
  price: '价格',
  qty: '数量',
  released_margin: '释放保证金',
  side: '方向',
  status: '状态',
  symbol: '交易对',
  to_address: '目标地址',
  trace_id: 'Trace ID',
  triggered_by: '触发来源',
  tx_hash: '链上交易',
  type: '订单类型',
  user_id: '用户 ID',
};

const surfacedPayloadKeys = new Set([
  ...primarySummaryKeys,
  'amount',
  'asset',
  'client_order_id',
  'fill_id',
  'funding_batch_id',
  'internal_seq',
  'ledger_tx_id',
  'liquidation_id',
  'order_id',
  'position_id',
  'to_address',
  'trace_id',
  'tx_hash',
  'user_id',
]);

function readPayloadString(payload: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const value = payload[key];
    if (typeof value === 'string' && value.trim()) {
      return value.trim();
    }
    if (typeof value === 'number' || typeof value === 'boolean') {
      return String(value);
    }
  }
  return '';
}

function isNumericString(value: string) {
  return /^-?\d+(\.\d+)?$/.test(value.trim());
}

function formatValue(value: unknown) {
  if (value == null) {
    return '-';
  }
  if (typeof value === 'boolean') {
    return value ? 'true' : 'false';
  }
  if (typeof value === 'number') {
    return formatDecimalAdaptive(value, 8);
  }
  if (typeof value === 'string') {
    const normalized = value.trim();
    if (!normalized) {
      return '-';
    }
    return isNumericString(normalized) ? formatDecimalAdaptive(normalized, 8) : normalized;
  }
  return JSON.stringify(value);
}

function formatAssetAmount(amount: string | null | undefined, asset: string | null | undefined) {
  if (!amount) {
    return '-';
  }
  const formatted = formatValue(amount);
  return asset ? `${formatted} ${asset}` : formatted;
}

function formatDisplayKey(key: string) {
  return detailLabels[key] || key.replace(/_/g, ' ');
}

function buildSummaryItems(event: ExplorerEvent) {
  return primarySummaryKeys.reduce<Array<{ key: string; label: string; value: string }>>((items, key) => {
    const value = event.payload[key];
    if (value == null || value === '') {
      return items;
    }
    items.push({ key, label: formatDisplayKey(key), value: formatValue(value) });
    return items;
  }, []);
}

function buildReferenceItems(event: ExplorerEvent) {
  return [
    { label: 'Event', value: event.event_id },
    { label: 'Ledger', value: event.ledger_tx_id || undefined },
    { label: 'Chain', value: event.chain_tx_hash || readPayloadString(event.payload, 'tx_hash') || undefined },
    { label: 'Order', value: event.order_id || undefined },
    { label: 'Fill', value: event.fill_id || undefined },
    { label: 'Position', value: event.position_id || undefined },
    { label: 'Address', value: event.address || readPayloadString(event.payload, 'to_address', 'address') || undefined },
    { label: 'Trace', value: readPayloadString(event.payload, 'trace_id') || undefined },
  ].reduce<Array<{ label: string; value: string }>>((items, item) => {
    if (item.value) {
      items.push({ label: item.label, value: item.value });
    }
    return items;
  }, []);
}

function buildDetailItems(event: ExplorerEvent) {
  const detailEntries: Array<{ key: string; label: string; value: string }> = [
    { key: 'event_id', label: '事件 ID', value: event.event_id },
    { key: 'event_type', label: '事件类型', value: event.event_type },
    { key: 'created_at', label: '时间', value: formatDateTime(event.created_at) },
  ];

  const add = (key: string, value: string | null | undefined) => {
    if (!value) {
      return;
    }
    detailEntries.push({ key, label: formatDisplayKey(key), value: formatValue(value) });
  };

  add('asset', event.asset || undefined);
  add('amount', event.amount || undefined);
  add('symbol', readPayloadString(event.payload, 'symbol'));
  add('status', readPayloadString(event.payload, 'status'));
  add('side', readPayloadString(event.payload, 'side'));
  add('type', readPayloadString(event.payload, 'type'));
  add('position_effect', readPayloadString(event.payload, 'position_effect'));
  add('qty', readPayloadString(event.payload, 'qty'));
  add('price', readPayloadString(event.payload, 'price'));
  add('avg_entry_price', readPayloadString(event.payload, 'avg_entry_price'));
  add('execution_price', readPayloadString(event.payload, 'execution_price'));
  add('fee_amount', readPayloadString(event.payload, 'fee_amount'));
  add('frozen_margin', readPayloadString(event.payload, 'frozen_margin'));
  add('gross_amount', readPayloadString(event.payload, 'gross_amount'));
  add('net_amount', readPayloadString(event.payload, 'net_amount'));
  add('ledger_tx_id', event.ledger_tx_id || readPayloadString(event.payload, 'ledger_tx_id'));
  add('chain_tx_hash', event.chain_tx_hash || readPayloadString(event.payload, 'tx_hash'));
  add('order_id', event.order_id || readPayloadString(event.payload, 'order_id'));
  add('fill_id', event.fill_id || readPayloadString(event.payload, 'fill_id'));
  add('position_id', event.position_id || readPayloadString(event.payload, 'position_id'));
  add('address', event.address || readPayloadString(event.payload, 'router_address', 'to_address', 'address'));
  add('liquidation_id', readPayloadString(event.payload, 'liquidation_id'));
  add('funding_batch_id', readPayloadString(event.payload, 'funding_batch_id', 'biz_ref_id'));
  add('internal_seq', readPayloadString(event.payload, 'internal_seq'));
  add('trace_id', readPayloadString(event.payload, 'trace_id'));
  add('chain_id', readPayloadString(event.payload, 'chain_id'));
  add('block_height', readPayloadString(event.payload, 'block_height', 'block_number'));
  add('confirmations', readPayloadString(event.payload, 'confirmations'));

  return detailEntries;
}

function buildExtraPayloadItems(event: ExplorerEvent) {
  return Object.entries(event.payload)
    .filter(([key]) => !surfacedPayloadKeys.has(key))
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => ({
      key,
      label: formatDisplayKey(key),
      value: formatValue(value),
      isStructured: typeof value === 'object' && value !== null,
    }));
}

function buildActionSummary(event: ExplorerEvent) {
  const symbol = readPayloadString(event.payload, 'symbol');
  const side = readPayloadString(event.payload, 'side');
  const type = readPayloadString(event.payload, 'type');
  const qty = readPayloadString(event.payload, 'qty');
  const price = readPayloadString(event.payload, 'price', 'avg_entry_price', 'execution_price');
  const status = readPayloadString(event.payload, 'status');
  const address = event.address || readPayloadString(event.payload, 'router_address', 'to_address', 'address');
  const liquidationID = readPayloadString(event.payload, 'liquidation_id');

  if (event.event_type.startsWith('wallet.deposit')) {
    return `充值 ${formatAssetAmount(event.amount, event.asset)}${address ? ` · 地址 ${formatAddress(address, 6)}` : ''}`;
  }
  if (event.event_type.startsWith('wallet.withdraw')) {
    return `提现 ${formatAssetAmount(event.amount, event.asset)}${address ? ` · 到 ${formatAddress(address, 6)}` : ''}`;
  }
  if (event.event_type === 'trade.order.accepted' || event.event_type === 'trade.order.canceled') {
    return `${symbol || event.asset || '订单'} ${side || ''} ${qty ? formatValue(qty) : ''}${type ? ` · ${type}` : ''}`.trim();
  }
  if (event.event_type === 'trade.fill.created') {
    return `${symbol || '成交'} ${side || ''} ${qty ? formatValue(qty) : ''}${price ? ` @ ${formatValue(price)}` : ''}`.trim();
  }
  if (event.event_type === 'trade.position.updated') {
    return `${symbol || '仓位'} ${side || ''}${qty ? ` · 持仓 ${formatValue(qty)}` : ''}${status ? ` · ${status}` : ''}`.trim();
  }
  if (event.event_type.startsWith('risk.liquidation')) {
    return `清算${liquidationID ? ` ${formatAddress(liquidationID, 6)}` : ''}${status ? ` · ${status}` : ''}`;
  }
  if (event.event_type === 'hedge.updated') {
    return `对冲 ${symbol || ''} ${side || ''}${qty ? ` · ${formatValue(qty)}` : ''}${status ? ` · ${status}` : ''}`.trim();
  }
  if (event.event_type === 'ledger.committed') {
    return `${readPayloadString(event.payload, 'biz_type') || '账本事件'}${event.amount ? ` · ${formatAssetAmount(event.amount, event.asset)}` : ''}`;
  }

  const summaryItems = buildSummaryItems(event).slice(0, 2);
  if (!summaryItems.length) {
    return '展开查看结构化详情';
  }
  return summaryItems.map((item) => `${item.label} ${item.value}`).join(' · ');
}

function EventSummary({ event }: { event: ExplorerEvent }) {
  const summaryItems = buildSummaryItems(event);
  const actionSummary = buildActionSummary(event);

  if (!summaryItems.length) {
    return <Text type="secondary">{actionSummary}</Text>;
  }

  return (
    <Space direction="vertical" size={6} style={{ width: '100%' }}>
      <Text className="explorer-action-summary">{actionSummary}</Text>
      <Space size={[6, 6]} wrap>
        {summaryItems.slice(0, 4).map((item) => (
          <Tag key={item.key} bordered={false} color={item.key === 'status' || item.key === 'side' ? undefined : 'default'}>
            {item.label}: {item.value}
          </Tag>
        ))}
      </Space>
      {summaryItems.length > 4 ? (
        <Text type="secondary" className="explorer-summary-more">
          另有 {summaryItems.length - 4} 个字段，展开查看详情
        </Text>
      ) : null}
    </Space>
  );
}

function EventReferences({ event }: { event: ExplorerEvent }) {
  const references = buildReferenceItems(event);

  if (!references.length) {
    return <Text type="secondary">-</Text>;
  }

  return (
    <Space direction="vertical" size={4} style={{ width: '100%' }}>
      {references.slice(0, 4).map((item) => (
        <div key={item.label} className="explorer-reference-item">
          <Text type="secondary" className="explorer-reference-label">
            {item.label}
          </Text>
          <Text className="explorer-reference-value">{formatAddress(item.value, 8)}</Text>
        </div>
      ))}
      {references.length > 4 ? (
        <Text type="secondary" className="explorer-summary-more">
          另有 {references.length - 4} 个关联字段
        </Text>
      ) : null}
    </Space>
  );
}

function ExpandedEventDetails({ event }: { event: ExplorerEvent }) {
  const details = buildDetailItems(event);
  const extraPayload = buildExtraPayloadItems(event);

  return (
    <div className="explorer-expanded">
      <Descriptions size="small" column={3} colon={false} className="explorer-descriptions">
        {details.map((item) => (
          <Descriptions.Item key={item.key} label={item.label}>
            <Text copyable={item.value !== '-' ? { text: item.value } : false} className="explorer-detail-value">
              {item.value}
            </Text>
          </Descriptions.Item>
        ))}
      </Descriptions>

      {extraPayload.length ? (
        <div className="explorer-extra-payload">
          <Text className="explorer-section-title">补充字段</Text>
          <div className="explorer-extra-grid">
            {extraPayload.map((item) => (
              <div key={item.key} className="explorer-extra-item">
                <Text type="secondary" className="explorer-extra-label">
                  {item.label}
                </Text>
                <Text className="explorer-extra-value">
                  {item.isStructured ? JSON.stringify(event.payload[item.key], null, 2) : item.value}
                </Text>
              </div>
            ))}
          </div>
        </div>
      ) : null}

      <div className="explorer-json-panel">
        <Text className="explorer-section-title">Raw Payload</Text>
        <pre className="explorer-json-block">{JSON.stringify(event.payload, null, 2)}</pre>
      </div>
    </div>
  );
}

export function ExplorerPage() {
  const { session } = useAuth();
  const isAdminExplorer = hasAdminAccess(session?.user);
  const [events, setEvents] = useState<ExplorerEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [query, setQuery] = useState('');

  async function loadData(background = false, search = query) {
    if (background && events.length > 0) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);

    try {
      const response = await api.explorer.getEvents({ query: search.trim() || undefined, limit: 100 });
      setEvents(response);
    } catch (loadError) {
      setError(loadError);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    if (!session) {
      setEvents([]);
      setLoading(false);
      setRefreshing(false);
      setError(null);
      return;
    }
    const timer = window.setTimeout(() => {
      void loadData(false, query);
    }, 200);
    return () => window.clearTimeout(timer);
  }, [session, query]);

  return (
    <div className="rg-app-page rg-app-page--explorer">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow={isAdminExplorer ? 'Admin Explorer' : 'Explorer'}
          title={isAdminExplorer ? 'Admin Explorer' : 'Event Explorer'}
          description={
            isAdminExplorer
              ? '管理员视图，展示资金、订单、成交、仓位、风控与运营事件，并提供结构化摘要。'
              : '查询资金、订单、成交和仓位事件，并用结构化摘要查看关键字段。'
          }
          titleEffect="shiny"
          descriptionEffect="proximity"
          extra={
            <Button onClick={() => void loadData(true, query)} loading={refreshing}>
              刷新事件
            </Button>
          }
        />

        <Card className="surface-card">
          <Space direction="vertical" style={{ width: '100%' }}>
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索 event_id / event_type / symbol / tx_hash / ledger_tx_id / order_id / fill_id / position_id / address"
            />
            <Paragraph type="secondary" style={{ marginBottom: 0 }}>
              {isAdminExplorer
                ? '管理员可搜索事件类型、交易对、链上哈希、账本交易、订单、成交、仓位、地址以及风控运营字段。'
                : '支持按事件类型、交易对、链上哈希、账本交易、订单、成交、仓位、地址和 payload 字段搜索。'}
            </Paragraph>
          </Space>
        </Card>

        {loading ? <Spin size="large" /> : null}
        <ErrorAlert error={error} />
        {!session ? <LoginRequiredCard title="登录后查询 Explorer" description="Explorer 允许未登录进入页面，但资金事件、充值提现追踪和链上哈希检索需要登录后才可查询。" /> : null}

        <Card className="table-card" title={`${isAdminExplorer ? 'Admin Events' : 'Events'}${session ? ` · ${events.length}` : ''}`}>
          <Table
            rowKey="event_id"
            dataSource={events}
            scroll={{ x: 1320 }}
            pagination={false}
            expandable={{
              expandRowByClick: true,
              expandedRowRender: (event) => <ExpandedEventDetails event={event} />,
            }}
            columns={[
              { title: 'Time', dataIndex: 'created_at', width: 180, render: (value: string) => formatDateTime(value) },
              {
                title: 'Event',
                dataIndex: 'event_type',
                width: 260,
                render: (_value: string, event: ExplorerEvent) => (
                  <Space direction="vertical" size={4}>
                    <StatusTag value={event.event_type} />
                    <Text type="secondary" className="explorer-mono">
                      {formatAddress(event.event_id, 8)}
                    </Text>
                  </Space>
                ),
              },
              {
                title: 'Asset / Amount',
                width: 170,
                render: (_value: unknown, event: ExplorerEvent) => (
                  <Space direction="vertical" size={4}>
                    <Text>{event.asset || '-'}</Text>
                    <Text type="secondary">{formatAssetAmount(event.amount, event.asset)}</Text>
                  </Space>
                ),
              },
              {
                title: 'Summary',
                width: 360,
                render: (_value: unknown, event: ExplorerEvent) => <EventSummary event={event} />,
              },
              {
                title: 'References',
                width: 260,
                render: (_value: unknown, event: ExplorerEvent) => <EventReferences event={event} />,
              },
            ]}
          />
        </Card>
      </Space>
    </div>
  );
}
