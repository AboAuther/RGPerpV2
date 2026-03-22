import { Button, Card, Input, Space, Spin, Table, Typography } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../../shared/api';
import { ErrorAlert, LoginRequiredCard, PageIntro, StatusTag } from '../../shared/components';
import type { ExplorerEvent } from '../../shared/domain';
import { formatAddress, formatDateTime, formatUsd } from '../../shared/format';
import { useAuth } from '../../shared/auth';

const { Paragraph, Text } = Typography;

export function ExplorerPage() {
  const { session } = useAuth();
  const [events, setEvents] = useState<ExplorerEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [query, setQuery] = useState('');

  async function loadData(background = false) {
    if (background && events.length > 0) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);

    try {
      const response = await api.explorer.getEvents();
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
    void loadData();
  }, [session]);

  const filtered = useMemo(() => {
    if (!query.trim()) {
      return events;
    }

    const keyword = query.toLowerCase();
    return events.filter((event) =>
      [event.event_type, event.asset, event.amount, event.ledger_tx_id, event.chain_tx_hash, event.order_id, event.address]
        .filter(Boolean)
        .some((item) => String(item).toLowerCase().includes(keyword)),
    );
  }, [events, query]);

  return (
    <div className="rg-app-page rg-app-page--explorer">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow="Explorer"
          title="Event Explorer"
          description="Explorer 是读模型，不反向修改账本或订单源表。当前支持按事件 ID、链上 hash、ledger_tx_id 和地址检索。"
          titleEffect="shiny"
          descriptionEffect="proximity"
          extra={
            <Button onClick={() => void loadData(true)} loading={refreshing}>
              刷新事件
            </Button>
          }
        />

      <Card className="surface-card">
        <Space direction="vertical" style={{ width: '100%' }}>
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索 event_type / asset / ledger_tx_id / chain_tx_hash / address"
          />
          <Paragraph type="secondary" style={{ marginBottom: 0 }}>
            Explorer 事件流只展示后端返回的读模型结果；查询失败会直接报错，不会自动伪造审计事件。
          </Paragraph>
        </Space>
      </Card>

      {loading ? <Spin size="large" /> : null}
      <ErrorAlert error={error} />
      {!session ? <LoginRequiredCard title="登录后查询 Explorer" description="Explorer 允许未登录进入页面，但资金事件、充值提现追踪和链上哈希检索需要登录后才可查询。" /> : null}

      <Card className="table-card" title="Events">
        <Table
          rowKey="event_id"
          dataSource={filtered}
          scroll={{ x: 1080 }}
          pagination={false}
          columns={[
            { title: 'Time', dataIndex: 'created_at', width: 180, render: (value: string) => formatDateTime(value) },
            { title: 'Type', dataIndex: 'event_type', render: (value: string) => <StatusTag value={value} /> },
            { title: 'Asset', dataIndex: 'asset', width: 90, render: (value: string | null | undefined) => <Text type="secondary">{value || '-'}</Text> },
            {
              title: 'Amount',
              dataIndex: 'amount',
              align: 'right',
              width: 140,
              render: (value: string | null | undefined) => <Text type="secondary">{value ? formatUsd(value) : '-'}</Text>,
            },
            { title: 'Ledger Tx', dataIndex: 'ledger_tx_id', render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text> },
            { title: 'Chain Tx', dataIndex: 'chain_tx_hash', render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text> },
            { title: 'Order', dataIndex: 'order_id', render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text> },
            { title: 'Address', dataIndex: 'address', render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text> },
            {
              title: 'Payload',
              dataIndex: 'payload',
              render: (value: Record<string, unknown>) => (
                <Text type="secondary">{JSON.stringify(value)}</Text>
              ),
            },
          ]}
        />
      </Card>
      </Space>
    </div>
  );
}
