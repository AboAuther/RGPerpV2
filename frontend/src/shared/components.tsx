import {
  AppstoreOutlined,
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
import type { MenuProps } from 'antd';
import type { PropsWithChildren, ReactNode } from 'react';
import { useMemo } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import GlitchText from '../components/landing/GlitchText';
import ShinyText from '../components/landing/ShinyText';
import VariableProximityText from '../components/landing/VariableProximityText';
import { hasAdminAccess, useAuth } from './auth';
import { appConfig } from './env';
import { ApiError } from './api';
import { formatAddress } from './format';

const { Header, Content, Footer } = Layout;
const { Title, Paragraph, Text } = Typography;

type IntroTextEffect = 'none' | 'shiny' | 'glitch' | 'proximity';

const baseNavMenuItems: NonNullable<MenuProps['items']> = [
  { key: '/trade', icon: <FundOutlined />, label: 'Trade' },
  { key: '/portfolio', icon: <DashboardOutlined />, label: 'Portfolio' },
  {
    key: '/wallet',
    icon: <WalletOutlined />,
    label: 'Wallet',
    children: [
      { key: '/wallet/deposit', icon: <WalletOutlined />, label: 'Deposit' },
      { key: '/wallet/transfer', icon: <SwapOutlined />, label: 'Transfer' },
      { key: '/wallet/withdraw', icon: <DollarOutlined />, label: 'Withdraw' },
    ],
  },
  {
    key: '/history',
    icon: <HistoryOutlined />,
    label: 'History',
    children: [
      { key: '/history/orders', label: 'Orders' },
      { key: '/history/fills', label: 'Fills' },
      { key: '/history/positions', label: 'Positions' },
      { key: '/history/funding', label: 'Funding' },
      { key: '/history/transfers', label: 'Transfers' },
    ],
  },
  { key: '/explorer', icon: <RadarChartOutlined />, label: 'Explorer' },
];

const adminNavItem: NonNullable<MenuProps['items']>[number] = {
  key: '/admin',
  icon: <SafetyCertificateOutlined />,
  label: 'Admin',
  children: [
    { key: '/admin/dashboard', icon: <AppstoreOutlined />, label: 'Dashboard' },
    { key: '/admin/withdrawals', icon: <DollarOutlined />, label: 'Withdrawals' },
    { key: '/admin/configs', icon: <LockOutlined />, label: 'Configs' },
    { key: '/admin/liquidations', icon: <AuditOutlined />, label: 'Liquidations' },
  ],
};

const statusColorMap: Record<string, string> = {
  ACTIVE: 'success',
  PASS: 'success',
  FAIL: 'error',
  WARN: 'warning',
  BUY: 'cyan',
  SELL: 'volcano',
  LONG: 'cyan',
  SHORT: 'volcano',
  PAY: 'gold',
  RECEIVE: 'success',
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
  RISK_REVIEW: 'gold',
  APPROVED: 'cyan',
  SIGNING: 'processing',
  BROADCASTED: 'processing',
  REJECTED: 'error',
  CANCELED: 'default',
  FAILED: 'error',
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
  eyebrowEffect = 'shiny',
  titleEffect = 'none',
  descriptionEffect = 'none',
}: {
  eyebrow?: string;
  title: string;
  description: string;
  extra?: ReactNode;
  eyebrowEffect?: IntroTextEffect;
  titleEffect?: IntroTextEffect;
  descriptionEffect?: IntroTextEffect;
}) {
  const eyebrowNode = renderAnimatedCopy(eyebrow, eyebrowEffect, 'page-intro-chip');
  const titleNode = renderAnimatedCopy(title, titleEffect, 'page-intro-title-text');
  const descriptionNode = renderAnimatedCopy(description, descriptionEffect, 'page-intro-description-text');

  return (
    <div className="page-intro">
      <div>
        {eyebrow ? <Text className="page-intro-eyebrow">{eyebrowNode}</Text> : null}
        <Title level={2} className={`page-intro-title ${titleEffect !== 'none' ? `page-intro-title--${titleEffect}` : ''}`}>
          {titleNode}
        </Title>
        <Paragraph className={`page-intro-description ${descriptionEffect !== 'none' ? `page-intro-description--${descriptionEffect}` : ''}`}>
          {descriptionNode}
        </Paragraph>
      </div>
      {extra ? <div>{extra}</div> : null}
    </div>
  );
}

