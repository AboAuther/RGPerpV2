import { Alert, Button, Card, List, Space, Table, Typography } from 'antd';
import { useEffect, useState } from 'react';
import { api } from '../../shared/api';
import { EmptyStateCard, ErrorAlert, PageIntro, StatusTag } from '../../shared/components';
import type { AdminWithdrawReviewItem } from '../../shared/domain';
import { formatAddress, formatChainName, formatDateTime, formatUsd } from '../../shared/format';
import { useWindowRefetch } from '../../shared/refetch';
import { useSystemConfig } from '../../shared/system';

const { Text } = Typography;

function AdminPageTemplate({
  title,
  description,
  items,
}: {
  title: string;
  description: string;
  items: string[];
}) {
  return (
    <div className="rg-app-page rg-app-page--admin">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro eyebrow="Admin" title={title} description={description} titleEffect="glitch" descriptionEffect="proximity" />
        <Alert
          showIcon
          type="warning"
          message="后台页仅做前端壳"
          description="管理操作的最终 RBAC、审计、审批链必须由后端控制。当前页面只提供信息架构和后续联调占位。"
        />
        <Card className="surface-card">
          <List
            dataSource={items}
            renderItem={(item) => <List.Item>{item}</List.Item>}
          />
        </Card>
        <EmptyStateCard
          title="待后端联调"
          description="当前后台只保留路由、结构和约束说明，避免在配置、风控、提现审核尚未具备服务端权限闭环时做危险假实现。"
        />
      </Space>
    </div>
  );
}

export function AdminDashboardPage() {
  return (
    <AdminPageTemplate
      title="Admin Dashboard"
      description="聚合风控、提现、资金费率、清算、对账和系统只读状态。"
      items={['风险告警汇总', '提现待审队列', '系统只读 / reduce-only 状态', '关键 worker 健康检查']}
    />
  );
}

export function AdminWithdrawalsPage() {
  const { chains } = useSystemConfig();
  const [items, setItems] = useState<AdminWithdrawReviewItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [approving, setApproving] = useState<string | null>(null);

  async function loadData(background = false) {
    if (!background) {
      setLoading(true);
    }
    setError(null);
    try {
      setItems(await api.admin.getWithdrawals());
    } catch (loadError) {
      setError(loadError);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadData();
  }, []);

  useWindowRefetch(() => {
    void loadData(true);
  }, true);

  async function handleApprove(withdrawId: string) {
    try {
      setApproving(withdrawId);
      await api.admin.approveWithdrawal(withdrawId);
      await loadData(true);
    } catch (approveError) {
      setError(approveError);
    } finally {
      setApproving(null);
    }
  }

  return (
    <div className="rg-app-page rg-app-page--admin">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro eyebrow="Admin" title="Withdraw Reviews" description="仅命中特殊风控规则的提现会进入人工复核。批准后会自动继续广播与确认链路。" titleEffect="glitch" descriptionEffect="proximity" />
        <Alert
          showIcon
          type="info"
          message="审批规则"
          description="当前仅当金额超过阈值、热钱包余额不足、链路异常或系统进入提现熔断/半熔断时，提现才会进入 RISK_REVIEW。"
        />
        <ErrorAlert error={error} />
        <Card className="table-card" title="Withdrawal Queue">
          <Table
            rowKey="withdraw_id"
            loading={loading}
            dataSource={items}
            pagination={false}
            scroll={{ x: 1200 }}
            locale={{ emptyText: <EmptyStateCard title="暂无待处理审核" description="当前没有进入 RISK_REVIEW 的提现，或你没有管理员权限。" /> }}
            columns={[
              { title: 'Time', dataIndex: 'created_at', width: 180, render: (value: string) => formatDateTime(value) },
              { title: 'User', dataIndex: 'user_address', render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text> },
              { title: 'Chain', dataIndex: 'chain_id', width: 120, render: (value: number) => formatChainName(value, chains) },
              { title: 'Asset', dataIndex: 'asset', width: 90 },
              { title: 'Amount', dataIndex: 'amount', align: 'right', render: (value: string) => formatUsd(value) },
              { title: 'Fee', dataIndex: 'fee_amount', align: 'right', render: (value: string) => formatUsd(value) },
              { title: 'To', dataIndex: 'to_address', render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text> },
              { title: 'Risk Flag', dataIndex: 'risk_flag', render: (value?: string | null) => value || '--' },
              { title: 'Status', dataIndex: 'status', width: 140, render: (value: string) => <StatusTag value={value} /> },
              {
                title: 'Action',
                width: 140,
                render: (_, record: AdminWithdrawReviewItem) => (
                  <Button
                    type="primary"
                    disabled={record.status !== 'RISK_REVIEW'}
                    loading={approving === record.withdraw_id}
                    onClick={() => void handleApprove(record.withdraw_id)}
                  >
                    批准并继续提现
                  </Button>
                ),
              },
            ]}
          />
        </Card>
      </Space>
    </div>
  );
}

export function AdminConfigsPage() {
  return (
    <AdminPageTemplate
      title="Configs"
      description="配置中心页面预留给可审计配置变更、审批与回滚。"
      items={['symbol 状态', 'risk 参数', 'withdraw 限额', '本地测试链开关']}
    />
  );
}

export function AdminLiquidationsPage() {
  return (
    <AdminPageTemplate
      title="Liquidations"
      description="强平执行、罚金、审计快照与追踪页占位。"
      items={['账户风险快照', '清算前撤单记录', '罚金分录', '重放与审计入口']}
    />
  );
}
