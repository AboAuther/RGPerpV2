import { Button, Card, Space, Spin, Table, Typography } from 'antd';
import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { api } from '../../shared/api';
import { EmptyStateCard, ErrorAlert, LoginRequiredCard, PageIntro, StatusTag } from '../../shared/components';
import type { FillItem, FundingItem, OrderItem, PositionItem, TransferItem } from '../../shared/domain';
import { formatAddress, formatDateTime, formatDecimalAdaptive, formatSignedUsd, formatSignedUsdAdaptive, formatUsd } from '../../shared/format';
import { useWindowRefetch } from '../../shared/refetch';
import { useAuth } from '../../shared/auth';

const { Text } = Typography;

function formatTransferDirection(direction: TransferItem['direction']): string {
  switch (direction) {
    case 'IN':
      return '转入';
    case 'OUT':
      return '转出';
    case 'SELF':
      return '自转';
    default:
      return '未知';
  }
}

function useHistoryLoader<T>(loader: () => Promise<T[]>) {
  const { session } = useAuth();
  const [data, setData] = useState<T[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<unknown>(null);

  async function load(background = false) {
    if (!session) {
      setData([]);
      setLoading(false);
      setRefreshing(false);
      setError(null);
      return;
    }

    if (background) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);

    try {
      const response = await loader();
      setData(response);
    } catch (loadError) {
      setError(loadError);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    void load();
  }, [loader, session]);

  useWindowRefetch(() => {
    void load(true);
  }, !!session);

  return { data, loading, refreshing, error, authenticated: !!session, reload: () => void load(true) };
}

export function OrdersHistoryPage() {
  const { data, loading, refreshing, error, authenticated, reload } = useHistoryLoader<OrderItem>(api.orders.getOrders);

  return (
    <HistoryPageScaffold
      eyebrow="History"
      title="Orders"
      description="查看订单时间、方向、数量、成交进度和状态。"
      loading={loading}
      refreshing={refreshing}
      error={error}
      authenticated={authenticated}
      onRefresh={reload}
      hasData={data.length > 0}
      emptyDescription="当前账户还没有订单记录。"
    >
      <Table
        rowKey="order_id"
        dataSource={data}
        pagination={false}
        scroll={{ x: 980 }}
        columns={[
          { title: '时间', dataIndex: 'created_at', render: (value: string) => formatDateTime(value) },
          { title: '交易对', dataIndex: 'symbol' },
          { title: '方向', dataIndex: 'side', render: (value: string) => <StatusTag value={value} /> },
          { title: '类型', dataIndex: 'type' },
          { title: '数量', dataIndex: 'qty', align: 'right', render: (value: string) => formatDecimalAdaptive(value, 8) },
          { title: '已成交', dataIndex: 'filled_qty', align: 'right', render: (value: string) => formatDecimalAdaptive(value, 8) },
          { title: '成交均价', dataIndex: 'avg_fill_price', align: 'right', render: (value: string) => formatUsd(value, 8) },
          { title: '状态', dataIndex: 'status', render: (value: string) => <StatusTag value={value} /> },
        ]}
      />
    </HistoryPageScaffold>
  );
}

export function FillsHistoryPage() {
  const { data, loading, refreshing, error, authenticated, reload } = useHistoryLoader<FillItem>(api.fills.getFills);

  return (
    <HistoryPageScaffold
      eyebrow="History"
      title="Fills"
      description="查看成交时间、价格、数量和手续费。"
      loading={loading}
      refreshing={refreshing}
      error={error}
      authenticated={authenticated}
      onRefresh={reload}
      hasData={data.length > 0}
      emptyDescription="当前账户还没有成交记录。"
    >
      <Table
        rowKey="fill_id"
        dataSource={data}
        pagination={false}
        columns={[
          { title: '时间', dataIndex: 'created_at', render: (value: string) => formatDateTime(value) },
          { title: '交易对', dataIndex: 'symbol' },
          { title: '方向', dataIndex: 'side', render: (value: string) => <StatusTag value={value} /> },
          { title: '数量', dataIndex: 'qty', align: 'right', render: (value: string) => formatDecimalAdaptive(value, 8) },
          { title: '价格', dataIndex: 'price', align: 'right', render: (value: string) => formatUsd(value, 8) },
          { title: '手续费', dataIndex: 'fee_amount', align: 'right', render: (value: string) => formatUsd(value, 8) },
        ]}
      />
    </HistoryPageScaffold>
  );
}