function renderAnimatedCopy(text: string | undefined, effect: IntroTextEffect, className: string) {
  if (!text) {
    return null;
  }

  if (effect === 'shiny') {
    return <ShinyText text={text} className={className} />;
  }

  if (effect === 'glitch') {
    return <GlitchText text={text} className={className} />;
  }

  if (effect === 'proximity') {
    return <VariableProximityText text={text} className={className} />;
  }

  return <span className={className}>{text}</span>;
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

  const message = normalizeErrorAlertMessage(error instanceof Error ? error.message : '请求失败');
  const traceId = typeof error === 'object' && error && 'traceId' in error ? String(error.traceId || '') : '';
  const title = deriveErrorAlertTitle(error, message);

  return (
    <Alert
      showIcon
      type="error"
      message={title}
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

function normalizeErrorAlertMessage(raw: string): string {
  const normalized = raw
    .replace(/^invalid argument:\s*/i, '')
    .replace(/^conflict:\s*/i, '')
    .replace(/^forbidden:\s*/i, '')
    .replace(/^not found:\s*/i, '')
    .trim();

  return normalized || '请求失败';
}

function deriveErrorAlertTitle(error: unknown, message: string): string {
  if (message.includes('未注册')) {
    return '无法发起内部转账';
  }
  if (error instanceof ApiError) {
    switch (error.status) {
      case 400:
        return '提交信息有误';
      case 401:
        return '登录状态失效';
      case 403:
        return '当前操作不可用';
      case 404:
        return '未找到相关数据';
      case 409:
        return '请求冲突';
      default:
        break;
    }
  }
  return '请求失败';
}

export function AppShell() {
  const { session, signOut } = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const canAccessAdmin = hasAdminAccess(session?.user);

  const selectedKeys = useMemo(() => {
    const path = location.pathname;
    if (path.startsWith('/wallet/')) {
      return [path];
    }
    if (path.startsWith('/history/')) {
      return [path];
    }
    if (path.startsWith('/admin/')) {
      return [path];
    }
    return [path];
  }, [location.pathname]);

  const navMenuItems = useMemo<NonNullable<MenuProps['items']>>(() => {
    return canAccessAdmin ? [...baseNavMenuItems, adminNavItem] : baseNavMenuItems;
  }, [canAccessAdmin]);

  return (
    <Layout className="app-shell">
      <Header className="app-shell-header rg-glass-card">
        <button type="button" className="brand-block" onClick={() => navigate('/')} aria-label="前往首页">
          <BrandLogo size={34} />
          <div>
            <Title level={4} style={{ margin: 0 }}>
              RGPerp
            </Title>
            <Paragraph type="secondary" style={{ margin: 0 }}>
              Production console
            </Paragraph>
          </div>
        </button>
        <Menu
          mode="horizontal"
          selectedKeys={selectedKeys}
          items={navMenuItems}
          className="app-shell-nav"
          onClick={({ key }) => navigate(key)}
        />
        <Space size={12} className="app-shell-actions">
          <Space wrap size={[8, 8]}>
            <Tag color="geekblue">{appConfig.appEnv.toUpperCase()}</Tag>
            <Tag color="success">API HTTP</Tag>
          </Space>
          <Text type="secondary" className="app-shell-identity">
            {session ? `${formatAddress(session.user.evm_address)} / ${session.user.status}` : '未登录'}
          </Text>
          {session ? <Button onClick={signOut}>退出</Button> : <Button type="primary" onClick={() => navigate('/login')}>登录</Button>}
        </Space>
      </Header>
      <Layout>
        <Content className="app-shell-content">
          <Outlet />
        </Content>
        <Footer className="app-shell-footer">RGPerp console</Footer>
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
    <Card className="surface-card">
      <Title level={4}>{title}</Title>
      <Paragraph type="secondary">{description}</Paragraph>
      {action}
    </Card>
  );
}

export function LoginRequiredCard({
  title = '请先登录',
  description = '登录后即可查看您的账户数据并使用相关功能。',
  action,
}: {
  title?: string;
  description?: string;
  action?: ReactNode;
}) {
  return <EmptyStateCard title={title} description={description} action={action} />;
}

export function BrandLogo({ size = 24 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <rect x="1.5" y="1.5" width="21" height="21" rx="7" fill="url(#rgperp-bg)" />
      <path
        d="M7 17V7H11.8C14.15 7 15.6 8.22 15.6 10.28C15.6 11.84 14.7 12.88 13.21 13.18L16.5 17H13.84L10.84 13.45H9.3V17H7ZM9.3 11.48H11.57C12.84 11.48 13.45 11.08 13.45 10.23C13.45 9.39 12.84 8.98 11.57 8.98H9.3V11.48Z"
        fill="white"
      />
      <path d="M17.4 7.4L19.9 9.9L15.25 14.55L13.95 13.25L17.4 9.8L16.1 8.5L17.4 7.4Z" fill="#7CFFF1" />
      <defs>
        <linearGradient id="rgperp-bg" x1="3" y1="3" x2="21" y2="21" gradientUnits="userSpaceOnUse">
          <stop stopColor="#20C9B5" />
          <stop offset="0.52" stopColor="#1A7F89" />
          <stop offset="1" stopColor="#103548" />
        </linearGradient>
      </defs>
    </svg>
  );
}
