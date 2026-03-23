import { Alert, Button, Card, Input, InputNumber, List, Select, Space, Spin, Switch, Table, Tooltip, Typography, message } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { ApiError, api } from '../../shared/api';
import { EmptyStateCard, ErrorAlert, MetricCard, PageIntro, StatusTag } from '../../shared/components';
import type { AdminWithdrawReviewItem, LedgerAuditReport, LedgerAssetOverview, LedgerChainBalance, LedgerOverview, RiskMonitorDashboard, RuntimeConfigHistoryItem, RuntimeConfigPatchRequest, RuntimeConfigSnapshotView, RuntimeConfigView, SymbolNetExposureItem } from '../../shared/domain';
import { formatAddress, formatChainName, formatDateTime, formatDecimal, formatPercent, formatUsd } from '../../shared/format';
import { useWindowRefetch } from '../../shared/refetch';
import { useSystemConfig } from '../../shared/system';

const { Paragraph, Text } = Typography;

const ledgerBreakdownColumns = [
  { key: 'user_wallet', title: '用户钱包', fullTitle: 'USER_WALLET' },
  { key: 'user_order_margin', title: '挂单保证金', fullTitle: 'USER_ORDER_MARGIN' },
  { key: 'user_position_margin', title: '持仓保证金', fullTitle: 'USER_POSITION_MARGIN' },
  { key: 'user_withdraw_hold', title: '提现冻结', fullTitle: 'USER_WITHDRAW_HOLD' },
  { key: 'system_pool', title: '系统池', fullTitle: 'SYSTEM_POOL' },
  { key: 'trading_fee_account', title: '交易手续费', fullTitle: 'TRADING_FEE_ACCOUNT' },
  { key: 'withdraw_fee_account', title: '提现手续费', fullTitle: 'WITHDRAW_FEE_ACCOUNT' },
  { key: 'penalty_account', title: '罚金账户', fullTitle: 'PENALTY_ACCOUNT' },
  { key: 'funding_pool', title: '资金费池', fullTitle: 'FUNDING_POOL' },
  { key: 'insurance_fund', title: '保险基金', fullTitle: 'INSURANCE_FUND' },
  { key: 'rounding_diff_account', title: '舍入差额', fullTitle: 'ROUNDING_DIFF_ACCOUNT' },
  { key: 'deposit_pending_confirm', title: '充值待确认', fullTitle: 'DEPOSIT_PENDING_CONFIRM' },
  { key: 'withdraw_in_transit', title: '提现在途', fullTitle: 'WITHDRAW_IN_TRANSIT' },
  { key: 'sweep_in_transit', title: '归集在途', fullTitle: 'SWEEP_IN_TRANSIT' },
  { key: 'custody_hot', title: '热钱包镜像', fullTitle: 'CUSTODY_HOT' },
  { key: 'custody_warm', title: '温钱包镜像', fullTitle: 'CUSTODY_WARM' },
  { key: 'custody_cold', title: '冷钱包镜像', fullTitle: 'CUSTODY_COLD' },
  { key: 'test_faucet_pool', title: '测试水龙头', fullTitle: 'TEST_FAUCET_POOL' },
] as const;

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
        <Card className="surface-card">
          <List
            dataSource={items}
            renderItem={(item) => <List.Item>{item}</List.Item>}
          />
        </Card>
        <EmptyStateCard
          title="暂无数据"
          description="当前暂无可展示内容。"
        />
      </Space>
    </div>
  );
}

type EditableRuntimeConfig = RuntimeConfigSnapshotView & {
  reason: string;
};

function toEditableRuntimeConfig(snapshot: RuntimeConfigSnapshotView): EditableRuntimeConfig {
  return {
    ...snapshot,
    reason: '',
  };
}

