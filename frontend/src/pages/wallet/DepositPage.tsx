import {
  App as AntdApp,
  Alert,
  Button,
  Card,
  Col,
  Row,
  Space,
  Spin,
  Table,
  Typography,
} from 'antd';
import { useEffect, useState } from 'react';
import { api } from '../../shared/api';
import { useAuth } from '../../shared/auth';
import { ErrorAlert, PageIntro, StatusTag } from '../../shared/components';
import type { DepositAddressItem, DepositItem } from '../../shared/domain';
import { appConfig } from '../../shared/env';
import { formatAddress, formatChainName, formatDateTime, formatUsd } from '../../shared/format';

const { Paragraph, Text } = Typography;

interface DepositState {
  addresses: DepositAddressItem[];
  deposits: DepositItem[];
}

export function DepositPage() {
  const { message } = AntdApp.useApp();
  const { session } = useAuth();
  const [state, setState] = useState<DepositState | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [faucetChain, setFaucetChain] = useState<number | null>(null);

  useEffect(() => {
    let active = true;

    async function load() {
      setLoading(true);
      setError(null);

      try {
        const [addresses, deposits] = await Promise.all([api.wallet.getDepositAddresses(), api.wallet.getDeposits()]);
        if (active) {
          setState({ addresses, deposits });
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

  async function handleCopyAddress(address: string) {
    try {
      await navigator.clipboard.writeText(address);
      message.success('充值地址已复制');
    } catch {
      message.error('复制失败，请手动复制');
    }
  }

  async function handleFaucet(chainId: number) {
    if (!session) {
      return;
    }

    try {
      setFaucetChain(chainId);
      await api.wallet.requestFaucet({
        address: session.user.evm_address,
        chainId,
      });
      const [addresses, deposits] = await Promise.all([api.wallet.getDepositAddresses(), api.wallet.getDeposits()]);
      setState({ addresses, deposits });
      message.success('已发起 review faucet');
    } catch (faucetError) {
      setError(faucetError);
    } finally {
      setFaucetChain(null);
    }
  }

  return (
    <Space direction="vertical" size={20} style={{ width: '100%' }}>
      <PageIntro
        eyebrow="Wallet"
        title="Deposit"
        description="每条链独立地址、确认数和到账状态必须显式展示。前端不会把检测到视为已到账，只有 CREDITED 才表示链下入账完成。"
      />

      <Alert
        showIcon
        type="info"
        message="充值状态说明"
        description="DETECTED 表示已检测到链上转账，CONFIRMING 表示确认中，CREDITED 才代表账本已记账。若发生重组回滚，状态应转为 REORGED。"
      />

      {loading ? <Spin size="large" /> : null}
      <ErrorAlert error={error} />

      {state ? (
        <>
          <Row gutter={[16, 16]}>
            {state.addresses.map((item) => (
              <Col xs={24} md={12} xl={8} key={item.chain_id}>
                <Card
                  className="surface-card"
                  title={`${formatChainName(item.chain_id)} / ${item.asset}`}
                  extra={<StatusTag value="ACTIVE" />}
                >
                  <Space direction="vertical" size={12} style={{ width: '100%' }}>
                    <Text strong>充值地址</Text>
                    <Text copyable={{ text: item.address }} code>
                      {item.address}
                    </Text>
                    <Text type="secondary">确认数要求: {item.confirmations}</Text>
                    <Space wrap>
                      <Button onClick={() => handleCopyAddress(item.address)}>复制地址</Button>
                      {appConfig.appEnv === 'review' && appConfig.reviewFaucetEnabled ? (
                        <Button
                          type="primary"
                          ghost
                          loading={faucetChain === item.chain_id}
                          onClick={() => handleFaucet(item.chain_id)}
                        >
                          faucet 测试资金
                        </Button>
                      ) : null}
                    </Space>
                    <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                      每链地址独立；仅支持白名单 USDC 资产。若链配置变更，前端仍以后端返回地址为准。
                    </Paragraph>
                  </Space>
                </Card>
              </Col>
            ))}
          </Row>

          <Card className="table-card" title="Recent Deposits">
            <Table
              rowKey="deposit_id"
              dataSource={state.deposits}
              scroll={{ x: 980 }}
              pagination={false}
              columns={[
                { title: 'Time', dataIndex: 'detected_at', render: (value: string) => formatDateTime(value), width: 180 },
                { title: 'Chain', dataIndex: 'chain_id', render: (value: number) => formatChainName(value), width: 120 },
                { title: 'Asset', dataIndex: 'asset', width: 90 },
                { title: 'Amount', dataIndex: 'amount', align: 'right', render: (value: string) => formatUsd(value) },
                {
                  title: 'Confirmations',
                  render: (_, record) => `${record.confirmations}/${record.required_confirmations}`,
                  width: 130,
                },
                { title: 'Status', dataIndex: 'status', width: 140, render: (value: string) => <StatusTag value={value} /> },
                {
                  title: 'Address',
                  dataIndex: 'address',
                  render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text>,
                },
                {
                  title: 'Tx Hash',
                  dataIndex: 'tx_hash',
                  render: (value: string) => <Text type="secondary">{formatAddress(value, 8)}</Text>,
                },
              ]}
            />
          </Card>
        </>
      ) : null}
    </Space>
  );
}
