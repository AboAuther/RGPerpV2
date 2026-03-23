import {
  App as AntdApp,
  Alert,
  Button,
  Card,
  Form,
  Input,
  Select,
  Space,
  Spin,
  Table,
  Typography,
} from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../../shared/api';
import { EmptyStateCard, ErrorAlert, LoginRequiredCard, PageIntro, StatusTag, TwoColumnRow } from '../../shared/components';
import type { BalanceItem, WithdrawItem, WithdrawRequest } from '../../shared/domain';
import { formatAddress, formatChainName, formatDateTime, formatUsd } from '../../shared/format';
import { useAuth } from '../../shared/auth';
import { useWindowRefetch } from '../../shared/refetch';
import { useSystemConfig } from '../../shared/system';

const { Paragraph, Text } = Typography;
const evmAddressPattern = /^0x[a-fA-F0-9]{40}$/;
const assetPrecisionMap: Record<string, number> = {
  USDC: 6,
};

interface WithdrawState {
  balances: BalanceItem[];
  withdrawals: WithdrawItem[];
}

export function WithdrawPage() {
  const [form] = Form.useForm<WithdrawRequest>();
  const { message } = AntdApp.useApp();
  const { session } = useAuth();
  const { chains, loading: chainsLoading, error: chainsError } = useSystemConfig();
  const [state, setState] = useState<WithdrawState | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const selectedAsset = Form.useWatch('asset', form) || 'USDC';

  async function loadData(background = false) {
    if (!session) {
      setState({ balances: [], withdrawals: [] });
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
      const [balances, withdrawals] = await Promise.all([api.account.getBalances(), api.wallet.getWithdrawals()]);
      setState({ balances, withdrawals });
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

  useEffect(() => {
    if (!form.getFieldValue('chain_id') && chains[0]) {
      form.setFieldValue('chain_id', chains[0].chain_id);
    }
  }, [chains, form]);

  useEffect(() => {
    if (!session?.user.evm_address) {
      return;
    }
    if (!form.getFieldValue('to_address')) {
      form.setFieldValue('to_address', session.user.evm_address);
    }
  }, [form, session?.user.evm_address]);

  useWindowRefetch(() => {
    void loadData(true);
  }, !!state);

  const withdrawableAssets = useMemo(() => {
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

  const withdrawHoldBalance = useMemo(() => {
    const balance = state?.balances.find((item) => item.account_code === 'USER_WITHDRAW_HOLD' && item.asset === selectedAsset);
    return balance?.balance || '0';
  }, [selectedAsset, state]);

  async function handleCopy(value: string, successMessage: string) {
    try {
      await navigator.clipboard.writeText(value);
      message.success(successMessage);
    } catch {
      message.error('复制失败，请手动复制');
    }
  }

  async function handleSubmit(values: WithdrawRequest) {
    if (!session) {
      message.warning('请先登录后再提交提现申请');
      return;
    }
    setSubmitting(true);
    setError(null);

    try {
      await api.wallet.createWithdrawal(values);
      await loadData(true);
      form.resetFields(['amount']);
      form.setFieldValue('to_address', session?.user.evm_address || values.to_address);
      message.success('提现申请已提交，资金已冻结。常规提现会自动广播，只有命中特殊风控时才进入人工复核。');
    } catch (submitError) {
      setError(submitError);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rg-app-page rg-app-page--withdraw">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow="Wallet"
          title="Withdraw"
          description="提交成功仅代表已申请，不代表提现完成。冻结、审核、待签名、链上确认和失败退款必须逐态展示。"
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
          type="warning"
          message="提现状态说明"
          description="HOLD 表示申请已提交并等待处理；APPROVED 表示审核通过；BROADCASTED / CONFIRMING / COMPLETED 表示提现已进入链上处理阶段。"
        />

        {loading || chainsLoading ? <Spin size="large" /> : null}
        <ErrorAlert error={chainsError || error} />
        {!session ? <LoginRequiredCard title="登录后发起提现" description="提现页允许未登录浏览，但账户余额、提现历史和提交提现申请都需要登录后才能使用。" /> : null}

        {state ? (
          <TwoColumnRow
            left={
              <Card className="surface-card" title="Create Withdrawal">
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                  <Text strong>{selectedAsset} 可提现余额: {formatUsd(availableBalance)}</Text>
                  <Text type="secondary">已冻结待处理: {formatUsd(withdrawHoldBalance)}</Text>
                  {chains.length === 0 ? (
                    <EmptyStateCard title="暂无可提现链" description="当前暂时没有可用的提现链，请稍后再试。" />
                  ) : null}
                  <Form form={form} layout="vertical" initialValues={{ asset: 'USDC' }} onFinish={handleSubmit}>
                    <Form.Item label="链" name="chain_id" rules={[{ required: true, message: '请选择链' }]}>
                      <Select
                        options={chains.map((chain) => ({
                          label: `${chain.name} (${chain.chain_id})`,
                          value: chain.chain_id,
                          disabled: !chain.withdraw_enabled,
                        }))}
                        disabled={chains.length === 0}
                      />
                    </Form.Item>
                    <Text type="secondary">仅支持当前可用的提现链，暂不可用的链会显示为禁用。</Text>

                    <Form.Item
                      label="资产"
                      name="asset"
                      rules={[
                        { required: true, message: '请选择资产' },
                        {
                          validator: (_, value) =>
                            withdrawableAssets.includes(value)
                              ? Promise.resolve()
                              : Promise.reject(new Error('当前资产不可提现')),
                        },
                      ]}
                    >
                      <Select
                        options={withdrawableAssets.map((asset) => ({
                          label: asset,
                          value: asset,
                        }))}
                      />
                    </Form.Item>

                    <Form.Item
                      label="数量"
                      name="amount"
                      rules={[
                        { required: true, message: '请输入提现数量' },
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
                              return Promise.reject(new Error('提现数量必须大于 0'));
                            }

                            if (numericAmount > Number(availableBalance)) {
                              return Promise.reject(new Error('提现数量不能超过可提现余额'));
                            }

                            return Promise.resolve();
                          },
                        },
                      ]}
                    >
                      <Input placeholder="100.00" />
                    </Form.Item>

                    <Form.Item
                      label="目标地址"
                      name="to_address"
                      extra={session?.user.evm_address ? '默认使用当前登录钱包地址，可手动修改。' : undefined}
                      rules={[
                        { required: true, message: '请输入目标地址' },
                        {
                          validator: (_, value) =>
                            evmAddressPattern.test(String(value || '').trim())
                              ? Promise.resolve()
                              : Promise.reject(new Error('请输入合法的 EVM 地址')),
                        },
                      ]}
                    >
                      <Input placeholder="0x..." />
                    </Form.Item>

                    {session?.user.evm_address ? (
                      <Button type="default" onClick={() => form.setFieldValue('to_address', session.user.evm_address)}>
                        使用当前钱包地址
                      </Button>
                    ) : null}

                    <Button type="primary" htmlType="submit" loading={submitting} disabled={chains.length === 0}>
                      提交提现申请
                    </Button>
                    {!session ? (
                      <Text type="secondary">未登录时仅可浏览提现流程说明，提交操作会被拦截。</Text>
                    ) : null}
                  </Form>
                </Space>
              </Card>
            }
            right={
              <Card className="surface-card" title="Rules">
                <Space direction="vertical" size={12}>
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    提现会根据链路状态、金额和风控规则进入自动处理或人工审核流程。
                  </Paragraph>
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    提交前请确认链、资产、金额和目标地址正确无误。
                  </Paragraph>
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    审核通过后，提现会继续进入广播与确认阶段。
                  </Paragraph>
                  <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                    如遇链路拥堵或异常，到账时间可能延长。
                  </Paragraph>
                </Space>
              </Card>
            }
          />
        ) : null}

        {state ? (
          <Card className="table-card" title="Withdrawal History">
            <Table
              rowKey="withdraw_id"
              dataSource={state.withdrawals}
              scroll={{ x: 980 }}
              pagination={false}
              columns={[
                { title: 'Time', dataIndex: 'created_at', render: (value: string) => formatDateTime(value), width: 180 },
                { title: 'Chain', dataIndex: 'chain_id', render: (value: number) => formatChainName(value, chains), width: 110 },
                { title: 'Asset', dataIndex: 'asset', width: 90 },
                { title: 'Amount', dataIndex: 'amount', align: 'right', render: (value: string) => formatUsd(value) },
                { title: 'Fee', dataIndex: 'fee_amount', align: 'right', render: (value: string) => formatUsd(value) },
                { title: 'Status', dataIndex: 'status', render: (value: string) => <StatusTag value={value} />, width: 150 },
                {
                  title: 'To Address',
                  dataIndex: 'to_address',
                  render: (value: string) => (
                    <Space size={4}>
                      <Text type="secondary">{formatAddress(value, 8)}</Text>
                      <Button type="text" size="small" icon={<CopyOutlined />} onClick={() => void handleCopy(value, '地址已复制')} />
                    </Space>
                  ),
                },
                {
                  title: 'Tx Hash',
                  dataIndex: 'tx_hash',
                  render: (value: string | null | undefined) =>
                    value ? (
                      <Space size={4}>
                        <Text type="secondary">{formatAddress(value, 8)}</Text>
                        <Button type="text" size="small" icon={<CopyOutlined />} onClick={() => void handleCopy(value, '交易哈希已复制')} />
                      </Space>
                    ) : (
                      <Text type="secondary">--</Text>
                    ),
                },
              ]}
            />
          </Card>
        ) : null}
      </Space>
    </div>
  );
}