export function AdminDashboardPage() {
  const [selectedAsset, setSelectedAsset] = useState<string>('ALL');
  const [showAllRiskRows, setShowAllRiskRows] = useState(false);
  const [overview, setOverview] = useState<LedgerOverview | null>(null);
  const [audit, setAudit] = useState<LedgerAuditReport | null>(null);
  const [riskDashboard, setRiskDashboard] = useState<RiskMonitorDashboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [runningAudit, setRunningAudit] = useState(false);
  const [exporting, setExporting] = useState<'json' | 'csv' | null>(null);
  const [error, setError] = useState<unknown>(null);

  async function loadData(asset: string, background = false) {
    if (!background) {
      setLoading(true);
    }
    setError(null);
    try {
      const scopeAsset = asset === 'ALL' ? undefined : asset;
      const [overviewData, riskData, latestAudit] = await Promise.all([
        api.admin.getLedgerOverview(scopeAsset),
        api.admin.getRiskMonitorDashboard(),
        api.admin.getLatestLedgerAudit(scopeAsset).catch((loadError) => {
          const text = loadError instanceof Error ? loadError.message.toLowerCase() : '';
          if ((loadError instanceof ApiError && loadError.status === 404) || text.includes('not found')) {
            return null;
          }
          throw loadError;
        }),
      ]);
      setOverview(overviewData);
      setRiskDashboard(riskData);
      setAudit(latestAudit);
    } catch (loadError) {
      setError(loadError);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadData(selectedAsset);
  }, [selectedAsset]);

  useWindowRefetch(() => {
    void loadData(selectedAsset, true);
  }, true);

  async function handleRunAudit() {
    try {
      setRunningAudit(true);
      const scopeAsset = selectedAsset === 'ALL' ? undefined : selectedAsset;
      const report = await api.admin.runLedgerAudit(scopeAsset);
      setAudit(report);
      await loadData(selectedAsset, true);
      message.success('账本审计已完成');
    } catch (runError) {
      setError(runError);
      message.error(runError instanceof Error ? runError.message : '账本审计失败');
    } finally {
      setRunningAudit(false);
    }
  }

  async function handleExport(format: 'json' | 'csv') {
    if (!audit) {
      message.warning('暂无可导出的审计报告');
      return;
    }
    try {
      setExporting(format);
      const scopeAsset = selectedAsset === 'ALL' ? undefined : selectedAsset;
      const blob = await api.admin.downloadLatestLedgerAudit(format, scopeAsset);
      const url = window.URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `ledger-audit-${(audit.scope_asset || selectedAsset).toLowerCase()}-${audit.audit_report_id}.${format}`;
      document.body.append(anchor);
      anchor.click();
      anchor.remove();
      window.URL.revokeObjectURL(url);
      message.success(`审计报告已导出为 ${format.toUpperCase()}`);
    } catch (downloadError) {
      setError(downloadError);
      message.error(downloadError instanceof Error ? downloadError.message : '导出审计报告失败');
    } finally {
      setExporting(null);
    }
  }

  const assetRows = overview?.assets || [];
  const riskRows = riskDashboard?.items || [];
  const visibleRiskRows = showAllRiskRows ? riskRows : riskRows.slice(0, 8);
  const failedChecks = audit?.checks.filter((item) => item.status === 'FAIL').length || 0;
  const blockedSymbols = riskRows.filter((item) => item.blocked_open_side).length;
  const assetOptions = useMemo(() => {
    const values = new Set<string>(['ALL']);
    for (const item of overview?.assets || []) {
      values.add(item.asset);
    }
    for (const item of audit?.overview || []) {
      values.add(item.asset);
    }
    if (selectedAsset) {
      values.add(selectedAsset);
    }
    return Array.from(values.values()).map((value) => ({
      label: value === 'ALL' ? '全部资产' : value,
      value,
    }));
  }, [audit?.overview, overview?.assets, selectedAsset]);

  return (
    <div className="rg-app-page rg-app-page--admin">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow="Admin"
          title="Ledger Dashboard"
          description="集中查看用户负债、平台收入、在途资金与账本审计结果。"
          titleEffect="glitch"
          descriptionEffect="proximity"
          extra={
            <Space wrap>
              <Select
                value={selectedAsset}
                style={{ minWidth: 180 }}
                options={assetOptions}
                onChange={setSelectedAsset}
              />
              <Button onClick={() => void handleExport('json')} loading={exporting === 'json'} disabled={!audit}>
                导出 JSON
              </Button>
              <Button onClick={() => void handleExport('csv')} loading={exporting === 'csv'} disabled={!audit}>
                导出 CSV
              </Button>
              <Button type="primary" loading={runningAudit} onClick={() => void handleRunAudit()}>
                运行一键审计
              </Button>
            </Space>
          }
        />
        <Alert
          showIcon
          type="info"
          message="账本口径"
          description="USER_MARGIN 为 USER_ORDER_MARGIN 与 USER_POSITION_MARGIN 的展示汇总；审计会校验快照守恒、ledger_tx 平衡、业务映射、outbox 完整性，以及链上 vault 余额与 custody mirror 总账镜像是否一致。"
        />
        {loading ? <Spin size="large" /> : null}
        <ErrorAlert error={error} />

        <Space wrap size={[16, 16]}>
          <MetricCard label="最新审计" value={<StatusTag value={audit?.status || 'PENDING'} />} hint={audit ? `报告 ${audit.audit_report_id}` : '尚未执行'} accent="cool" />
          <MetricCard label="已检查资产" value={String(audit?.overview.length || assetRows.length)} hint="当前账本资产维度" accent="neutral" />
          <MetricCard label="失败检查项" value={String(failedChecks)} hint={audit ? `最近执行于 ${formatDateTime(audit.finished_at)}` : '暂无审计记录'} accent={failedChecks > 0 ? 'warm' : 'cool'} />
          <MetricCard label="受限交易对" value={String(blockedSymbols)} hint={riskDashboard ? `风险看板更新于 ${formatDateTime(riskDashboard.generated_at)}` : '暂无风险看板'} accent={blockedSymbols > 0 ? 'warm' : 'cool'} />
          <MetricCard label="概览更新时间" value={formatDateTime(overview?.generated_at)} hint="账本快照读取时间" accent="neutral" />
        </Space>

        {riskDashboard ? (
          <Card
            className="table-card"
            title="净敞口监控"
            extra={
              riskRows.length > 8 ? (
                <Button type="text" onClick={() => setShowAllRiskRows((value) => !value)}>
                  {showAllRiskRows ? `收起 · 已显示 ${visibleRiskRows.length}/${riskRows.length}` : `展开全部 · 已显示 ${visibleRiskRows.length}/${riskRows.length}`}
                </Button>
              ) : null
            }
          >
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                当前单品种净敞口硬上限为 {formatUsd(riskDashboard.hard_limit_notional)}，最大动态滑点调节为 {riskDashboard.max_dynamic_slippage_bps} bps。
              </Paragraph>
              <Table
                rowKey="symbol"
                dataSource={visibleRiskRows}
                pagination={false}
                scroll={{ x: 1320 }}
                locale={{ emptyText: <EmptyStateCard title="暂无净敞口数据" description="当前没有可监控的交易对或未生成标记价格。" /> }}
                columns={[
                  { title: '交易对', dataIndex: 'symbol', width: 140 },
                  { title: '状态', dataIndex: 'status', width: 120, render: (value: string) => <StatusTag value={value} /> },
                  { title: '标记价格', dataIndex: 'mark_price', align: 'right', render: (value: string) => formatUsd(value, 8) },
                  { title: '多头数量', dataIndex: 'long_qty', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                  { title: '空头数量', dataIndex: 'short_qty', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                  { title: '净数量', dataIndex: 'net_qty', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                  { title: '净名义价值', dataIndex: 'net_notional', align: 'right', render: (value: string) => formatUsd(value, 8) },
                  { title: '占用率', dataIndex: 'utilization_ratio', align: 'right', render: (value: string) => formatPercent(value, 1) },
                  {
                    title: '限制方向',
                    dataIndex: 'blocked_open_side',
                    width: 140,
                    render: (value?: SymbolNetExposureItem['blocked_open_side']) => (value ? <StatusTag value={value} /> : '--'),
                  },
                  {
                    title: '买入调节',
                    dataIndex: 'buy_adjustment_bps',
                    align: 'right',
                    render: (value: number) => formatBps(value),
                  },
                  {
                    title: '卖出调节',
                    dataIndex: 'sell_adjustment_bps',
                    align: 'right',
                    render: (value: number) => formatBps(value),
                  },
                ]}
              />
            </Space>
          </Card>
        ) : null}

        {overview ? (
          <>
            <Card className="table-card" title={`按资产汇总账本${overview.scope_asset !== 'ALL' ? ` · ${overview.scope_asset}` : ''}`}>
              <Table
                rowKey="asset"
                dataSource={assetRows}
                pagination={false}
                scroll={{ x: 1280 }}
                columns={[
                  { title: '资产', dataIndex: 'asset', width: 100 },
                  { title: '用户负债', dataIndex: 'user_liability', align: 'right', render: (value: string, record: LedgerAssetOverview) => formatLedgerValue(record.asset, value) },
                  { title: '用户钱包', dataIndex: 'user_wallet', align: 'right', render: (value: string, record: LedgerAssetOverview) => formatLedgerValue(record.asset, value) },
                  { title: '用户保证金', dataIndex: 'user_margin', align: 'right', render: (value: string, record: LedgerAssetOverview) => formatLedgerValue(record.asset, value) },
                  { title: '提现冻结', dataIndex: 'user_withdraw_hold', align: 'right', render: (value: string, record: LedgerAssetOverview) => formatLedgerValue(record.asset, value) },
                  { title: '系统池', dataIndex: 'system_pool', align: 'right', render: (value: string, record: LedgerAssetOverview) => formatLedgerValue(record.asset, value) },
                  { title: '平台收入', dataIndex: 'platform_revenue', align: 'right', render: (value: string, record: LedgerAssetOverview) => formatLedgerValue(record.asset, value) },
                  { title: '风险缓冲', dataIndex: 'risk_buffer', align: 'right', render: (value: string, record: LedgerAssetOverview) => formatLedgerValue(record.asset, value) },
                  { title: '在途资金', dataIndex: 'in_flight', align: 'right', render: (value: string, record: LedgerAssetOverview) => formatLedgerValue(record.asset, value) },
                  { title: '托管镜像', dataIndex: 'custody_mirror', align: 'right', render: (value: string, record: LedgerAssetOverview) => formatLedgerValue(record.asset, value) },
                  {
                    title: '净余额',
                    dataIndex: 'net_balance',
                    align: 'right',
                    render: (value: string, record: LedgerAssetOverview) => (
                      <Space size={8}>
                        <StatusTag value={value === '0' ? 'PASS' : 'FAIL'} />
                        <Text>{formatLedgerValue(record.asset, value)}</Text>
                      </Space>
                    ),
                  },
                ]}
              />
            </Card>

            <Card className="table-card" title={`账本账户拆分${overview.scope_asset !== 'ALL' ? ` · ${overview.scope_asset}` : ''}`}>
              <Table
                className="admin-ledger-breakdown-table"
                rowKey="asset"
                dataSource={assetRows}
                pagination={false}
                scroll={{ x: 2280 }}
                columns={[
                  { title: 'Asset', dataIndex: 'asset', fixed: 'left', width: 100 },
                  ...ledgerBreakdownColumns.map((column) => ({
                    title: (
                      <Tooltip title={column.fullTitle}>
                        <span>{column.title}</span>
                      </Tooltip>
                    ),
                    dataIndex: column.key,
                    align: 'right' as const,
                    width: 126,
                    render: (value: string, record: LedgerAssetOverview) => (
                      <Text className="admin-ledger-breakdown-table__value">{formatLedgerValue(record.asset, value)}</Text>
                    ),
                  })),
                ]}
              />
            </Card>
          </>
        ) : null}

        {audit ? (
          <Card className="table-card" title="Latest Audit Report">
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                最近一次审计由 {audit.executed_by} 于 {formatDateTime(audit.finished_at)} 执行，范围 {audit.scope_asset}。
              </Paragraph>
              {audit.chain_balances && audit.chain_balances.length > 0 ? (
                <Table
                  rowKey={(record: LedgerChainBalance) => `${record.row_type}-${record.asset}-${record.chain_id}-${record.chain_key}`}
                  dataSource={audit.chain_balances}
                  pagination={false}
                  scroll={{ x: 960 }}
                  columns={[
                    { title: 'Row', dataIndex: 'row_type', width: 100, render: (value: string) => (value === 'TOTAL' ? 'Total' : 'Chain') },
                    { title: 'Asset', dataIndex: 'asset', width: 100 },
                    { title: 'Chain', render: (_, record: LedgerChainBalance) => (record.row_type === 'TOTAL' ? 'All Enabled Chains' : `${record.chain_name} (${record.chain_id})`) },
                    { title: 'Vault', dataIndex: 'vault_address', render: (value: string) => value || '--' },
                    { title: 'On-chain', dataIndex: 'onchain_balance', align: 'right', render: (value: string, record: LedgerChainBalance) => formatLedgerValue(record.asset, value) },
                    { title: 'Custody Mirror', dataIndex: 'custody_mirror', align: 'right', render: (value: string, record: LedgerChainBalance) => (value ? formatLedgerValue(record.asset, value) : '--') },
                    { title: 'Delta', dataIndex: 'delta', align: 'right', render: (value: string, record: LedgerChainBalance) => (value ? formatLedgerValue(record.asset, value) : '--') },
                    { title: 'Status', dataIndex: 'status', width: 120, render: (value: string) => <StatusTag value={value} /> },
                  ]}
                />
              ) : null}
              <Table
                rowKey="check_key"
                dataSource={audit.checks}
                pagination={false}
                scroll={{ x: 960 }}
                columns={[
                  { title: 'Check', dataIndex: 'label', width: 220 },
                  { title: 'Status', dataIndex: 'status', width: 120, render: (value: string) => <StatusTag value={value} /> },
                  { title: 'Value', dataIndex: 'value', width: 120 },
                  { title: 'Summary', dataIndex: 'summary' },
                  {
                    title: 'Samples',
                    dataIndex: 'sample_refs',
                    render: (value?: string[]) =>
                      value && value.length > 0 ? (
                        <Space direction="vertical" size={0}>
                          {value.map((item) => (
                            <Text key={item} type="secondary">
                              {item}
                            </Text>
                          ))}
                        </Space>
                      ) : (
                        '--'
                      ),
                  },
                ]}
              />
            </Space>
          </Card>
        ) : (
          <EmptyStateCard title="暂无审计报告" description="点击上方按钮即可执行一键审计并生成首份报告。" />
        )}
      </Space>
    </div>
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
        <PageIntro eyebrow="Admin" title="Withdraw Reviews" description="仅命中特殊风控规则的提现会进入人工复核。审核通过后会继续处理。" titleEffect="glitch" descriptionEffect="proximity" />
        <Alert
          showIcon
          type="info"
          message="审批规则"
          description="命中风控条件的提现将进入人工复核，审核通过后继续处理。"
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
  const [configView, setConfigView] = useState<RuntimeConfigView | null>(null);
  const [form, setForm] = useState<EditableRuntimeConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<unknown>(null);

  async function loadData(background = false) {
    if (!background) {
      setLoading(true);
    }
    setError(null);
    try {
      const view = await api.admin.getRuntimeConfig();
      setConfigView(view);
      setForm((current) => {
        if (!current || !background) {
          return toEditableRuntimeConfig(view.snapshot);
        }
        return current;
      });
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

  async function handleSave() {
    if (!form) {
      return;
    }
    if (!form.reason.trim()) {
      message.warning('请填写变更原因');
      return;
    }
    const payload: RuntimeConfigPatchRequest = {
      reason: form.reason.trim(),
      global: {
        read_only: form.read_only,
        reduce_only: form.reduce_only,
        trace_header_required: form.trace_header_required,
      },
      risk: {
        global_buffer_ratio: form.risk_global_buffer_ratio,
        mark_price_stale_sec: form.risk_mark_price_stale_sec,
        force_reduce_only_on_stale_price: form.risk_force_reduce_only_on_stale_price,
        liquidation_penalty_rate: form.risk_liquidation_penalty_rate,
        liquidation_extra_slippage_bps: form.risk_liquidation_extra_slippage_bps,
        max_open_orders_per_user_per_symbol: form.risk_max_open_orders_per_user_per_symbol,
        net_exposure_hard_limit: form.risk_net_exposure_hard_limit,
        max_exposure_slippage_bps: form.risk_max_exposure_slippage_bps,
      },
      hedge: {
        enabled: form.hedge_enabled,
        soft_threshold_ratio: form.hedge_soft_threshold_ratio,
        hard_threshold_ratio: form.hedge_hard_threshold_ratio,
      },
    };
    try {
      setSaving(true);
      const view = await api.admin.updateRuntimeConfig(payload);
      setConfigView(view);
      setForm(toEditableRuntimeConfig(view.snapshot));
      message.success('运行时配置已更新');
    } catch (saveError) {
      setError(saveError);
      message.error(saveError instanceof Error ? saveError.message : '更新运行时配置失败');
    } finally {
      setSaving(false);
    }
  }

  const snapshot = form;
  const history = configView?.history || [];

  return (
    <div className="rg-app-page rg-app-page--admin">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro eyebrow="Admin" title="Configs" description="管理 kill switch 与关键风险参数。变更会写入 config_items 与审计日志，并向各进程热更新。" titleEffect="glitch" descriptionEffect="proximity" />
        <Alert
          showIcon
          type="warning"
          message="变更约束"
          description="本页仅操作阶段四关键运行时参数。变更会版本化、审计化，并在下个轮询周期同步到 api-server、market-data、funding-worker。"
        />
        {loading ? <Spin size="large" /> : null}
        <ErrorAlert error={error} />

        {snapshot ? (
          <>
            <Space wrap size={[16, 16]}>
              <MetricCard label="System Mode" value={snapshot.system_mode} hint="当前运行环境模式" accent="neutral" />
              <MetricCard label="Read Only" value={<StatusTag value={snapshot.read_only ? 'ON' : 'OFF'} />} hint="全站写入 kill switch" accent={snapshot.read_only ? 'warm' : 'cool'} />
              <MetricCard label="Reduce Only" value={<StatusTag value={snapshot.reduce_only ? 'ON' : 'OFF'} />} hint="全站只减仓 kill switch" accent={snapshot.reduce_only ? 'warm' : 'cool'} />
              <MetricCard label="Last Loaded" value={formatDateTime(configView?.generated_at)} hint="Admin 读取时间" accent="neutral" />
            </Space>

            <Card className="surface-card" title="Global Switches">
              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Space size={12}>
                  <Text style={{ minWidth: 180 }}>Read Only</Text>
                  <Switch checked={snapshot.read_only} onChange={(checked) => setForm((current) => (current ? { ...current, read_only: checked } : current))} />
                </Space>
                <Space size={12}>
                  <Text style={{ minWidth: 180 }}>Reduce Only</Text>
                  <Switch checked={snapshot.reduce_only} onChange={(checked) => setForm((current) => (current ? { ...current, reduce_only: checked } : current))} />
                </Space>
                <Space size={12}>
                  <Text style={{ minWidth: 180 }}>Trace Header Required</Text>
                  <Switch checked={snapshot.trace_header_required} onChange={(checked) => setForm((current) => (current ? { ...current, trace_header_required: checked } : current))} />
                </Space>
              </Space>
            </Card>

            <Card className="surface-card" title="Risk Parameters">
              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Space wrap size={16}>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">Risk Buffer Ratio</Text>
                    <Input value={snapshot.risk_global_buffer_ratio} onChange={(event) => setForm((current) => (current ? { ...current, risk_global_buffer_ratio: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">Mark Price Stale Sec</Text>
                    <InputNumber min={1} value={snapshot.risk_mark_price_stale_sec} onChange={(value) => setForm((current) => (current ? { ...current, risk_mark_price_stale_sec: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">Liquidation Penalty Rate</Text>
                    <Input value={snapshot.risk_liquidation_penalty_rate} onChange={(event) => setForm((current) => (current ? { ...current, risk_liquidation_penalty_rate: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">Liquidation Extra Slippage Bps</Text>
                    <InputNumber min={0} value={snapshot.risk_liquidation_extra_slippage_bps} onChange={(value) => setForm((current) => (current ? { ...current, risk_liquidation_extra_slippage_bps: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                </Space>
                <Space wrap size={16}>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">Max Open Orders / User / Symbol</Text>
                    <InputNumber min={1} value={snapshot.risk_max_open_orders_per_user_per_symbol} onChange={(value) => setForm((current) => (current ? { ...current, risk_max_open_orders_per_user_per_symbol: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">Net Exposure Hard Limit</Text>
                    <Input value={snapshot.risk_net_exposure_hard_limit} onChange={(event) => setForm((current) => (current ? { ...current, risk_net_exposure_hard_limit: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">Max Exposure Slippage Bps</Text>
                    <InputNumber min={0} value={snapshot.risk_max_exposure_slippage_bps} onChange={(value) => setForm((current) => (current ? { ...current, risk_max_exposure_slippage_bps: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space size={12}>
                    <Text style={{ minWidth: 180 }}>Force Reduce Only On Stale Price</Text>
                    <Switch checked={snapshot.risk_force_reduce_only_on_stale_price} onChange={(checked) => setForm((current) => (current ? { ...current, risk_force_reduce_only_on_stale_price: checked } : current))} />
                  </Space>
                </Space>
              </Space>
            </Card>

            <Card className="surface-card" title="Hedge Thresholds">
              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Space size={12}>
                  <Text style={{ minWidth: 180 }}>Hedge Enabled</Text>
                  <Switch checked={snapshot.hedge_enabled} onChange={(checked) => setForm((current) => (current ? { ...current, hedge_enabled: checked } : current))} />
                </Space>
                <Space wrap size={16}>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">Soft Threshold Ratio</Text>
                    <Input value={snapshot.hedge_soft_threshold_ratio} onChange={(event) => setForm((current) => (current ? { ...current, hedge_soft_threshold_ratio: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">Hard Threshold Ratio</Text>
                    <Input value={snapshot.hedge_hard_threshold_ratio} onChange={(event) => setForm((current) => (current ? { ...current, hedge_hard_threshold_ratio: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                </Space>
              </Space>
            </Card>

            <Card className="surface-card" title="Change Submission">
              <Space direction="vertical" size={12} style={{ width: '100%' }}>
                <Input.TextArea
                  rows={3}
                  placeholder="填写本次变更原因，便于审计和回滚。"
                  value={snapshot.reason}
                  onChange={(event) => setForm((current) => (current ? { ...current, reason: event.target.value } : current))}
                />
                <Space>
                  <Button type="primary" loading={saving} onClick={() => void handleSave()}>
                    提交配置变更
                  </Button>
                  <Button onClick={() => setForm(configView ? toEditableRuntimeConfig(configView.snapshot) : snapshot)}>
                    重置到当前快照
                  </Button>
                </Space>
              </Space>
            </Card>

            <Card className="table-card" title="Recent Runtime Config History">
              <Table
                rowKey={(record: RuntimeConfigHistoryItem) => `${record.config_key}-${record.version}-${record.created_at}`}
                dataSource={history}
                pagination={false}
                scroll={{ x: 1200 }}
                locale={{ emptyText: <EmptyStateCard title="暂无配置历史" description="当前还没有动态配置变更记录。" /> }}
                columns={[
                  { title: 'Time', dataIndex: 'created_at', width: 180, render: (value: string) => formatDateTime(value) },
                  { title: 'Key', dataIndex: 'config_key', width: 260 },
                  { title: 'Version', dataIndex: 'version', width: 90 },
                  { title: 'Status', dataIndex: 'status', width: 140, render: (value: string) => <StatusTag value={value} /> },
                  { title: 'Value', dataIndex: 'value', render: (value: unknown) => <Text type="secondary">{typeof value === 'string' ? value : JSON.stringify(value)}</Text> },
                  { title: 'Operator', dataIndex: 'created_by', width: 140 },
                  { title: 'Reason', dataIndex: 'reason' },
                ]}
              />
            </Card>
          </>
        ) : (
          <EmptyStateCard title="暂无配置快照" description="当前未能加载运行时配置。" />
        )}
      </Space>
    </div>
  );
}

export function AdminLiquidationsPage() {
  return (
    <AdminPageTemplate
      title="Liquidations"
      description="查看强平执行记录与相关状态。"
      items={['账户风险快照', '清算前撤单记录', '罚金分录', '重放与审计入口']}
    />
  );
}

function formatLedgerValue(asset: string, value: string) {
  if (asset === 'USDC') {
    return formatUsd(value, 8);
  }
  return `${formatDecimal(value, 8)} ${asset}`;
}

function formatBps(value: number) {
  if (value > 0) {
    return `+${value} bps`;
  }
  if (value < 0) {
    return `${value} bps`;
  }
  return '0 bps';
}
