import { App as AntdApp, Alert, Button, Card, Form, Input, Select, Space, Spin, Table, Typography } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../../shared/api';
import { EmptyStateCard, ErrorAlert, LoginRequiredCard, PageIntro, StatusTag, TwoColumnRow } from '../../shared/components';
import type { BalanceItem, TransferItem, TransferRequest } from '../../shared/domain';
import { useAuth } from '../../shared/auth';
import { formatAddress, formatDateTime, formatUsd } from '../../shared/format';
import { useWindowRefetch } from '../../shared/refetch';

const { Text, Paragraph } = Typography;
const evmAddressPattern = /^0x[a-fA-F0-9]{40}$/;
const assetPrecisionMap: Record<string, number> = {
  USDC: 6,
};

interface TransferState {
  balances: BalanceItem[];
  transfers: TransferItem[];
}

function formatTransferDirection(direction: TransferItem['direction']): string {
  switch (direction) {
    case 'IN':
      return '转入';
    case 'OUT':
      return '转出';
    case 'SELF':
      return '自转';
    default:
      return '未知';
  }
}

export function TransferPage() {
  const [form] = Form.useForm<TransferRequest>();
  const { message } = AntdApp.useApp();
  const { session } = useAuth();
  const [state, setState] = useState<TransferState | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const selectedAsset = Form.useWatch('asset', form) || 'USDC';

  async function loadData(background = false) {
    if (!session) {
      setState({ balances: [], transfers: [] });
      setLoading(false);
      setRefreshing(false);
      setError(null);
      return;
    }
    if (background && state) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);

    try {
      const [balances, transfers] = await Promise.all([api.account.getBalances(), api.transfers.getHistory()]);
      setState({ balances, transfers });
    } catch (loadError) {
      setError(loadError);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    void loadData();
  }, [session]);

  useWindowRefetch(() => {
    void loadData(true);
  }, !!state);

  const transferableAssets = useMemo(() => {
    const assetSet = new Set(['USDC']);
    (state?.balances || [])
      .filter((item) => item.account_code === 'USER_WALLET')
      .forEach((item) => assetSet.add(item.asset));
    return Array.from(assetSet.values());
  }, [state]);

  const availableBalance = useMemo(() => {
    const balance = state?.balances.find((item) => item.account_code === 'USER_WALLET' && item.asset === selectedAsset);
    return balance?.balance || '0';
  }, [selectedAsset, state]);

  async function handleSubmit(values: TransferRequest) {
    if (!session) {
      message.warning('请先登录后再发起内部转账');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await api.transfers.create(values);
      await loadData(true);
      form.resetFields(['amount']);
      message.success('内部转账已提交并入账');
    } catch (submitError) {
      setError(submitError);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rg-app-page rg-app-page--wallet-transfer">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow="Wallet"
          title="Transfer"
          description="账户内转会实时落账到统一账本，不收手续费，但仍受余额校验、幂等和审计约束。"
          titleEffect="shiny"
          descriptionEffect="proximity"
          extra={
            <Button onClick={() => void loadData(true)} loading={refreshing}>
              刷新状态
            </Button>
          }
        />

        <Alert
          showIcon
          type="info"
          message="内部转账说明"
          description="内部转账仅支持已注册地址之间的同资产划转，默认实时到账且不收费。自转账允许，但同样会走账本与审计链路。"
        />

        {loading ? <Spin size="large" /> : null}
        <ErrorAlert error={error} />
        {!session ? <LoginRequiredCard title="登录后发起内部转账" description="内部转账需要登录后查询余额、填写收款地址并提交。" /> : null}

        {state ? (
          <TwoColumnRow
            left={
              <Card className="surface-card" title="Create Transfer">
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                  <Text strong>{selectedAsset} 可转账余额: {formatUsd(availableBalance, 8)}</Text>
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    仅 `USER_WALLET` 余额可用于内部转账，挂单保证金、持仓保证金和提现冻结余额不会参与本次划转。
                  </Paragraph>
                  <Form form={form} layout="vertical" initialValues={{ asset: 'USDC' }} onFinish={handleSubmit}>
                    <Form.Item label="资产" name="asset" rules={[{ required: true, message: '请选择资产' }]}>
                      <Select
                        options={transferableAssets.map((asset) => ({
                          label: asset,
                          value: asset,
                        }))}
                      />
                    </Form.Item>

                    <Form.Item
                      label="收款地址"
                      name="to_address"
                      rules={[
                        { required: true, message: '请输入收款地址' },
                        {
                          validator: (_, value) => {
                            const address = String(value || '').trim();
                            if (!evmAddressPattern.test(address)) {
                              return Promise.reject(new Error('请输入合法 EVM 地址'));
                            }
                            return Promise.resolve();
                          },
                        },
                      ]}
                    >
                      <Input placeholder="0x..." autoComplete="off" />
                    </Form.Item>

                    <Form.Item
                      label="数量"
                      name="amount"
                      rules={[
                        { required: true, message: '请输入转账数量' },
                        {
                          validator: (_, value) => {
                            const amount = String(value || '').trim();
                            if (!/^\d+(\.\d+)?$/.test(amount)) {
                              return Promise.reject(new Error('请输入合法数量'));
                            }
                            const decimals = amount.split('.')[1]?.length || 0;
                            const precision = assetPrecisionMap[selectedAsset] ?? 18;
                            if (decimals > precision) {
                              return Promise.reject(new Error(`最多支持 ${precision} 位小数`));
                            }
                            const numericAmount = Number(amount);
                            if (!Number.isFinite(numericAmount) || numericAmount <= 0) {
                              return Promise.reject(new Error('转账数量必须大于 0'));
                            }
                            if (numericAmount > Number(availableBalance)) {
                              return Promise.reject(new Error('转账数量不能超过可转账余额'));
                            }
                            return Promise.resolve();
                          },
                        },
                      ]}
                    >
                      <Input inputMode="decimal" placeholder="100.00" />
                    </Form.Item>

                    <Button type="primary" htmlType="submit" loading={submitting} block>
                      Confirm Transfer
                    </Button>
                  </Form>
                </Space>
              </Card>
            }
            right={
              <Card className="table-card" title="Recent Transfers">
                {state.transfers.length === 0 ? (
                  <EmptyStateCard title="暂无内部转账记录" description="提交成功后，这里会展示最近的转账记录。" />
                ) : (
                  <Table
                    rowKey="transfer_id"
                    dataSource={state.transfers.slice(0, 10)}
                    pagination={false}
                    scroll={{ x: 760 }}
                    columns={[
                      { title: 'Time', dataIndex: 'created_at', width: 180, render: (value: string) => formatDateTime(value) },
                      { title: 'Direction', dataIndex: 'direction', width: 90, render: (value: TransferItem['direction']) => formatTransferDirection(value) },
                      { title: 'Counterparty', dataIndex: 'counterparty_address', render: (value: string) => <Text type="secondary">{value ? formatAddress(value, 8) : '--'}</Text> },
                      { title: 'Asset', dataIndex: 'asset', width: 90 },
                      { title: 'Amount', dataIndex: 'amount', align: 'right', render: (value: string) => formatUsd(value, 8) },
                      { title: 'Status', dataIndex: 'status', width: 110, render: (value: string) => <StatusTag value={value} /> },
                    ]}
                  />
                )}
              </Card>
            }
          />
        ) : null}
      </Space>
    </div>
  );
}
