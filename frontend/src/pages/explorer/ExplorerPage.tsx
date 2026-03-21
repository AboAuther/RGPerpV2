import { Card, Input, Space, Spin, Table, Typography } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../../shared/api';
import { ErrorAlert, PageIntro, StatusTag } from '../../shared/components';
import type { ExplorerEvent } from '../../shared/domain';
import { formatAddress } from '../../shared/format';

const { Paragraph, Text } = Typography;

export function ExplorerPage() {
  const [events, setEvents] = useState<ExplorerEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [query, setQuery] = useState('');

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError(null);

      try {
        const response = await api.explorer.getEvents();
        if (active) {
          setEvents(response);
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

  const filtered = useMemo(() => {
    if (!query.trim()) {
      return events;
    }

    const keyword = query.toLowerCase();
    return events.filter((event) =>
      [event.event_id, event.event_type, event.ledger_tx_id, event.chain_tx_hash, event.order_id, event.address]
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
        />

      <Card className="surface-card">
        <Space direction="vertical" style={{ width: '100%' }}>
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索 event_id / ledger_tx_id / chain_tx_hash / address"
          />
          <Paragraph type="secondary" style={{ marginBottom: 0 }}>
            若后端缺失某些查询能力，当前页面会回退到 review mock 读模型，便于继续联调 UI。
          </Paragraph>
        </Space>
      </Card>

      {loading ? <Spin size="large" /> : null}
      <ErrorAlert error={error} />

      <Card className="table-card" title="Events">
        <Table
          rowKey="event_id"
          dataSource={filtered}
          scroll={{ x: 1080 }}
          pagination={false}
          columns={[
            { title: 'Event ID', dataIndex: 'event_id', render: (value: string) => <Text code>{formatAddress(value, 10)}</Text> },
            { title: 'Type', dataIndex: 'event_type', render: (value: string) => <StatusTag value={value} /> },
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
