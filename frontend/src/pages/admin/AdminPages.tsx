import { Alert, Button, Card, Input, InputNumber, List, Modal, Select, Space, Spin, Switch, Table, Tooltip, Typography, message } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { ApiError, api } from '../../shared/api';
import { EmptyStateCard, ErrorAlert, MetricCard, PageIntro, StatusTag } from '../../shared/components';
import type { AdminHedgeIntentItem, AdminLiquidationItem, AdminWithdrawReviewItem, LedgerAuditReport, LedgerAssetOverview, LedgerChainBalance, LedgerOverview, RiskMonitorDashboard, RuntimeConfigHistoryItem, RuntimeConfigPairOverrideView, RuntimeConfigPairPatchRequest, RuntimeConfigPatchRequest, RuntimeConfigSnapshotView, RuntimeConfigView, SymbolItem, SymbolNetExposureItem, SystemHedgeSnapshotItem } from '../../shared/domain';
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

type EditablePairOverride = {
  max_leverage: string;
  session_policy: string;
  taker_fee_rate: string;
  maker_fee_rate: string;
  default_max_slippage_bps?: number;
  liquidation_penalty_rate: string;
  maintenance_margin_uplift_ratio: string;
  funding_interval_sec?: number;
};

type PairEditorState = {
  mode: 'create' | 'edit';
  pair: string;
  source_pair?: string;
  initial_override: EditablePairOverride;
  override: EditablePairOverride;
};

type EditableRuntimeConfig = Omit<RuntimeConfigSnapshotView, 'pair_overrides'> & {
  pair_overrides: Record<string, EditablePairOverride>;
  new_pair: string;
};

type RuntimeConfigScope = 'global' | 'market' | 'risk' | 'funding' | 'hedge' | 'pairs';

type PendingRuntimeConfigSave = {
  scope: RuntimeConfigScope;
  label: string;
  payload: RuntimeConfigPatchRequest;
  stagedForm: EditableRuntimeConfig;
};

const sessionPolicyOptions = [
  { label: '始终开放', value: 'ALWAYS_OPEN' },
  { label: '美股常规时段', value: 'US_EQUITY_REGULAR' },
  { label: '黄金 24x5', value: 'XAUUSD_24_5' },
];

function normalizeLeverageValue(value?: string) {
  const trimmed = (value || '').trim();
  if (!trimmed) {
    return '';
  }
  const [whole] = trimmed.split('.');
  return /^\d+$/.test(whole) ? whole : '';
}

function toEditableRuntimeConfig(snapshot: RuntimeConfigSnapshotView): EditableRuntimeConfig {
  return {
    ...snapshot,
    pair_overrides: toEditablePairOverrides(snapshot.pair_overrides),
    new_pair: '',
  };
}

function toEditablePairOverrides(source?: Record<string, RuntimeConfigPairOverrideView>) {
  return Object.fromEntries(Object.entries(source || {}).map(([pair, value]) => [pair, {
    max_leverage: normalizeLeverageValue(value.max_leverage),
    session_policy: value.session_policy || '',
    taker_fee_rate: value.taker_fee_rate || '',
    maker_fee_rate: value.maker_fee_rate || '',
    default_max_slippage_bps: value.default_max_slippage_bps,
    liquidation_penalty_rate: value.liquidation_penalty_rate || '',
    maintenance_margin_uplift_ratio: value.maintenance_margin_uplift_ratio || '',
    funding_interval_sec: value.funding_interval_sec,
  }]));
}

function emptyPairOverride(): EditablePairOverride {
  return {
    max_leverage: '',
    session_policy: '',
    taker_fee_rate: '',
    maker_fee_rate: '',
    liquidation_penalty_rate: '',
    maintenance_margin_uplift_ratio: '',
  };
}

function normalizePair(value: string) {
  return value.trim().toUpperCase();
}

function isPairFormatValid(value: string) {
  return /^[A-Z0-9]+-[A-Z0-9]+$/.test(normalizePair(value));
}

function buildEffectivePairOverride(
  pair: string,
  snapshot: EditableRuntimeConfig,
  symbols: SymbolItem[],
): EditablePairOverride {
  const normalizedPair = normalizePair(pair);
  const symbol = symbols.find((item) => item.symbol === normalizedPair);
  const pairOverride = snapshot.pair_overrides[normalizedPair];
  return {
    max_leverage: normalizeLeverageValue(pairOverride?.max_leverage || symbol?.max_leverage),
    session_policy: pairOverride?.session_policy || symbol?.session_policy || 'ALWAYS_OPEN',
    taker_fee_rate: pairOverride?.taker_fee_rate || snapshot.market_taker_fee_rate,
    maker_fee_rate: pairOverride?.maker_fee_rate || snapshot.market_maker_fee_rate,
    default_max_slippage_bps: pairOverride?.default_max_slippage_bps || snapshot.market_default_max_slippage_bps,
    liquidation_penalty_rate: pairOverride?.liquidation_penalty_rate || snapshot.risk_liquidation_penalty_rate,
    maintenance_margin_uplift_ratio: pairOverride?.maintenance_margin_uplift_ratio || snapshot.risk_maintenance_margin_uplift_ratio,
    funding_interval_sec: pairOverride?.funding_interval_sec || snapshot.funding_interval_sec,
  };
}

function trimPairField(value: string) {
  return value.trim();
}

function positivePairNumber(value?: number) {
  return typeof value === 'number' && value > 0 ? value : undefined;
}

function buildPersistedPairOverrideDraft(
  pair: string,
  draft: EditablePairOverride,
  snapshot: EditableRuntimeConfig,
  symbols: SymbolItem[],
): EditablePairOverride {
  const current = snapshot.pair_overrides[pair] || emptyPairOverride();
  const effective = buildEffectivePairOverride(pair, snapshot, symbols);
  const resolveString = (currentValue: string, effectiveValue: string, draftValue: string) => {
    const currentTrimmed = trimPairField(currentValue);
    const effectiveTrimmed = trimPairField(effectiveValue);
    const draftTrimmed = trimPairField(draftValue);
    if (currentTrimmed) {
      return draftTrimmed || currentTrimmed;
    }
    if (draftTrimmed && draftTrimmed !== effectiveTrimmed) {
      return draftTrimmed;
    }
    return '';
  };
  const resolveNumber = (currentValue: number | undefined, effectiveValue: number | undefined, draftValue: number | undefined) => {
    const currentPositive = positivePairNumber(currentValue);
    const effectivePositive = positivePairNumber(effectiveValue);
    const draftPositive = positivePairNumber(draftValue);
    if (currentPositive !== undefined) {
      return draftPositive ?? currentPositive;
    }
    if (draftPositive !== undefined && draftPositive !== effectivePositive) {
      return draftPositive;
    }
    return undefined;
  };
  return {
    max_leverage: normalizeLeverageValue(resolveString(current.max_leverage, effective.max_leverage, draft.max_leverage)),
    session_policy: resolveString(current.session_policy, effective.session_policy, draft.session_policy),
    taker_fee_rate: resolveString(current.taker_fee_rate, effective.taker_fee_rate, draft.taker_fee_rate),
    maker_fee_rate: resolveString(current.maker_fee_rate, effective.maker_fee_rate, draft.maker_fee_rate),
    default_max_slippage_bps: resolveNumber(current.default_max_slippage_bps, effective.default_max_slippage_bps, draft.default_max_slippage_bps),
    liquidation_penalty_rate: resolveString(current.liquidation_penalty_rate, effective.liquidation_penalty_rate, draft.liquidation_penalty_rate),
    maintenance_margin_uplift_ratio: resolveString(current.maintenance_margin_uplift_ratio, effective.maintenance_margin_uplift_ratio, draft.maintenance_margin_uplift_ratio),
    funding_interval_sec: resolveNumber(current.funding_interval_sec, effective.funding_interval_sec, draft.funding_interval_sec),
  };
}

function hasPersistedPairOverrideValue(override: EditablePairOverride) {
  return Boolean(
    trimPairField(override.max_leverage)
    || trimPairField(override.session_policy)
    || trimPairField(override.taker_fee_rate)
    || trimPairField(override.maker_fee_rate)
    || trimPairField(override.liquidation_penalty_rate)
    || trimPairField(override.maintenance_margin_uplift_ratio)
    || positivePairNumber(override.default_max_slippage_bps)
    || positivePairNumber(override.funding_interval_sec),
  );
}

