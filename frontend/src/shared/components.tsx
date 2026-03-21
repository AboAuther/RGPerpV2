import {
  AuditOutlined,
  DashboardOutlined,
  DollarOutlined,
  FundOutlined,
  HistoryOutlined,
  LockOutlined,
  RadarChartOutlined,
  SafetyCertificateOutlined,
  SwapOutlined,
  WalletOutlined,
} from '@ant-design/icons';
import { Alert, Button, Card, Col, Layout, Menu, Row, Space, Tag, Typography } from 'antd';
import Grid from 'antd/es/grid';
import type { MenuProps } from 'antd';
import type { PropsWithChildren, ReactNode } from 'react';
import { useMemo } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';
import { useAuth } from './auth';
import { appConfig } from './env';
import { formatAddress } from './format';

const { Header, Sider, Content } = Layout;
const { Title, Paragraph, Text } = Typography;

const userMenuItems: MenuProps['items'] = [
  { key: '/portfolio', icon: <DashboardOutlined />, label: <Link to="/portfolio">Portfolio</Link> },
  { key: '/trade', icon: <FundOutlined />, label: <Link to="/trade">Trade</Link> },
  { key: '/wallet/deposit', icon: <WalletOutlined />, label: <Link to="/wallet/deposit">Deposit</Link> },
  { key: '/wallet/withdraw', icon: <DollarOutlined />, label: <Link to="/wallet/withdraw">Withdraw</Link> },
  { key: '/history/orders', icon: <HistoryOutlined />, label: <Link to="/history/orders">Orders</Link> },
  { key: '/history/fills', icon: <HistoryOutlined />, label: <Link to="/history/fills">Fills</Link> },
  { key: '/history/funding', icon: <SwapOutlined />, label: <Link to="/history/funding">Funding</Link> },
  { key: '/history/transfers', icon: <SwapOutlined />, label: <Link to="/history/transfers">Transfers</Link> },
  { key: '/explorer', icon: <RadarChartOutlined />, label: <Link to="/explorer">Explorer</Link> },
];

const adminMenuItems: MenuProps['items'] = [
  { key: '/admin/dashboard', icon: <SafetyCertificateOutlined />, label: <Link to="/admin/dashboard">Admin Dashboard</Link> },
  { key: '/admin/withdrawals', icon: <DollarOutlined />, label: <Link to="/admin/withdrawals">Withdraw Reviews</Link> },
  { key: '/admin/configs', icon: <LockOutlined />, label: <Link to="/admin/configs">Configs</Link> },
  { key: '/admin/liquidations', icon: <AuditOutlined />, label: <Link to="/admin/liquidations">Liquidations</Link> },
];

const statusColorMap: Record<string, string> = {
  ACTIVE: 'success',
  SAFE: 'success',
  WATCH: 'warning',
  NO_NEW_RISK: 'gold',
  LIQUIDATING: 'error',
  TRADING: 'success',
  REDUCE_ONLY: 'volcano',
  HALTED: 'default',
  DETECTED: 'processing',
  CONFIRMING: 'processing',
  CREDITED: 'success',
  SWEEPING: 'cyan',
  REORGED: 'error',
  REQUESTED: 'processing',
  HOLD: 'gold',
  MANUAL_REVIEW: 'gold',
  SIGNING: 'processing',
  BROADCASTING: 'processing',
  COMPLETED: 'success',
  REFUNDED: 'default',
  TRIGGER_WAIT: 'gold',
  PARTIALLY_FILLED: 'processing',
  OPEN: 'success',
};

export function StatusTag({ value }: { value: string | null | undefined }) {
  const normalized = value || 'UNKNOWN';
  return <Tag color={statusColorMap[normalized] || 'default'}>{normalized}</Tag>;
}

export function PageIntro({
  eyebrow,
  title,
  description,
  extra,
}: {
  eyebrow?: string;
  title: string;
  description: string;
  extra?: ReactNode;
}) {
  return (
    <div className="page-intro">
      <div>
        {eyebrow ? <Text className="page-intro-eyebrow">{eyebrow}</Text> : null}
        <Title level={2} style={{ marginBottom: 8 }}>
          {title}
        </Title>
        <Paragraph className="page-intro-description">{description}</Paragraph>
      </div>
      {extra ? <div>{extra}</div> : null}
    </div>
  );
}

