import { Card, Space, Spin, Table, Typography } from 'antd';
import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import { api } from '../../shared/api';
import { ErrorAlert, PageIntro, StatusTag } from '../../shared/components';
import type { FillItem, FundingItem, OrderItem, TransferItem } from '../../shared/domain';
import { formatDateTime, formatSignedUsd, formatUsd } from '../../shared/format';

const { Text } = Typography;

function useHistoryLoader<T>(loader: () => Promise<T[]>) {
  const [data, setData] = useState<T[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError(null);
      try {
        const response = await loader();
        if (active) {
          setData(response);
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
  }, [loader]);

  return { data, loading, error };
}

export function OrdersHistoryPage() {
  const { data, loading, error } = useHistoryLoader<OrderItem>(api.orders.getOrders);

  return (
    <HistoryPageScaffold
      eyebrow="History"
      title="Orders"
      description="订单状态必须区分 ACCEPTED、PARTIALLY_FILLED、TRIGGER_WAIT、SYSTEM_CANCELED 等语义。"
      loading={loading}
      error={error}
    >
      <Table
        rowKey="order_id"
        dataSource={data}
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
    </HistoryPageScaffold>
  );
}

export function FillsHistoryPage() {
  const { data, loading, error } = useHistoryLoader<FillItem>(api.fills.getFills);

  return (
    <HistoryPageScaffold
      eyebrow="History"
      title="Fills"
      description="成交记录是订单执行结果，不应被前端乐观推断。"
      loading={loading}
      error={error}
    >
      <Table
        rowKey="fill_id"
        dataSource={data}
        pagination={false}
        columns={[
          { title: 'Time', dataIndex: 'created_at', render: (value: string) => formatDateTime(value) },
          { title: 'Symbol', dataIndex: 'symbol' },
          { title: 'Side', dataIndex: 'side', render: (value: string) => <StatusTag value={value} /> },
          { title: 'Qty', dataIndex: 'qty', align: 'right' },
          { title: 'Price', dataIndex: 'price', align: 'right', render: (value: string) => formatUsd(value) },
          { title: 'Fee', dataIndex: 'fee_amount', align: 'right', render: (value: string) => formatUsd(value) },
        ]}
      />
    </HistoryPageScaffold>
  );
}

export function FundingHistoryPage() {
  const { data, loading, error } = useHistoryLoader<FundingItem>(api.funding.getHistory);

  return (
    <HistoryPageScaffold
      eyebrow="History"
      title="Funding"
      description="资金费历史是只读状态，实际扣收与返还以后端结算批次和账本为准。"
      loading={loading}
      error={error}
    >
      <Table
        rowKey="funding_id"
        dataSource={data}
        pagination={false}
        columns={[
          { title: 'Time', dataIndex: 'settled_at', render: (value: string) => formatDateTime(value) },
          { title: 'Symbol', dataIndex: 'symbol' },
          { title: 'Direction', dataIndex: 'direction', render: (value: string) => <StatusTag value={value} /> },
          { title: 'Rate', dataIndex: 'rate', align: 'right' },
          { title: 'Amount', dataIndex: 'amount', align: 'right', render: (value: string) => formatSignedUsd(value) },
          { title: 'Batch', dataIndex: 'batch_id', render: (value: string) => <Text type="secondary">{value}</Text> },
        ]}
      />
    </HistoryPageScaffold>
  );
}

export function TransfersHistoryPage() {
  const { data, loading, error } = useHistoryLoader<TransferItem>(api.transfers.getHistory);

  return (
    <HistoryPageScaffold
      eyebrow="History"
      title="Transfers"
      description="内部划转页面预留展示，实际资金变更依然以统一账本分录为准。"
      loading={loading}
      error={error}
    >
      <Table
        rowKey="transfer_id"
        dataSource={data}
        pagination={false}
        columns={[
          { title: 'Time', dataIndex: 'created_at', render: (value: string) => formatDateTime(value) },
          { title: 'Asset', dataIndex: 'asset' },
          { title: 'Amount', dataIndex: 'amount', align: 'right', render: (value: string) => formatUsd(value) },
          { title: 'From', dataIndex: 'from_account' },
          { title: 'To', dataIndex: 'to_account' },
          { title: 'Status', dataIndex: 'status', render: (value: string) => <StatusTag value={value} /> },
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
  error,
  children,
}: {
  eyebrow: string;
  title: string;
  description: string;
  loading: boolean;
  error: unknown;
  children: ReactNode;
}) {
  return (
    <div className={`rg-app-page rg-app-page--history rg-app-page--${title.toLowerCase()}`}>
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro eyebrow={eyebrow} title={title} description={description} />
        {loading ? <Spin size="large" /> : null}
        <ErrorAlert error={error} />
        <Card className="table-card">{children}</Card>
      </Space>
    </div>
  );
}
