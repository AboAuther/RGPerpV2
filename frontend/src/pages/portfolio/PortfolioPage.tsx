import { Alert, Card, Col, Row, Space, Spin, Table, Typography } from 'antd';
import { useEffect, useState } from 'react';
import { api } from '../../shared/api';
import { ErrorAlert, MetricCard, PageIntro, StatusTag, TwoColumnRow } from '../../shared/components';
import type { AccountSummary, BalanceItem, PositionItem, RiskSnapshot } from '../../shared/domain';
import { formatAddress, formatDecimal, formatPercent, formatSignedUsd, formatUsd } from '../../shared/format';

const { Paragraph, Text } = Typography;

interface PortfolioState {
  summary: AccountSummary;
  balances: BalanceItem[];
  positions: PositionItem[];
  risk: RiskSnapshot;
}

export function PortfolioPage() {
  const [state, setState] = useState<PortfolioState | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError(null);

      try {
        const [summary, balances, positions, risk] = await Promise.all([
          api.account.getSummary(),
          api.account.getBalances(),
          api.positions.getPositions(),
          api.account.getRisk(),
        ]);

        if (active) {
          setState({ summary, balances, positions, risk });
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

  return (
    <div className="rg-app-page rg-app-page--portfolio">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow="Account"
          title="Portfolio Overview"
          description="权益、可用余额、保证金和风险提示都以后端或 review mock 返回值为准。前端不自行推导已完成的资金状态。"
        />

      {loading ? <Spin size="large" /> : null}
      <ErrorAlert error={error} />

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
                  <Text key={note}>{note}</Text>
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
                  <Text strong>关键约束</Text>
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    行情延迟、账户清算中、symbol reduce-only 时，前端只能展示并锁定高风险动作，不能自行放行。
                  </Paragraph>
                  <Text strong>前端边界</Text>
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    当前读模型不直连数据库，不持有账本或仓位真相；刷新后重新拉取接口数据。
                  </Paragraph>
                </Space>
              </Card>
            }
          />

          <Card className="table-card" title="Open Positions">
            <Table
              rowKey="position_id"
              dataSource={state.positions}
              scroll={{ x: 880 }}
              pagination={false}
              columns={[
                { title: 'Symbol', dataIndex: 'symbol', width: 120 },
                { title: 'Side', dataIndex: 'side', width: 90, render: (value: string) => <StatusTag value={value} /> },
                { title: 'Qty', dataIndex: 'qty', align: 'right', render: (value: string) => formatDecimal(value, 4) },
                { title: 'Entry', dataIndex: 'avg_entry_price', align: 'right', render: (value: string) => formatUsd(value) },
                { title: 'Mark', dataIndex: 'mark_price', align: 'right', render: (value: string) => formatUsd(value) },
                { title: 'uPnL', dataIndex: 'unrealized_pnl', align: 'right', render: (value: string) => formatSignedUsd(value) },
                {
                  title: 'Liquidation',
                  dataIndex: 'liquidation_price',
                  align: 'right',
                  render: (value: string) => formatUsd(value),
                },
                { title: 'Status', dataIndex: 'status', render: (value: string) => <StatusTag value={value} /> },
                {
                  title: 'Position ID',
                  dataIndex: 'position_id',
                  render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text>,
                },
              ]}
            />
          </Card>
        </>
      ) : null}
      </Space>
    </div>
  );
}