export function MetricCard({
  label,
  value,
  hint,
  accent,
}: {
  label: string;
  value: ReactNode;
  hint: ReactNode;
  accent?: 'warm' | 'cool' | 'neutral';
}) {
  return (
    <Card className={`metric-card metric-card-${accent || 'neutral'}`}>
      <Text className="metric-card-label">{label}</Text>
      <div className="metric-card-value">{value}</div>
      <Text className="metric-card-hint">{hint}</Text>
    </Card>
  );
}

export function ErrorAlert({ error }: { error: unknown }) {
  if (!error) {
    return null;
  }

  const message = error instanceof Error ? error.message : '请求失败';
  const traceId = typeof error === 'object' && error && 'traceId' in error ? String(error.traceId || '') : '';

  return (
    <Alert
      showIcon
      type="error"
      message="请求失败"
      description={
        traceId ? (
          <Space direction="vertical" size={0}>
            <Text>{message}</Text>
            <Text type="secondary">trace_id: {traceId}</Text>
          </Space>
        ) : (
          message
        )
      }
    />
  );
}

export function AppShell() {
  const { session, signOut } = useAuth();
  const location = useLocation();
  const screens = Grid.useBreakpoint();

  const selectedKeys = useMemo(() => {
    const path = location.pathname;
    const allKeys = [
      ...(userMenuItems ?? []).map((item) => String(item?.key ?? '')),
      ...(adminMenuItems ?? []).map((item) => String(item?.key ?? '')),
    ];
    return [allKeys.find((key) => path.startsWith(key)) || path];
  }, [location.pathname]);

  return (
    <Layout className="app-shell">
      <Sider
        breakpoint="lg"
        collapsedWidth={0}
        width={272}
        theme="light"
        className="app-shell-sider"
      >
        <div className="brand-block">
          <Text className="brand-badge">RGPerp</Text>
          <Title level={4} style={{ margin: 0 }}>
            Trading Console
          </Title>
          <Paragraph type="secondary" style={{ margin: 0 }}>
            钱包、账户、交易与审计视图统一入口。
          </Paragraph>
        </div>
        <Menu selectedKeys={selectedKeys} mode="inline" items={userMenuItems} />
        <div className="menu-section-title">Admin</div>
        <Menu selectedKeys={selectedKeys} mode="inline" items={adminMenuItems} />
      </Sider>
      <Layout>
        <Header className="app-shell-header">
          <Space wrap size={[12, 12]}>
            <Tag color="geekblue">{appConfig.appEnv.toUpperCase()}</Tag>
            <Tag color={appConfig.apiProvider === 'http' ? 'success' : appConfig.apiProvider === 'auto' ? 'gold' : 'cyan'}>
              API {appConfig.apiProvider.toUpperCase()}
            </Tag>
            {session ? <Tag color="default">{session.provider.toUpperCase()} session</Tag> : null}
          </Space>
          <Space size={16}>
            {!screens.md ? null : (
              <Text type="secondary">
                {session ? `${formatAddress(session.user.evm_address)} / ${session.user.status}` : '未登录'}
              </Text>
            )}
            <Button onClick={signOut}>退出</Button>
          </Space>
        </Header>
        <Content className="app-shell-content">
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}

export function TwoColumnRow({ left, right }: { left: ReactNode; right: ReactNode }) {
  return (
    <Row gutter={[20, 20]}>
      <Col xs={24} xl={16}>
        {left}
      </Col>
      <Col xs={24} xl={8}>
        {right}
      </Col>
    </Row>
  );
}

export function EmptyStateCard({
  title,
  description,
  action,
}: PropsWithChildren<{ title: string; description: string; action?: ReactNode }>) {
  return (
    <Card>
      <Title level={4}>{title}</Title>
      <Paragraph type="secondary">{description}</Paragraph>
      {action}
    </Card>
  );
}
