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
import { useEffect, useMemo, useState } from 'react';
import { api } from '../../shared/api';
import { ErrorAlert, PageIntro, StatusTag, TwoColumnRow } from '../../shared/components';
import type { BalanceItem, WithdrawItem, WithdrawRequest } from '../../shared/domain';
import { appConfig } from '../../shared/env';
import { formatAddress, formatChainName, formatDateTime, formatUsd } from '../../shared/format';
import { useWindowRefetch } from '../../shared/refetch';

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
  const [state, setState] = useState<WithdrawState | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const selectedAsset = Form.useWatch('asset', form) || 'USDC';

  async function loadData(background = false) {
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
  }, []);

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

  async function handleSubmit(values: WithdrawRequest) {
    setSubmitting(true);
    setError(null);

    try {
      await api.wallet.createWithdrawal(values);
      await loadData(true);
      form.resetFields(['amount', 'to_address']);
      message.success('提现已申请，请等待审核与链上状态推进');
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
        description="REQUESTED / HOLD / MANUAL_REVIEW / SIGNING / CONFIRMING / COMPLETED / REFUNDED 对应不同链路阶段。前端不得把已申请视为已完成。"
      />

      {loading ? <Spin size="large" /> : null}
      <ErrorAlert error={error} />

      {state ? (
        <TwoColumnRow
          left={
            <Card className="surface-card" title="Create Withdrawal">
              <Space direction="vertical" size={16} style={{ width: '100%' }}>
                <Text strong>
                  {selectedAsset} 可提现余额: {formatUsd(availableBalance)}
                </Text>
                <Form
                  form={form}
                  layout="vertical"
                  initialValues={{ asset: 'USDC', chain_id: appConfig.supportedChains[0]?.id }}
                  onFinish={handleSubmit}
                >
                  <Form.Item label="链" name="chain_id" rules={[{ required: true, message: '请选择链' }]}>
                    <Select
                      options={appConfig.supportedChains.map((chain) => ({
                        label: `${chain.name} (${chain.id})`,
                        value: chain.id,
                      }))}
                    />
                  </Form.Item>

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

                  <Button type="primary" htmlType="submit" loading={submitting}>
                    提交提现申请
                  </Button>
                </Form>
              </Space>
            </Card>
          }
          right={
            <Card className="surface-card" title="Rules">
              <Space direction="vertical" size={12}>
                <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                  提现手续费、单笔限额、日限额和人工审核阈值以后端配置与服务端校验为准，前端不再写死展示数字。
                </Paragraph>
                <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                  当前表单只做 UX 校验：资产白名单、余额上限、精度和地址格式会先在前端拦截，但最终可提现性仍以后端为准。
                </Paragraph>
                <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                  若提现失败，最终状态应回到 `REFUNDED`，而不是停留在模糊的“处理中”。
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
              { title: 'Chain', dataIndex: 'chain_id', render: (value: number) => formatChainName(value), width: 110 },
              { title: 'Asset', dataIndex: 'asset', width: 90 },
              { title: 'Amount', dataIndex: 'amount', align: 'right', render: (value: string) => formatUsd(value) },
              { title: 'Fee', dataIndex: 'fee_amount', align: 'right', render: (value: string) => formatUsd(value) },
              { title: 'Status', dataIndex: 'status', render: (value: string) => <StatusTag value={value} />, width: 150 },
              {
                title: 'To Address',
                dataIndex: 'to_address',
                render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text>,
              },
              {
                title: 'Tx Hash',
                dataIndex: 'tx_hash',
                render: (value: string | null | undefined) => <Text type="secondary">{formatAddress(value, 8)}</Text>,
              },
            ]}
          />
        </Card>
      ) : null}
      </Space>
    </div>
  );
}
