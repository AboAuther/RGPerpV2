import { Alert, Button, Card, Col, Row, Space, Spin, Table, Typography } from 'antd';
import { useEffect, useState } from 'react';
import { api } from '../../shared/api';
import { ErrorAlert, LoginRequiredCard, MetricCard, PageIntro, StatusTag, TwoColumnRow } from '../../shared/components';
import type { AccountSummary, BalanceItem, RiskSnapshot } from '../../shared/domain';
import { formatDecimalAdaptive, formatPercent, formatSignedUsd, formatUsd } from '../../shared/format';
import { useWindowRefetch } from '../../shared/refetch';
import { useAuth } from '../../shared/auth';

const { Paragraph, Text } = Typography;

interface PortfolioState {
  summary: AccountSummary;
  balances: BalanceItem[];
  risk: RiskSnapshot;
}

function formatRiskNote(note: string): string {
  const snapshotMatch = note.match(/权益=([0-9.]+)\s*,\s*维持保证金=([0-9.]+)\s*,\s*风险率=([0-9.]+)/);
  if (snapshotMatch) {
    const [, equity, maintenanceMargin, marginRatio] = snapshotMatch;
    return `最新风险快照：权益 ${formatUsd(equity)}，维持保证金 ${formatUsd(maintenanceMargin)}，风险率 ${formatPercent(marginRatio)}。`;
  }

  return note.replace(/\d+\.\d{6,}/g, (value) => formatDecimalAdaptive(value, 6, 2));
}

export function PortfolioPage() {
  const { session } = useAuth();
  const [state, setState] = useState<PortfolioState | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<unknown>(null);

  async function loadData(background = false) {
    if (background && state) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);

    try {
      const [summary, balances, risk] = await Promise.all([
        api.account.getSummary(),
        api.account.getBalances(),
        api.account.getRisk(),
      ]);
      setState({ summary, balances, risk });
    } catch (loadError) {
      setError(loadError);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    if (!session) {
      setState(null);
      setLoading(false);
      setRefreshing(false);
      setError(null);
      return;
    }
    void loadData();
  }, [session]);

  function refreshInBackground() {
    void (async () => {
      await loadData(true);
    })();
  }

  useWindowRefetch(refreshInBackground, !!state);

  return (
    <div className="rg-app-page rg-app-page--portfolio">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow="Account"
          title="Portfolio Overview"
          description="查看账户权益、可用余额、保证金占用、风险状态和持仓概览。"
          titleEffect="shiny"
          descriptionEffect="proximity"
          extra={
            <Button onClick={refreshInBackground} loading={refreshing}>
              刷新数据
            </Button>
          }
        />

      {loading ? <Spin size="large" /> : null}
      <ErrorAlert error={error} />

      {!session ? <LoginRequiredCard title="登录后查看账户资产" description="Portfolio 允许未登录进入浏览，但账户权益、余额、仓位和风险数据属于个人资金信息，需登录后才能拉取。" /> : null}

      {state ? (
        <>
          <Alert
            showIcon
            type={state.risk.risk_state === 'SAFE' ? 'success' : state.risk.risk_state === 'WATCH' ? 'warning' : 'error'}
            message={
              <Space wrap>
                <span>账户状态</span>
                <StatusTag value={state.risk.account_status} />
                <StatusTag value={state.risk.risk_state} />
              </Space>
            }
            description={
              <Space direction="vertical" size={2}>
                {state.risk.notes.map((note) => (
                  <Text key={note}>{formatRiskNote(note)}</Text>
                ))}
                {state.risk.mark_price_stale ? <Text>标记价格延迟，系统应禁止新增风险。</Text> : null}
              </Space>
            }
          />

          <Row gutter={[16, 16]}>
            <Col xs={24} md={12} xl={8}>
              <MetricCard label="Equity" value={formatUsd(state.summary.equity)} hint="账户权益" accent="warm" />
            </Col>
            <Col xs={24} md={12} xl={8}>
              <MetricCard
                label="Available Balance"
                value={formatUsd(state.summary.available_balance)}
                hint="可用于新增风险的余额"
                accent="cool"
              />
            </Col>
            <Col xs={24} md={12} xl={8}>
              <MetricCard
                label="Margin Ratio"
                value={formatPercent(state.summary.margin_ratio)}
                hint="风险率越高越接近预警或清算"
              />
            </Col>
            <Col xs={24} md={12} xl={8}>
              <MetricCard
                label="Initial Margin"
                value={formatUsd(state.summary.total_initial_margin)}
                hint="当前占用初始保证金"
              />
            </Col>
            <Col xs={24} md={12} xl={8}>
              <MetricCard
                label="Maintenance Margin"
                value={formatUsd(state.summary.total_maintenance_margin)}
                hint="维持保证金要求"
              />
            </Col>
            <Col xs={24} md={12} xl={8}>
              <MetricCard
                label="Unrealized PnL"
                value={formatSignedUsd(state.summary.unrealized_pnl)}
                hint="未实现盈亏"
              />
            </Col>
          </Row>

          <TwoColumnRow
            left={
              <Card className="table-card" title="Balances">
                <Table
                  rowKey={(record) => `${record.account_code}-${record.asset}`}
                  pagination={false}
                  dataSource={state.balances}
                  columns={[
                    { title: 'Account', dataIndex: 'account_code' },
                    { title: 'Asset', dataIndex: 'asset', width: 90 },
                    {
                      title: 'Balance',
                      dataIndex: 'balance',
                      align: 'right',
                      render: (value: string) => formatUsd(value),
                    },
                  ]}
                />
              </Card>
            }
            right={
              <Card className="table-card" title="Risk Notes">
                <Space direction="vertical" size={10}>
                  <Text strong>风险提示</Text>
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    当风险率升高、行情延迟或交易状态受限时，请优先关注保证金和仓位变化。
                  </Paragraph>
                  <Text strong>建议操作</Text>
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    如需降低风险，可减少仓位、补充保证金，或等待行情恢复稳定后再操作。
                  </Paragraph>
                </Space>
              </Card>
            }
          />
        </>
      ) : null}
      </Space>
    </div>
  );
}
