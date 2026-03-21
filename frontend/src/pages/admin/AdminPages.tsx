import { Alert, Card, List, Space } from 'antd';
import { EmptyStateCard, PageIntro } from '../../shared/components';

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
  return (
    <AdminPageTemplate
      title="Withdraw Reviews"
      description="提现审核、广播与失败退款会在后续里程碑联调。"
      items={['审核原因记录', 'trace_id / event_id 查询', '广播前签名状态', '失败退款闭环']}
    />
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