function equalOptionalNumber(left?: number, right?: number) {
  return positivePairNumber(left) === positivePairNumber(right);
}

function equalOptionalString(left?: string, right?: string) {
  return trimPairField(left || '') === trimPairField(right || '');
}

function equalPairOverridePatch(left: RuntimeConfigPairPatchRequest | null, right: RuntimeConfigPairPatchRequest | null) {
  if (!left && !right) {
    return true;
  }
  if (!left || !right) {
    return false;
  }
  return (
    equalOptionalString(left.market?.max_leverage, right.market?.max_leverage)
    && equalOptionalString(left.market?.session_policy, right.market?.session_policy)
    && equalOptionalString(left.market?.taker_fee_rate, right.market?.taker_fee_rate)
    && equalOptionalString(left.market?.maker_fee_rate, right.market?.maker_fee_rate)
    && equalOptionalNumber(left.market?.default_max_slippage_bps, right.market?.default_max_slippage_bps)
    && equalOptionalString(left.risk?.liquidation_penalty_rate, right.risk?.liquidation_penalty_rate)
    && equalOptionalString(left.risk?.maintenance_margin_uplift_ratio, right.risk?.maintenance_margin_uplift_ratio)
    && equalOptionalNumber(left.funding?.interval_sec, right.funding?.interval_sec)
  );
}

function toPairOverridePatch(override: EditablePairOverride) {
  const market: Record<string, string | number> = {};
  const risk: Record<string, string> = {};
  const funding: Record<string, number> = {};
  if (override.max_leverage.trim()) {
    market.max_leverage = normalizeLeverageValue(override.max_leverage);
  }
  if (override.session_policy.trim()) {
    market.session_policy = override.session_policy.trim();
  }
  if (override.taker_fee_rate.trim()) {
    market.taker_fee_rate = override.taker_fee_rate.trim();
  }
  if (override.maker_fee_rate.trim()) {
    market.maker_fee_rate = override.maker_fee_rate.trim();
  }
  if ((override.default_max_slippage_bps || 0) > 0) {
    market.default_max_slippage_bps = override.default_max_slippage_bps as number;
  }
  if (override.liquidation_penalty_rate.trim()) {
    risk.liquidation_penalty_rate = override.liquidation_penalty_rate.trim();
  }
  if (override.maintenance_margin_uplift_ratio.trim()) {
    risk.maintenance_margin_uplift_ratio = override.maintenance_margin_uplift_ratio.trim();
  }
  if ((override.funding_interval_sec || 0) > 0) {
    funding.interval_sec = override.funding_interval_sec as number;
  }
  const patch: RuntimeConfigPairPatchRequest = {};
  if (Object.keys(market).length > 0) {
    patch.market = market;
  }
  if (Object.keys(risk).length > 0) {
    patch.risk = risk;
  }
  if (Object.keys(funding).length > 0) {
    patch.funding = funding;
  }
  return Object.keys(patch).length > 0 ? patch : null;
}