export function PositionsHistoryPage() {
  const { data, loading, refreshing, error, authenticated, reload } = useHistoryLoader<PositionItem>(api.positions.getPositions);

  return (
    <HistoryPageScaffold
      eyebrow="History"
      title="Positions"
      description="查看各交易对的仓位、盈亏、资金费和状态。"
      loading={loading}
      refreshing={refreshing}
      error={error}
      authenticated={authenticated}
      onRefresh={reload}
      hasData={data.length > 0}
      emptyDescription="当前账户还没有仓位记录。"
    >
      <Table
        rowKey="position_id"
        dataSource={data}
        pagination={false}
        scroll={{ x: 1180 }}
        columns={[
          { title: '交易对', dataIndex: 'symbol' },
          { title: '方向', dataIndex: 'side', render: (value: string) => <StatusTag value={value} /> },
          { title: '数量', dataIndex: 'qty', align: 'right', render: (value: string) => formatDecimalAdaptive(value, 8) },
          { title: '开仓均价', dataIndex: 'avg_entry_price', align: 'right', render: (value: string) => formatUsd(value, 8) },
          { title: '标记价格', dataIndex: 'mark_price', align: 'right', render: (value: string) => formatUsd(value, 8) },
          { title: '未实现盈亏', dataIndex: 'unrealized_pnl', align: 'right', render: (value: string) => formatSignedUsdAdaptive(value, 8) },
          { title: '已实现盈亏', dataIndex: 'realized_pnl', align: 'right', render: (value: string) => formatSignedUsdAdaptive(value, 8) },
          { title: '资金费', dataIndex: 'funding_accrual', align: 'right', render: (value: string) => formatSignedUsdAdaptive(value, 8) },
          { title: '状态', dataIndex: 'status', render: (value: string) => <StatusTag value={value} /> },
        ]}
      />
    </HistoryPageScaffold>
  );
}

export function FundingHistoryPage() {
  const { data, loading, refreshing, error, authenticated, reload } = useHistoryLoader<FundingItem>(api.funding.getHistory);

  return (
    <HistoryPageScaffold
      eyebrow="History"
      title="Funding"
      description="查看资金费率结算记录与结算方向。"
      loading={loading}
      refreshing={refreshing}
      error={error}
      authenticated={authenticated}
      onRefresh={reload}
      hasData={data.length > 0}
      emptyDescription="当前账户还没有资金费结算记录。"
    >
      <Table
        rowKey="funding_id"
        dataSource={data}
        pagination={false}
        columns={[
          { title: '时间', dataIndex: 'settled_at', render: (value: string) => formatDateTime(value) },
          { title: '交易对', dataIndex: 'symbol' },
          { title: '方向', dataIndex: 'direction', render: (value: string) => <StatusTag value={value} /> },
          { title: '费率', dataIndex: 'rate', align: 'right' },
          { title: '金额', dataIndex: 'amount', align: 'right', render: (value: string) => formatSignedUsd(value) },
          { title: '批次', dataIndex: 'batch_id', render: (value: string) => <Text type="secondary">{value}</Text> },
        ]}
      />
    </HistoryPageScaffold>
  );
}

export function TransfersHistoryPage() {
  const { data, loading, refreshing, error, authenticated, reload } = useHistoryLoader<TransferItem>(api.transfers.getHistory);

  return (
    <HistoryPageScaffold
      eyebrow="History"
      title="Transfers"
      description="查看账户内部划转记录。"
      loading={loading}
      refreshing={refreshing}
      error={error}
      authenticated={authenticated}
      onRefresh={reload}
      hasData={data.length > 0}
      emptyDescription="当前账户还没有内部划转记录。"
    >
      <Table
        rowKey="transfer_id"
        dataSource={data}
        pagination={false}
        columns={[
          { title: '时间', dataIndex: 'created_at', render: (value: string) => formatDateTime(value) },
          { title: '方向', dataIndex: 'direction', render: (value: TransferItem['direction']) => formatTransferDirection(value) },
          { title: '对手方地址', dataIndex: 'counterparty_address', render: (value: string) => (value ? formatAddress(value, 8) : '--') },
          { title: '资产', dataIndex: 'asset' },
          { title: '金额', dataIndex: 'amount', align: 'right', render: (value: string) => formatUsd(value, 8) },
          { title: '状态', dataIndex: 'status', render: (value: string) => <StatusTag value={value} /> },
        ]}
      />
    </HistoryPageScaffold>
  );
}

function HistoryPageScaffold({
  eyebrow,
  title,
  description,
  loading,
  refreshing,
  error,
  authenticated,
  onRefresh,
  hasData,
  emptyDescription,
  children,
}: {
  eyebrow: string;
  title: string;
  description: string;
  loading: boolean;
  refreshing: boolean;
  error: unknown;
  authenticated: boolean;
  onRefresh: () => void;
  hasData: boolean;
  emptyDescription: string;
  children: ReactNode;
}) {
  return (
    <div className={`rg-app-page rg-app-page--history rg-app-page--${title.toLowerCase()}`}>
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow={eyebrow}
          title={title}
          description={description}
          titleEffect="shiny"
          descriptionEffect="proximity"
          extra={
            authenticated ? (
              <Button onClick={onRefresh} loading={refreshing}>
                刷新记录
              </Button>
            ) : null
          }
        />
        {loading ? <Spin size="large" /> : null}
        <ErrorAlert error={error} />
        {!authenticated ? <LoginRequiredCard title={`登录后查看 ${title}`} description="历史页允许未登录进入，但订单、成交、资金费和内部转账都属于账户私有数据，需登录后才能查询。" /> : null}
        {authenticated && !loading && !error && !hasData ? <EmptyStateCard title={`${title} 暂无数据`} description={emptyDescription} /> : null}
        {authenticated && hasData ? <Card className="table-card">{children}</Card> : null}
      </Space>
    </div>
  );
}