export function AdminDashboardPage() {
  const [selectedAsset, setSelectedAsset] = useState<string>('ALL');
  const [showAllRiskRows, setShowAllRiskRows] = useState(false);
  const [showAllHedgeRows, setShowAllHedgeRows] = useState(false);
  const [showAllHedgeIntentRows, setShowAllHedgeIntentRows] = useState(false);
  const [overview, setOverview] = useState<LedgerOverview | null>(null);
  const [audit, setAudit] = useState<LedgerAuditReport | null>(null);
  const [riskDashboard, setRiskDashboard] = useState<RiskMonitorDashboard | null>(null);
  const [hedgeIntents, setHedgeIntents] = useState<AdminHedgeIntentItem[]>([]);
  const [hedgeSnapshots, setHedgeSnapshots] = useState<SystemHedgeSnapshotItem[]>([]);
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
      const [overviewData, riskData, latestAudit, hedgeIntentData, hedgeSnapshotData] = await Promise.all([
        api.admin.getLedgerOverview(scopeAsset),
        api.admin.getRiskMonitorDashboard(),
        api.admin.getLatestLedgerAudit(scopeAsset).catch((loadError) => {
          const text = loadError instanceof Error ? loadError.message.toLowerCase() : '';
          if ((loadError instanceof ApiError && loadError.status === 404) || text.includes('not found')) {
            return null;
          }
          throw loadError;
        }),
        api.admin.getHedgeIntents(),
        api.admin.getHedgeSnapshots(),
      ]);
      setOverview(overviewData);
      setRiskDashboard(riskData);
      setAudit(latestAudit);
      setHedgeIntents(hedgeIntentData);
      setHedgeSnapshots(hedgeSnapshotData);
    } catch (loadError) {
      setError(loadError);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadData(selectedAsset);
  }, [selectedAsset]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadData(selectedAsset, true);
    }, 5000);
    return () => window.clearInterval(timer);
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
  const hedgeRows = useMemo(() => [...hedgeSnapshots].sort(compareHedgeSnapshotRows), [hedgeSnapshots]);
  const visibleHedgeRows = showAllHedgeRows ? hedgeRows : hedgeRows.slice(0, 8);
  const sortedHedgeIntents = useMemo(() => [...hedgeIntents].sort(compareHedgeIntentRows), [hedgeIntents]);
  const activeHedgeIntentRows = useMemo(() => sortedHedgeIntents.filter(isOpenHedgeIntentRow), [sortedHedgeIntents]);
  const defaultHedgeIntentRows = useMemo(() => {
    if (activeHedgeIntentRows.length === 0) {
      return sortedHedgeIntents.slice(0, 8);
    }
    const historyRows = sortedHedgeIntents.filter((item) => !isOpenHedgeIntentRow(item));
    return [...activeHedgeIntentRows, ...historyRows.slice(0, Math.max(0, 8 - activeHedgeIntentRows.length))];
  }, [activeHedgeIntentRows, sortedHedgeIntents]);
  const visibleHedgeIntentRows = showAllHedgeIntentRows ? sortedHedgeIntents : defaultHedgeIntentRows;
  const failedChecks = audit?.checks.filter((item) => item.status === 'FAIL').length || 0;
  const blockedSymbols = riskRows.filter((item) => item.blocked_open_side).length;
  const unhealthyHedgeSnapshots = hedgeSnapshots.filter((item) => !item.hedge_healthy).length;
  const pendingHedgeIntents = hedgeIntents.filter((item) => item.status === 'PENDING' || item.status === 'EXECUTING').length;
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
          <MetricCard label="待处理对冲" value={String(pendingHedgeIntents)} hint={hedgeIntents.length > 0 ? `最近 intent ${formatDateTime(hedgeIntents[0]?.updated_at)}` : '暂无对冲意图'} accent={pendingHedgeIntents > 0 ? 'warm' : 'cool'} />
          <MetricCard label="对冲异常快照" value={String(unhealthyHedgeSnapshots)} hint={hedgeSnapshots.length > 0 ? `最近快照 ${formatDateTime(hedgeSnapshots[0]?.created_at)}` : '暂无对冲快照'} accent={unhealthyHedgeSnapshots > 0 ? 'warm' : 'cool'} />
          <MetricCard label="概览更新时间" value={formatDateTime(overview?.generated_at)} hint="账本快照读取时间" accent="neutral" />
        </Space>

        {hedgeSnapshots.length > 0 ? (
          <Card
            className="table-card"
            title="对冲风险快照"
            extra={
              showAllHedgeRows || hedgeRows.length > 8 ? (
                <Button type="text" onClick={() => setShowAllHedgeRows((value) => !value)}>
                  {showAllHedgeRows ? `收起 · 已显示 ${visibleHedgeRows.length}/${hedgeRows.length}` : `展开全部 · 已显示 ${visibleHedgeRows.length}/${hedgeRows.length}`}
                </Button>
              ) : null
            }
          >
            <Paragraph type="secondary" style={{ marginBottom: 12 }}>
              当前对冲执行只基于内部净敞口与系统已管理仓位。外部仓位字段仅作观测展示，不参与新对冲单目标计算。
            </Paragraph>
            <Table
              rowKey="symbol"
              dataSource={visibleHedgeRows}
              pagination={false}
              scroll={{ x: 1180 }}
              columns={[
                { title: '交易对', dataIndex: 'symbol', width: 140 },
                { title: '健康度', dataIndex: 'hedge_healthy', width: 120, render: (value: boolean) => <StatusTag value={value ? 'HEALTHY' : 'DRIFT'} /> },
                { title: '内部净仓', dataIndex: 'internal_net_qty', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                { title: '目标对冲', dataIndex: 'target_hedge_qty', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                { title: '已管理仓位', dataIndex: 'managed_hedge_qty', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                { title: '外部仓位', dataIndex: 'external_hedge_qty', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                { title: '管理漂移', dataIndex: 'managed_drift_qty', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                { title: '外部漂移', dataIndex: 'external_drift_qty', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                { title: '时间', dataIndex: 'created_at', width: 180, render: (value: string) => formatDateTime(value) },
              ]}
            />
          </Card>
        ) : null}

        {hedgeIntents.length > 0 ? (
          <Card
            className="table-card"
            title="对冲执行队列"
            extra={
              showAllHedgeIntentRows || sortedHedgeIntents.length > defaultHedgeIntentRows.length ? (
                <Button type="text" onClick={() => setShowAllHedgeIntentRows((value) => !value)}>
                  {showAllHedgeIntentRows
                    ? `收起 · 已显示 ${visibleHedgeIntentRows.length}/${sortedHedgeIntents.length}`
                    : `展开全部 · 已显示 ${visibleHedgeIntentRows.length}/${sortedHedgeIntents.length}`}
                </Button>
              ) : null
            }
          >
            <Paragraph type="secondary" style={{ marginBottom: 12 }}>
              PENDING、EXECUTING、FAILED 是当前需要处理的对冲任务。COMPLETED、SUPERSEDED 是系统历史执行留痕，不代表你手工下过这些数量。
            </Paragraph>
            <Table
              rowKey="hedge_intent_id"
              dataSource={visibleHedgeIntentRows}
              pagination={false}
              scroll={{ x: 1180 }}
              columns={[
                { title: 'Intent', dataIndex: 'hedge_intent_id', width: 160, render: (value: string) => formatAddress(value, 8) },
                { title: '交易对', dataIndex: 'symbol', width: 140 },
                { title: '方向', dataIndex: 'side', width: 120, render: (value: string) => <StatusTag value={value} /> },
                { title: '目标数量', dataIndex: 'target_qty', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                { title: '当前净敞口', dataIndex: 'current_net_exposure', align: 'right', render: (value: string) => formatDecimal(value, 8) },
                { title: 'Intent 状态', dataIndex: 'status', width: 140, render: (value: string) => <StatusTag value={value} /> },
                { title: 'Venue', dataIndex: 'latest_venue', width: 180, render: (value?: string | null) => value || '--' },
                { title: '订单状态', dataIndex: 'latest_order_status', width: 140, render: (value?: string | null) => (value ? <StatusTag value={value} /> : '--') },
                { title: 'Venue Order', dataIndex: 'latest_venue_order_id', render: (value?: string | null) => value || '--' },
                { title: '错误码', dataIndex: 'latest_error_code', width: 140, render: (value?: string | null) => value || '--' },
                { title: '更新时间', dataIndex: 'updated_at', width: 180, render: (value: string) => formatDateTime(value) },
              ]}
            />
          </Card>
        ) : null}

        {riskDashboard ? (
          <Card
            className="table-card"
            title="净敞口监控"
            extra={
              showAllRiskRows || riskRows.length > 8 ? (
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
  const [processingAction, setProcessingAction] = useState<string | null>(null);

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
      setProcessingAction(`approve:${withdrawId}`);
      await api.admin.approveWithdrawal(withdrawId);
      await loadData(true);
    } catch (approveError) {
      setError(approveError);
    } finally {
      setProcessingAction(null);
    }
  }

  async function handleReturnToReview(withdrawId: string) {
    try {
      setProcessingAction(`review:${withdrawId}`);
      await api.admin.returnWithdrawalToReview(withdrawId);
      await loadData(true);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setProcessingAction(null);
    }
  }

  async function handleRefund(withdrawId: string) {
    try {
      setProcessingAction(`refund:${withdrawId}`);
      await api.admin.refundWithdrawal(withdrawId);
      await loadData(true);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setProcessingAction(null);
    }
  }

  return (
    <div className="rg-app-page rg-app-page--admin">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro eyebrow="Admin" title="Withdraw Reviews" description="仅命中特殊风控规则的提现会进入人工复核。审核通过后会继续处理。" titleEffect="glitch" descriptionEffect="proximity" />
        <Alert
          showIcon
          type="info"
          message="审批与异常处理"
          description="RISK_REVIEW 表示待审核；FAILED 表示执行阶段被系统驳回；管理员可对异常单执行“退回审核”或“退款”。"
        />
        <ErrorAlert error={error} />
        <Card className="table-card" title="Withdrawal Queue">
          <Table
            rowKey="withdraw_id"
            loading={loading}
            dataSource={items}
            pagination={false}
            scroll={{ x: 1500 }}
            locale={{ emptyText: <EmptyStateCard title="暂无待处理审核" description="当前没有进入 RISK_REVIEW 的提现，或你没有管理员权限。" /> }}
            columns={[
              { title: 'Time', dataIndex: 'created_at', width: 180, render: (value: string) => formatDateTime(value) },
              {
                title: 'User',
                dataIndex: 'user_address',
                width: 150,
                render: (value: string) => (
                  <Tooltip title={value}>
                    <Text type="secondary" style={{ whiteSpace: 'nowrap' }}>{formatAddress(value, 8)}</Text>
                  </Tooltip>
                ),
              },
              { title: 'Chain', dataIndex: 'chain_id', width: 120, render: (value: number) => formatChainName(value, chains) },
              { title: 'Asset', dataIndex: 'asset', width: 90 },
              { title: 'Amount', dataIndex: 'amount', width: 140, align: 'right', render: (value: string) => formatUsd(value) },
              { title: 'Fee', dataIndex: 'fee_amount', width: 110, align: 'right', render: (value: string) => formatUsd(value) },
              {
                title: 'To',
                dataIndex: 'to_address',
                width: 160,
                render: (value: string) => (
                  <Tooltip title={value}>
                    <Text type="secondary" style={{ whiteSpace: 'nowrap' }}>{formatAddress(value, 8)}</Text>
                  </Tooltip>
                ),
              },
              {
                title: 'Reason',
                dataIndex: 'risk_flag',
                width: 240,
                ellipsis: true,
                render: (value?: string | null) => (
                  <Tooltip title={value || '--'}>
                    <Text style={{ whiteSpace: 'nowrap' }}>{value || '--'}</Text>
                  </Tooltip>
                ),
              },
              { title: 'Status', dataIndex: 'status', width: 140, render: (value: string) => <StatusTag value={value} /> },
              { title: 'Updated', dataIndex: 'updated_at', width: 180, render: (value: string) => formatDateTime(value) },
              {
                title: 'Action',
                width: 260,
                render: (_, record: AdminWithdrawReviewItem) => (
                  <Space wrap>
                    <Button
                      size="small"
                      type="primary"
                      disabled={record.status !== 'RISK_REVIEW'}
                      loading={processingAction === `approve:${record.withdraw_id}`}
                      onClick={() => void handleApprove(record.withdraw_id)}
                    >
                      批准
                    </Button>
                    <Button
                      size="small"
                      disabled={!['FAILED', 'SIGNING', 'APPROVED', 'HOLD'].includes(record.status)}
                      loading={processingAction === `review:${record.withdraw_id}`}
                      onClick={() => void handleReturnToReview(record.withdraw_id)}
                    >
                      退回审核
                    </Button>
                    <Button
                      size="small"
                      danger
                      disabled={!['HOLD', 'RISK_REVIEW', 'APPROVED', 'SIGNING', 'FAILED'].includes(record.status)}
                      loading={processingAction === `refund:${record.withdraw_id}`}
                      onClick={() => void handleRefund(record.withdraw_id)}
                    >
                      退款
                    </Button>
                  </Space>
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
  const [symbols, setSymbols] = useState<SymbolItem[]>([]);
  const [pairEditor, setPairEditor] = useState<PairEditorState | null>(null);
  const [pendingSave, setPendingSave] = useState<PendingRuntimeConfigSave | null>(null);
  const [saveReason, setSaveReason] = useState('');
  const [loading, setLoading] = useState(true);
  const [savingScope, setSavingScope] = useState<RuntimeConfigScope | null>(null);
  const [error, setError] = useState<unknown>(null);

  async function loadData(background = false) {
    if (!background) {
      setLoading(true);
    }
    setError(null);
    try {
      const [view, symbolItems] = await Promise.all([
        api.admin.getRuntimeConfig(),
        api.market.getSymbols(),
      ]);
      setConfigView(view);
      setSymbols(symbolItems);
      setForm((current) => {
        if (!current || !background) {
          return toEditableRuntimeConfig(view.snapshot);
        }
        return current;
      });
      if (!background) {
        setPairEditor(null);
      }
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

  function stagePairEditorForSubmit(currentForm: EditableRuntimeConfig) {
    if (!pairEditor) {
      return currentForm;
    }
    const pair = normalizePair(pairEditor.pair || '');
    if (!pair) {
      message.warning('pair 为必填项');
      return null;
    }
    if (!isPairFormatValid(pair)) {
      message.warning('pair 格式必须为 BASE-QUOTE，例如 BTC-USDC');
      return null;
    }
    if (!symbols.some((item) => item.symbol === pair)) {
      message.warning('只能选择系统内已有的交易对');
      return null;
    }
    if (pairEditor.mode === 'create' && currentForm.pair_overrides[pair]) {
      message.warning('该 pair 已存在，请直接调整');
      return null;
    }
    const persistedOverride = buildPersistedPairOverrideDraft(pair, pairEditor.override, currentForm, symbols);
    const nextPairOverrides = { ...currentForm.pair_overrides };
    if (hasPersistedPairOverrideValue(persistedOverride)) {
      nextPairOverrides[pair] = persistedOverride;
    } else {
      delete nextPairOverrides[pair];
    }
    return {
      ...currentForm,
      new_pair: pair,
      pair_overrides: nextPairOverrides,
    };
  }

  function buildRuntimeConfigPayload(stagedForm: EditableRuntimeConfig, currentSnapshot: RuntimeConfigSnapshotView, scope: RuntimeConfigScope) {
    const payload: RuntimeConfigPatchRequest = { reason: '' };

    if (scope === 'global') {
      const globalPatch: NonNullable<RuntimeConfigPatchRequest['global']> = {};
      if (stagedForm.read_only !== currentSnapshot.read_only) {
        globalPatch.read_only = stagedForm.read_only;
      }
      if (stagedForm.reduce_only !== currentSnapshot.reduce_only) {
        globalPatch.reduce_only = stagedForm.reduce_only;
      }
      if (stagedForm.trace_header_required !== currentSnapshot.trace_header_required) {
        globalPatch.trace_header_required = stagedForm.trace_header_required;
      }
      if (Object.keys(globalPatch).length > 0) {
        payload.global = globalPatch;
      }
    }

    if (scope === 'market') {
      const marketPatch: NonNullable<RuntimeConfigPatchRequest['market']> = {};
      if (trimPairField(stagedForm.market_taker_fee_rate) !== trimPairField(currentSnapshot.market_taker_fee_rate)) {
        marketPatch.taker_fee_rate = trimPairField(stagedForm.market_taker_fee_rate);
      }
      if (trimPairField(stagedForm.market_maker_fee_rate) !== trimPairField(currentSnapshot.market_maker_fee_rate)) {
        marketPatch.maker_fee_rate = trimPairField(stagedForm.market_maker_fee_rate);
      }
      if (stagedForm.market_default_max_slippage_bps !== currentSnapshot.market_default_max_slippage_bps) {
        marketPatch.default_max_slippage_bps = stagedForm.market_default_max_slippage_bps;
      }
      if (Object.keys(marketPatch).length > 0) {
        payload.market = marketPatch;
      }
    }

    if (scope === 'risk') {
      const riskPatch: NonNullable<RuntimeConfigPatchRequest['risk']> = {};
      if (trimPairField(stagedForm.risk_global_buffer_ratio) !== trimPairField(currentSnapshot.risk_global_buffer_ratio)) {
        riskPatch.global_buffer_ratio = trimPairField(stagedForm.risk_global_buffer_ratio);
      }
      if (stagedForm.risk_mark_price_stale_sec !== currentSnapshot.risk_mark_price_stale_sec) {
        riskPatch.mark_price_stale_sec = stagedForm.risk_mark_price_stale_sec;
      }
      if (stagedForm.risk_force_reduce_only_on_stale_price !== currentSnapshot.risk_force_reduce_only_on_stale_price) {
        riskPatch.force_reduce_only_on_stale_price = stagedForm.risk_force_reduce_only_on_stale_price;
      }
      if (trimPairField(stagedForm.risk_liquidation_penalty_rate) !== trimPairField(currentSnapshot.risk_liquidation_penalty_rate)) {
        riskPatch.liquidation_penalty_rate = trimPairField(stagedForm.risk_liquidation_penalty_rate);
      }
      if (trimPairField(stagedForm.risk_maintenance_margin_uplift_ratio) !== trimPairField(currentSnapshot.risk_maintenance_margin_uplift_ratio)) {
        riskPatch.maintenance_margin_uplift_ratio = trimPairField(stagedForm.risk_maintenance_margin_uplift_ratio);
      }
      if (stagedForm.risk_liquidation_extra_slippage_bps !== currentSnapshot.risk_liquidation_extra_slippage_bps) {
        riskPatch.liquidation_extra_slippage_bps = stagedForm.risk_liquidation_extra_slippage_bps;
      }
      if (stagedForm.risk_max_open_orders_per_user_per_symbol !== currentSnapshot.risk_max_open_orders_per_user_per_symbol) {
        riskPatch.max_open_orders_per_user_per_symbol = stagedForm.risk_max_open_orders_per_user_per_symbol;
      }
      if (trimPairField(stagedForm.risk_net_exposure_hard_limit) !== trimPairField(currentSnapshot.risk_net_exposure_hard_limit)) {
        riskPatch.net_exposure_hard_limit = trimPairField(stagedForm.risk_net_exposure_hard_limit);
      }
      if (stagedForm.risk_max_exposure_slippage_bps !== currentSnapshot.risk_max_exposure_slippage_bps) {
        riskPatch.max_exposure_slippage_bps = stagedForm.risk_max_exposure_slippage_bps;
      }
      if (Object.keys(riskPatch).length > 0) {
        payload.risk = riskPatch;
      }
    }

    if (scope === 'funding') {
      const fundingPatch: NonNullable<RuntimeConfigPatchRequest['funding']> = {};
      if (stagedForm.funding_interval_sec !== currentSnapshot.funding_interval_sec) {
        fundingPatch.interval_sec = stagedForm.funding_interval_sec;
      }
      if (stagedForm.funding_source_poll_interval_sec !== currentSnapshot.funding_source_poll_interval_sec) {
        fundingPatch.source_poll_interval_sec = stagedForm.funding_source_poll_interval_sec;
      }
      if (trimPairField(stagedForm.funding_cap_rate_per_hour) !== trimPairField(currentSnapshot.funding_cap_rate_per_hour)) {
        fundingPatch.cap_rate_per_hour = trimPairField(stagedForm.funding_cap_rate_per_hour);
      }
      if (stagedForm.funding_min_valid_source_count !== currentSnapshot.funding_min_valid_source_count) {
        fundingPatch.min_valid_source_count = stagedForm.funding_min_valid_source_count;
      }
      if (trimPairField(stagedForm.funding_default_model_crypto) !== trimPairField(currentSnapshot.funding_default_model_crypto)) {
        fundingPatch.default_model_crypto = trimPairField(stagedForm.funding_default_model_crypto);
      }
      if (Object.keys(fundingPatch).length > 0) {
        payload.funding = fundingPatch;
      }
    }

    if (scope === 'hedge') {
      const hedgePatch: NonNullable<RuntimeConfigPatchRequest['hedge']> = {};
      if (stagedForm.hedge_enabled !== currentSnapshot.hedge_enabled) {
        hedgePatch.enabled = stagedForm.hedge_enabled;
      }
      if (trimPairField(stagedForm.hedge_soft_threshold_ratio) !== trimPairField(currentSnapshot.hedge_soft_threshold_ratio)) {
        hedgePatch.soft_threshold_ratio = trimPairField(stagedForm.hedge_soft_threshold_ratio);
      }
      if (trimPairField(stagedForm.hedge_hard_threshold_ratio) !== trimPairField(currentSnapshot.hedge_hard_threshold_ratio)) {
        hedgePatch.hard_threshold_ratio = trimPairField(stagedForm.hedge_hard_threshold_ratio);
      }
      if (Object.keys(hedgePatch).length > 0) {
        payload.hedge = hedgePatch;
      }
    }

    if (scope === 'pairs') {
      const currentPairOverrides = toEditablePairOverrides(currentSnapshot.pair_overrides);
      const pairPatchEntries = Object.entries(stagedForm.pair_overrides).flatMap(([pair, value]) => {
        const nextPatch = toPairOverridePatch(value);
        const currentPatch = toPairOverridePatch(currentPairOverrides[pair] || emptyPairOverride());
        if (!nextPatch || equalPairOverridePatch(nextPatch, currentPatch)) {
          return [];
        }
        return [[pair.trim().toUpperCase(), nextPatch] as const];
      });
      if (pairPatchEntries.length > 0) {
        payload.pairs = Object.fromEntries(pairPatchEntries);
      }
    }

    return payload;
  }

  function hasPayloadChanges(payload: RuntimeConfigPatchRequest) {
    return Boolean(payload.global || payload.market || payload.risk || payload.funding || payload.hedge || payload.pairs);
  }

  async function handleScopedSave(scope: RuntimeConfigScope, label: string) {
    if (!form || !configView) {
      return;
    }
    const stagedForm = scope === 'pairs' ? stagePairEditorForSubmit(form) : form;
    if (!stagedForm) {
      return;
    }
    const payload = buildRuntimeConfigPayload(stagedForm, configView.snapshot, scope);
    if (!hasPayloadChanges(payload)) {
      message.warning(`未检测到${label}的待提交变更`);
      return;
    }
    setForm(stagedForm);
    setSaveReason('');
    setPendingSave({
      scope,
      label,
      payload,
      stagedForm,
    });
  }

  async function confirmScopedSave() {
    if (!pendingSave) {
      return;
    }
    if (!saveReason.trim()) {
      message.warning('请填写变更理由');
      return;
    }
    try {
      setSavingScope(pendingSave.scope);
      const view = await api.admin.updateRuntimeConfig({
        ...pendingSave.payload,
        reason: saveReason.trim(),
      });
      setConfigView(view);
      setForm(toEditableRuntimeConfig(view.snapshot));
      if (pendingSave.scope === 'pairs') {
        setPairEditor(null);
      }
      setPendingSave(null);
      setSaveReason('');
      message.success(`${pendingSave.label}已更新`);
    } catch (saveError) {
      setError(saveError);
      message.error(saveError instanceof Error ? saveError.message : `${pendingSave.label}更新失败`);
    } finally {
      setSavingScope(null);
    }
  }

  const snapshot = form;
  const history = configView?.history || [];
  const pairEntries = useMemo(() => Object.entries(snapshot?.pair_overrides || {}).sort(([left], [right]) => left.localeCompare(right)), [snapshot]);
  const systemPairs = useMemo(() => symbols.map((item) => item.symbol).sort((left, right) => left.localeCompare(right)), [symbols]);

  function openCreatePairEditor() {
    setPairEditor({
      mode: 'create',
      pair: '',
      initial_override: emptyPairOverride(),
      override: emptyPairOverride(),
    });
  }

  function openEditPairEditor() {
    const pair = normalizePair(snapshot?.new_pair || '');
    if (!snapshot || !pair) {
      message.warning('请先选择要调整的 pair');
      return;
    }
    if (!symbols.some((item) => item.symbol === pair)) {
      message.warning('请选择系统已有的 pair');
      return;
    }
    const effectiveOverride = buildEffectivePairOverride(pair, snapshot, symbols);
    setPairEditor({
      mode: 'edit',
      pair,
      source_pair: pair,
      initial_override: effectiveOverride,
      override: effectiveOverride,
    });
  }

  function updatePairEditor(patch: Partial<PairEditorState['override']>) {
    setPairEditor((current) => {
      if (!current) {
        return current;
      }
      return {
        ...current,
        override: {
          ...current.override,
          ...patch,
        },
      };
    });
  }

  function applyPairEditor() {
    const pair = normalizePair(pairEditor?.pair || '');
    if (!snapshot || !pairEditor) {
      return;
    }
    if (!pair) {
      message.warning('pair 为必填项');
      return;
    }
    if (!isPairFormatValid(pair)) {
      message.warning('pair 格式必须为 BASE-QUOTE，例如 BTC-USDC');
      return;
    }
    if (!symbols.some((item) => item.symbol === pair)) {
      message.warning('只能选择系统内已有的交易对');
      return;
    }
    if (pairEditor.mode === 'create' && snapshot.pair_overrides[pair]) {
      message.warning('该 pair 已存在，请直接调整');
      return;
    }
    const persistedDraft = buildPersistedPairOverrideDraft(pair, pairEditor.override, snapshot, symbols);
    const nextPairOverrides = {
      ...snapshot.pair_overrides,
    };
    if (hasPersistedPairOverrideValue(persistedDraft)) {
      nextPairOverrides[pair] = persistedDraft;
    } else {
      delete nextPairOverrides[pair];
    }
    const nextSnapshot = {
      ...snapshot,
      pair_overrides: nextPairOverrides,
    };
    const effectiveOverride = buildEffectivePairOverride(pair, nextSnapshot, symbols);
    setForm((current) => current ? {
      ...current,
      new_pair: pair,
      pair_overrides: nextPairOverrides,
    } : current);
    setPairEditor({
      mode: 'edit',
      pair,
      source_pair: pair,
      initial_override: effectiveOverride,
      override: effectiveOverride,
    });
    message.success(pairEditor.mode === 'create' ? `已将 ${pair} 加入当前变更表单` : `已将 ${pair} 的修改保存到本次变更`);
  }

  function resetEditingPair() {
    if (!pairEditor || !snapshot) {
      return;
    }
    if (pairEditor.mode === 'create') {
      setPairEditor({
        mode: 'create',
        pair: '',
        initial_override: emptyPairOverride(),
        override: emptyPairOverride(),
      });
      return;
    }
    if (!pairEditor.source_pair) {
      return;
    }
    setPairEditor({
      mode: 'edit',
      pair: pairEditor.source_pair,
      source_pair: pairEditor.source_pair,
      initial_override: pairEditor.initial_override,
      override: pairEditor.initial_override,
    });
  }

  return (
    <div className="rg-app-page rg-app-page--admin">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro eyebrow="Admin" title="运行配置" description="集中管理运行开关、市场参数与风险阈值。所有变更都会记录理由并写入审计历史。" titleEffect="glitch" descriptionEffect="proximity" />
        <Alert
          showIcon
          type="warning"
          message="变更约束"
          description="每张卡片独立提交，提交前必须填写变更理由。配置会版本化、审计化，并在下个轮询周期同步到各个后端进程。"
        />
        {loading ? <Spin size="large" /> : null}
        <ErrorAlert error={error} />

        {snapshot ? (
          <>
            <Space wrap size={[16, 16]}>
              <MetricCard label="系统模式" value={snapshot.system_mode} hint="当前运行环境模式" accent="neutral" />
              <MetricCard label="只读模式" value={<StatusTag value={snapshot.read_only ? 'ON' : 'OFF'} />} hint="关闭所有写入" accent={snapshot.read_only ? 'warm' : 'cool'} />
              <MetricCard label="只减仓模式" value={<StatusTag value={snapshot.reduce_only ? 'ON' : 'OFF'} />} hint="禁止新增风险敞口" accent={snapshot.reduce_only ? 'warm' : 'cool'} />
              <MetricCard label="最近加载时间" value={formatDateTime(configView?.generated_at)} hint="Admin 页面读取时间" accent="neutral" />
            </Space>

            <Card
              className="surface-card"
              title="全局开关"
              extra={<Button type="primary" loading={savingScope === 'global'} onClick={() => void handleScopedSave('global', '全局开关')}>提交变更</Button>}
            >
              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Space size={12}>
                  <Text style={{ minWidth: 180 }}>只读模式</Text>
                  <Switch checked={snapshot.read_only} onChange={(checked) => setForm((current) => (current ? { ...current, read_only: checked } : current))} />
                </Space>
                <Space size={12}>
                  <Text style={{ minWidth: 180 }}>只减仓模式</Text>
                  <Switch checked={snapshot.reduce_only} onChange={(checked) => setForm((current) => (current ? { ...current, reduce_only: checked } : current))} />
                </Space>
                <Space size={12}>
                  <Text style={{ minWidth: 180 }}>请求追踪头必填</Text>
                  <Switch checked={snapshot.trace_header_required} onChange={(checked) => setForm((current) => (current ? { ...current, trace_header_required: checked } : current))} />
                </Space>
              </Space>
            </Card>

            <Card
              className="surface-card"
              title="市场参数"
              extra={<Button type="primary" loading={savingScope === 'market'} onClick={() => void handleScopedSave('market', '市场参数')}>提交变更</Button>}
            >
              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Space wrap size={16}>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">吃单费率</Text>
                    <Input value={snapshot.market_taker_fee_rate} onChange={(event) => setForm((current) => (current ? { ...current, market_taker_fee_rate: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">挂单费率</Text>
                    <Input value={snapshot.market_maker_fee_rate} onChange={(event) => setForm((current) => (current ? { ...current, market_maker_fee_rate: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">默认最大滑点（bps）</Text>
                    <InputNumber min={1} value={snapshot.market_default_max_slippage_bps} onChange={(value) => setForm((current) => (current ? { ...current, market_default_max_slippage_bps: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                </Space>
              </Space>
            </Card>

            <Card
              className="surface-card"
              title="风险参数"
              extra={<Button type="primary" loading={savingScope === 'risk'} onClick={() => void handleScopedSave('risk', '风险参数')}>提交变更</Button>}
            >
              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Space wrap size={16}>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">风险缓冲比例</Text>
                    <Input value={snapshot.risk_global_buffer_ratio} onChange={(event) => setForm((current) => (current ? { ...current, risk_global_buffer_ratio: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">标记价格失效秒数</Text>
                    <InputNumber min={1} value={snapshot.risk_mark_price_stale_sec} onChange={(value) => setForm((current) => (current ? { ...current, risk_mark_price_stale_sec: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">强平罚金费率</Text>
                    <Input value={snapshot.risk_liquidation_penalty_rate} onChange={(event) => setForm((current) => (current ? { ...current, risk_liquidation_penalty_rate: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">维持保证金上浮比例</Text>
                    <Input value={snapshot.risk_maintenance_margin_uplift_ratio} onChange={(event) => setForm((current) => (current ? { ...current, risk_maintenance_margin_uplift_ratio: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">强平附加滑点（bps）</Text>
                    <InputNumber min={0} value={snapshot.risk_liquidation_extra_slippage_bps} onChange={(value) => setForm((current) => (current ? { ...current, risk_liquidation_extra_slippage_bps: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                </Space>
                <Space wrap size={16}>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">单用户单交易对最大挂单数</Text>
                    <InputNumber min={1} value={snapshot.risk_max_open_orders_per_user_per_symbol} onChange={(value) => setForm((current) => (current ? { ...current, risk_max_open_orders_per_user_per_symbol: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">净敞口硬上限</Text>
                    <Input value={snapshot.risk_net_exposure_hard_limit} onChange={(event) => setForm((current) => (current ? { ...current, risk_net_exposure_hard_limit: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">最大敞口滑点（bps）</Text>
                    <InputNumber min={0} value={snapshot.risk_max_exposure_slippage_bps} onChange={(value) => setForm((current) => (current ? { ...current, risk_max_exposure_slippage_bps: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space size={12}>
                    <Text style={{ minWidth: 180 }}>价格失效时强制只减仓</Text>
                    <Switch checked={snapshot.risk_force_reduce_only_on_stale_price} onChange={(checked) => setForm((current) => (current ? { ...current, risk_force_reduce_only_on_stale_price: checked } : current))} />
                  </Space>
                </Space>
              </Space>
            </Card>

            <Card
              className="surface-card"
              title="资金费参数"
              extra={<Button type="primary" loading={savingScope === 'funding'} onClick={() => void handleScopedSave('funding', '资金费参数')}>提交变更</Button>}
            >
              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Space wrap size={16}>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">资金费结算间隔（秒）</Text>
                    <InputNumber min={60} value={snapshot.funding_interval_sec} onChange={(value) => setForm((current) => (current ? { ...current, funding_interval_sec: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">价格源轮询间隔（秒）</Text>
                    <InputNumber min={1} value={snapshot.funding_source_poll_interval_sec} onChange={(value) => setForm((current) => (current ? { ...current, funding_source_poll_interval_sec: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">每小时费率上限</Text>
                    <Input value={snapshot.funding_cap_rate_per_hour} onChange={(event) => setForm((current) => (current ? { ...current, funding_cap_rate_per_hour: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">最少有效价格源数量</Text>
                    <InputNumber min={1} value={snapshot.funding_min_valid_source_count} onChange={(value) => setForm((current) => (current ? { ...current, funding_min_valid_source_count: Number(value || 0) } : current))} style={{ width: 220 }} />
                  </Space>
                </Space>
                <Space wrap size={16}>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">加密资产默认模型</Text>
                    <Input value={snapshot.funding_default_model_crypto} onChange={(event) => setForm((current) => (current ? { ...current, funding_default_model_crypto: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                </Space>
              </Space>
            </Card>

            <Card
              className="surface-card"
              title="对冲阈值"
              extra={<Button type="primary" loading={savingScope === 'hedge'} onClick={() => void handleScopedSave('hedge', '对冲阈值')}>提交变更</Button>}
            >
              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Space size={12}>
                  <Text style={{ minWidth: 180 }}>启用对冲</Text>
                  <Switch checked={snapshot.hedge_enabled} onChange={(checked) => setForm((current) => (current ? { ...current, hedge_enabled: checked } : current))} />
                </Space>
                <Space wrap size={16}>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">软阈值比例</Text>
                    <Input value={snapshot.hedge_soft_threshold_ratio} onChange={(event) => setForm((current) => (current ? { ...current, hedge_soft_threshold_ratio: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                  <Space direction="vertical" size={4}>
                    <Text type="secondary">硬阈值比例</Text>
                    <Input value={snapshot.hedge_hard_threshold_ratio} onChange={(event) => setForm((current) => (current ? { ...current, hedge_hard_threshold_ratio: event.target.value } : current))} style={{ width: 220 }} />
                  </Space>
                </Space>
              </Space>
            </Card>

            <Card
              className="surface-card"
              title="Pair 参数调整"
              extra={(
                <Space>
                  <Button type="primary" loading={savingScope === 'pairs'} onClick={() => void handleScopedSave('pairs', 'Pair 参数')}>提交变更</Button>
                  <Select
                    showSearch
                    placeholder={systemPairs.length > 0 ? '选择系统已有 Pair' : '暂无系统 Pair 数据'}
                    value={snapshot.new_pair}
                    onChange={(value) => setForm((current) => (current ? { ...current, new_pair: value } : current))}
                    style={{ width: 220 }}
                    options={systemPairs.map((pair) => ({ label: pair, value: pair }))}
                    optionFilterProp="label"
                    notFoundContent={<Text type="secondary">暂无系统 Pair 数据。</Text>}
                  />
                  <Button onClick={openEditPairEditor} disabled={systemPairs.length === 0}>调整 Pair</Button>
                  <Button onClick={openCreatePairEditor}>新增 Pair</Button>
                </Space>
              )}
            >
              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                {pairEditor ? (
                  <Card
                    type="inner"
                    title={pairEditor.mode === 'create' ? '新增 Pair 草稿' : `调整 Pair：${pairEditor.pair}`}
                    extra={(
                      <Space>
                        <Button type="primary" onClick={applyPairEditor}>保存到本次变更</Button>
                        <Button onClick={resetEditingPair}>重置当前草稿</Button>
                        <Button onClick={() => setPairEditor(null)}>关闭</Button>
                      </Space>
                    )}
                  >
                    <Space wrap size={16}>
                      <Space direction="vertical" size={4}>
                        <Text type="secondary">交易对</Text>
                        <Input
                          placeholder="例如 BTC-USDC"
                          value={pairEditor.pair}
                          disabled={pairEditor.mode === 'edit'}
                          onChange={(event) => setPairEditor((current) => current ? { ...current, pair: normalizePair(event.target.value) } : current)}
                          style={{ width: 180 }}
                        />
                      </Space>
                      <Space direction="vertical" size={4}>
                        <Text type="secondary">最大杠杆</Text>
                        <InputNumber
                          min={1}
                          precision={0}
                          value={pairEditor.override.max_leverage ? Number(pairEditor.override.max_leverage) : undefined}
                          onChange={(value) => updatePairEditor({ max_leverage: value ? String(value) : '' })}
                          style={{ width: 180 }}
                        />
                      </Space>
                      <Space direction="vertical" size={4}>
                        <Text type="secondary">交易时段策略</Text>
                        <Select
                          value={pairEditor.override.session_policy || undefined}
                          onChange={(value) => updatePairEditor({ session_policy: value })}
                          style={{ width: 180 }}
                          options={sessionPolicyOptions}
                        />
                      </Space>
                      <Space direction="vertical" size={4}>
                        <Text type="secondary">吃单费率</Text>
                        <Input value={pairEditor.override.taker_fee_rate} onChange={(event) => updatePairEditor({ taker_fee_rate: event.target.value })} style={{ width: 180 }} />
                      </Space>
                      <Space direction="vertical" size={4}>
                        <Text type="secondary">挂单费率</Text>
                        <Input value={pairEditor.override.maker_fee_rate} onChange={(event) => updatePairEditor({ maker_fee_rate: event.target.value })} style={{ width: 180 }} />
                      </Space>
                    </Space>
                    <Space wrap size={16} style={{ marginTop: 16 }}>
                      <Space direction="vertical" size={4}>
                        <Text type="secondary">默认最大滑点（bps）</Text>
                        <InputNumber min={1} value={pairEditor.override.default_max_slippage_bps} onChange={(value) => updatePairEditor({ default_max_slippage_bps: Number(value || 0) || undefined })} style={{ width: 180 }} />
                      </Space>
                      <Space direction="vertical" size={4}>
                        <Text type="secondary">强平罚金费率</Text>
                        <Input value={pairEditor.override.liquidation_penalty_rate} onChange={(event) => updatePairEditor({ liquidation_penalty_rate: event.target.value })} style={{ width: 180 }} />
                      </Space>
                      <Space direction="vertical" size={4}>
                        <Text type="secondary">维持保证金上浮比例</Text>
                        <Input value={pairEditor.override.maintenance_margin_uplift_ratio} onChange={(event) => updatePairEditor({ maintenance_margin_uplift_ratio: event.target.value })} style={{ width: 180 }} />
                      </Space>
                      <Space direction="vertical" size={4}>
                        <Text type="secondary">资金费结算间隔（秒）</Text>
                        <InputNumber min={60} value={pairEditor.override.funding_interval_sec} onChange={(value) => updatePairEditor({ funding_interval_sec: Number(value || 0) || undefined })} style={{ width: 180 }} />
                      </Space>
                    </Space>
                  </Card>
                ) : null}
                {pairEntries.length === 0 ? <EmptyStateCard title="暂无 Pair 覆盖" description="当前只有全局默认参数，尚未写入任何 pair 级覆盖。" /> : null}
                {pairEntries.length > 0 ? (
                  <Card type="inner" title="已配置 Pair 列表">
                    <Space wrap size={[8, 8]}>
                      {pairEntries.map(([pair]) => <StatusTag key={pair} value={pair} />)}
                    </Space>
                  </Card>
                ) : null}
                <Card type="inner" title="提交说明">
                  <Text type="secondary">Pair 变更先保存到当前草稿，点击卡片右上角“提交变更”后再填写理由并提交。</Text>
                </Card>
              </Space>
            </Card>

            <Card className="table-card" title="最近运行配置历史">
              <Table
                rowKey={(record: RuntimeConfigHistoryItem) => `${record.config_key}-${record.version}-${record.created_at}`}
                dataSource={history}
                pagination={false}
                scroll={{ x: 1200 }}
                locale={{ emptyText: <EmptyStateCard title="暂无配置历史" description="当前还没有动态配置变更记录。" /> }}
                columns={[
                  { title: '时间', dataIndex: 'created_at', width: 180, render: (value: string) => formatDateTime(value) },
                  { title: '配置键', dataIndex: 'config_key', width: 260 },
                  { title: '作用域类型', dataIndex: 'scope_type', width: 120 },
                  { title: '作用域值', dataIndex: 'scope_value', width: 160 },
                  { title: '版本', dataIndex: 'version', width: 90 },
                  { title: '状态', dataIndex: 'status', width: 140, render: (value: string) => <StatusTag value={value} /> },
                  { title: '配置值', dataIndex: 'value', render: (value: unknown) => <Text type="secondary">{typeof value === 'string' ? value : JSON.stringify(value)}</Text> },
                  { title: '操作人', dataIndex: 'created_by', width: 140 },
                  { title: '变更理由', dataIndex: 'reason' },
                ]}
              />
            </Card>
          </>
        ) : (
          <EmptyStateCard title="暂无配置快照" description="当前未能加载运行时配置。" />
        )}
      </Space>
      <Modal
        title={pendingSave ? `提交${pendingSave.label}` : '提交配置变更'}
        open={Boolean(pendingSave)}
        okText="确认提交"
        cancelText="取消"
        confirmLoading={Boolean(pendingSave) && savingScope === pendingSave?.scope}
        onCancel={() => {
          if (savingScope) {
            return;
          }
          setPendingSave(null);
          setSaveReason('');
        }}
        onOk={() => void confirmScopedSave()}
      >
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Text type="secondary">请填写本次配置变更的原因，后端会基于当前管理员身份完成鉴权并写入审计记录。</Text>
          <Input.TextArea
            rows={4}
            placeholder="例如：为降低市场波动期间的爆仓风险，临时上调强平附加滑点。"
            value={saveReason}
            onChange={(event) => setSaveReason(event.target.value)}
            maxLength={200}
            showCount
          />
        </Space>
      </Modal>
    </div>
  );
}

export function AdminLiquidationsPage() {
  const [items, setItems] = useState<AdminLiquidationItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [processingAction, setProcessingAction] = useState<string | null>(null);
  const [topUpAmount, setTopUpAmount] = useState('');
  const [topUpReason, setTopUpReason] = useState('');
  const [topUpSource, setTopUpSource] = useState<'SYSTEM_POOL' | 'CUSTODY_HOT'>('SYSTEM_POOL');
  const [closeTarget, setCloseTarget] = useState<AdminLiquidationItem | null>(null);
  const [closeReason, setCloseReason] = useState('');

  async function loadData(background = false) {
    if (!background) {
      setLoading(true);
    }
    setError(null);
    try {
      setItems(await api.admin.getLiquidations());
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

  async function handleTopUpInsuranceFund() {
    if (!topUpAmount.trim()) {
      message.warning('请输入注资金额');
      return;
    }
    if (!topUpReason.trim()) {
      message.warning('请输入注资原因');
      return;
    }
    try {
      setProcessingAction('insurance-topup');
      const result = await api.admin.topUpInsuranceFund({
        asset: 'USDC',
        amount: topUpAmount.trim(),
        source_account: topUpSource,
        reason: topUpReason.trim(),
      });
      message.success(`保险基金已注资 ${formatUsd(result.amount, 8)}`);
      setTopUpAmount('');
      setTopUpReason('');
      await loadData(true);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setProcessingAction(null);
    }
  }

  async function handleRetryLiquidation(liquidationId: string) {
    try {
      setProcessingAction(`retry:${liquidationId}`);
      const result = await api.admin.retryLiquidation(liquidationId);
      message.success(`清算记录已更新为 ${result.status}`);
      await loadData(true);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setProcessingAction(null);
    }
  }

  async function handleConfirmCloseLiquidation() {
    if (!closeTarget) {
      return;
    }
    if (!closeReason.trim()) {
      message.warning('请输入结案原因');
      return;
    }
    try {
      setProcessingAction(`close:${closeTarget.liquidation_id}`);
      const result = await api.admin.closeLiquidation(closeTarget.liquidation_id, closeReason.trim());
      message.success(`清算记录已更新为 ${result.status}`);
      setCloseTarget(null);
      setCloseReason('');
      await loadData(true);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setProcessingAction(null);
    }
  }

  const executedCount = items.filter((item) => item.status === 'EXECUTED').length;
  const executingCount = items.filter((item) => item.status === 'EXECUTING').length;
  const pendingManualCount = items.filter((item) => item.status === 'PENDING_MANUAL').length;
  const abortedCount = items.filter((item) => item.status === 'ABORTED').length;

  return (
    <div className="rg-app-page rg-app-page--admin">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro eyebrow="Admin" title="Liquidations" description="查看真实的强平执行记录、罚金和状态。" titleEffect="glitch" descriptionEffect="proximity" />
        <Alert
          showIcon
          type="info"
          message="坏账处理"
          description="PENDING_MANUAL 表示清算触发后结算仍有缺口。先给 INSURANCE_FUND 注资，再对仍未收口的记录点击“重试清算”；如果仓位已经关闭，可直接“标记结案”。"
        />
        <ErrorAlert error={error} />
        <Space size={16} wrap>
          <MetricCard label="总记录数" value={String(items.length)} hint="最近查询结果" accent="cool" />
          <MetricCard label="已执行" value={<StatusTag value="EXECUTED" />} hint={String(executedCount)} accent="cool" />
          <MetricCard label="执行中" value={<StatusTag value="EXECUTING" />} hint={String(executingCount)} accent="warm" />
          <MetricCard label="待人工处理" value={<StatusTag value="PENDING_MANUAL" />} hint={String(pendingManualCount)} accent="warm" />
          <MetricCard label="已中止" value={<StatusTag value="ABORTED" />} hint={String(abortedCount)} accent="warm" />
        </Space>
        <Card className="table-card" title="保险基金注资">
          <Space direction="vertical" size={12} style={{ width: '100%' }}>
            <Paragraph type="secondary" style={{ marginBottom: 0 }}>
              当前本地环境建议优先从 `SYSTEM_POOL` 向 `INSURANCE_FUND` 注资，再重试 `PENDING_MANUAL` 的逐仓清算。
            </Paragraph>
            <Space wrap size={12}>
              <Select
                value={topUpSource}
                style={{ width: 180 }}
                options={[
                  { label: '系统池 SYSTEM_POOL', value: 'SYSTEM_POOL' },
                  { label: '热钱包镜像 CUSTODY_HOT', value: 'CUSTODY_HOT' },
                ]}
                onChange={(value) => setTopUpSource(value)}
              />
              <Input
                placeholder="注资金额，例如 10"
                value={topUpAmount}
                style={{ width: 220 }}
                onChange={(event) => setTopUpAmount(event.target.value)}
              />
              <Input
                placeholder="注资原因"
                value={topUpReason}
                style={{ minWidth: 320 }}
                onChange={(event) => setTopUpReason(event.target.value)}
              />
              <Button type="primary" loading={processingAction === 'insurance-topup'} onClick={() => void handleTopUpInsuranceFund()}>
                注资到保险基金
              </Button>
            </Space>
          </Space>
        </Card>
        <Card className="table-card" title="Liquidation Records">
          <Table
            rowKey="liquidation_id"
            loading={loading}
            dataSource={items}
            pagination={false}
            scroll={{ x: 1620 }}
            locale={{ emptyText: <EmptyStateCard title="暂无清算记录" description="当前数据库里还没有任何 liquidations 记录。" /> }}
            columns={[
              { title: 'Time', dataIndex: 'created_at', width: 180, render: (value: string) => formatDateTime(value) },
              { title: 'Liquidation ID', dataIndex: 'liquidation_id', width: 200, render: (value: string) => <Text code>{value}</Text> },
              { title: 'User', dataIndex: 'user_address', width: 160, render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text> },
              { title: 'Symbol', dataIndex: 'symbol', width: 120, render: (value?: string | null) => value || '--' },
              { title: 'Mode', dataIndex: 'mode', width: 110, render: (value: string) => <StatusTag value={value} /> },
              { title: 'Status', dataIndex: 'status', width: 140, render: (value: string) => <StatusTag value={value} /> },
              { title: 'Positions', dataIndex: 'position_count', width: 100 },
              { title: 'Penalty', dataIndex: 'penalty_amount', align: 'right', render: (value: string) => formatUsd(value, 8) },
              { title: 'Insurance Used', dataIndex: 'insurance_fund_used', align: 'right', render: (value: string) => formatUsd(value, 8) },
              { title: 'Bankrupt', dataIndex: 'bankrupt_amount', align: 'right', render: (value: string) => formatUsd(value, 8) },
              { title: 'Risk Snapshot', dataIndex: 'trigger_risk_snapshot_id', width: 130 },
              { title: 'Abort Reason', dataIndex: 'abort_reason', width: 180, render: (value?: string | null) => value || '--' },
              { title: 'Updated', dataIndex: 'updated_at', width: 180, render: (value: string) => formatDateTime(value) },
              {
                title: 'Action',
                width: 220,
                render: (_, record: AdminLiquidationItem) => (
                  <Space wrap>
                    <Button
                      size="small"
                      type="primary"
                      disabled={record.status !== 'PENDING_MANUAL'}
                      loading={processingAction === `retry:${record.liquidation_id}`}
                      onClick={() => void handleRetryLiquidation(record.liquidation_id)}
                    >
                      重试清算
                    </Button>
                    <Button
                      size="small"
                      disabled={record.status !== 'PENDING_MANUAL'}
                      loading={processingAction === `close:${record.liquidation_id}`}
                      onClick={() => {
                        setCloseTarget(record);
                        setCloseReason('');
                      }}
                    >
                      标记结案
                    </Button>
                  </Space>
                ),
              },
            ]}
          />
        </Card>
        <Modal
          open={Boolean(closeTarget)}
          title="标记坏账记录结案"
          okText="确认结案"
          cancelText="取消"
          confirmLoading={closeTarget ? processingAction === `close:${closeTarget.liquidation_id}` : false}
          onCancel={() => {
            setCloseTarget(null);
            setCloseReason('');
          }}
          onOk={() => void handleConfirmCloseLiquidation()}
        >
          <Space direction="vertical" size={12} style={{ width: '100%' }}>
            <Paragraph type="secondary" style={{ marginBottom: 0 }}>
              仅在对应仓位已经关闭、这条记录只是历史坏账遗留时使用该操作。
            </Paragraph>
            <Input.TextArea
              value={closeReason}
              rows={4}
              maxLength={200}
              placeholder="请输入结案原因，例如：仓位已手动关闭，清算记录仅保留审计痕迹"
              onChange={(event) => setCloseReason(event.target.value)}
            />
          </Space>
        </Modal>
      </Space>
    </div>
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

function isZeroDecimalString(value?: string | null) {
  return !value || Number(value) === 0;
}

function hasMeaningfulHedgeValue(item: SystemHedgeSnapshotItem) {
  return !isZeroDecimalString(item.internal_net_qty)
    || !isZeroDecimalString(item.target_hedge_qty)
    || !isZeroDecimalString(item.managed_hedge_qty)
    || !isZeroDecimalString(item.external_hedge_qty)
    || !isZeroDecimalString(item.managed_drift_qty)
    || !isZeroDecimalString(item.external_drift_qty);
}

function compareHedgeSnapshotRows(left: SystemHedgeSnapshotItem, right: SystemHedgeSnapshotItem) {
  const leftHasValue = hasMeaningfulHedgeValue(left);
  const rightHasValue = hasMeaningfulHedgeValue(right);
  if (leftHasValue !== rightHasValue) {
    return leftHasValue ? -1 : 1;
  }
  if (left.hedge_healthy !== right.hedge_healthy) {
    return left.hedge_healthy ? 1 : -1;
  }
  return left.symbol.localeCompare(right.symbol);
}

function isOpenHedgeIntentRow(row: AdminHedgeIntentItem) {
  return row.status === 'PENDING' || row.status === 'EXECUTING' || row.status === 'FAILED';
}

function hedgeIntentPriority(row: AdminHedgeIntentItem) {
  if (row.status === 'EXECUTING') {
    return 0;
  }
  if (row.status === 'PENDING') {
    return 1;
  }
  if (row.status === 'FAILED') {
    return 2;
  }
  if (row.status === 'SUPERSEDED') {
    return 4;
  }
  return 3;
}

function compareHedgeIntentRows(left: AdminHedgeIntentItem, right: AdminHedgeIntentItem) {
  const statusDelta = hedgeIntentPriority(left) - hedgeIntentPriority(right);
  if (statusDelta !== 0) {
    return statusDelta;
  }
  return right.updated_at.localeCompare(left.updated_at);
}
