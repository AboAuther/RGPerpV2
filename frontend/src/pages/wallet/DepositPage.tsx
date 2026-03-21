import { EyeInvisibleOutlined, EyeOutlined } from '@ant-design/icons';
import { App as AntdApp, Alert, Button, Card, Col, Row, Space, Spin, Table, Typography } from 'antd';
import { BrowserProvider, Contract, parseUnits } from 'ethers';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../../shared/api';
import { useAuth } from '../../shared/auth';
import { ErrorAlert, PageIntro, StatusTag } from '../../shared/components';
import type { DepositAddressItem, DepositItem } from '../../shared/domain';
import { appConfig } from '../../shared/env';
import { formatAddress, formatChainName, formatDateTime, formatUsd } from '../../shared/format';
import { useWindowRefetch } from '../../shared/refetch';

const { Paragraph, Text } = Typography;

const mockUsdcAbi = [
  'function mint(address to, uint256 amount)',
  'function transfer(address to, uint256 amount) returns (bool)',
];

interface DepositState {
  addresses: DepositAddressItem[];
  deposits: DepositItem[];
}

export function DepositPage() {
  const { message } = AntdApp.useApp();
  const { session } = useAuth();
  const [state, setState] = useState<DepositState | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [revealed, setRevealed] = useState<Record<number, boolean>>({});
  const [generatingChain, setGeneratingChain] = useState<number | null>(null);
  const [mintingChain, setMintingChain] = useState<number | null>(null);
  const [fundingNativeChain, setFundingNativeChain] = useState<number | null>(null);
  const [quickDepositingChain, setQuickDepositingChain] = useState<number | null>(null);

  async function loadData(background = false) {
    if (background && state) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);

    try {
      const [addresses, deposits] = await Promise.all([api.wallet.getDepositAddresses(), api.wallet.getDeposits()]);
      setState({ addresses, deposits });
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

  const addressMap = useMemo(() => {
    const map = new Map<number, DepositAddressItem>();
    for (const item of state?.addresses || []) {
      map.set(item.chain_id, item);
    }
    return map;
  }, [state?.addresses]);

  async function handleCopyAddress(address: string) {
    try {
      await navigator.clipboard.writeText(address);
      message.success('充值地址已复制');
    } catch {
      message.error('复制失败，请手动复制');
    }
  }

  async function ensureWalletOnChain(chainId: number) {
    if (!window.ethereum) {
      throw new Error('未检测到 MetaMask 或兼容钱包');
    }

    const currentHex = (await window.ethereum.request({ method: 'eth_chainId' })) as string;
    const currentChainId = Number.parseInt(currentHex, 16);
    if (currentChainId === chainId) {
      return;
    }

    await window.ethereum.request({
      method: 'wallet_switchEthereumChain',
      params: [{ chainId: `0x${chainId.toString(16)}` }],
    });
  }

  async function getUsdcContract(chainId: number) {
    if (!window.ethereum) {
      throw new Error('未检测到 MetaMask 或兼容钱包');
    }
    if (!appConfig.localUsdcAddress) {
      throw new Error('本地测试 USDC 合约地址未配置');
    }

    await ensureWalletOnChain(chainId);
    const provider = new BrowserProvider(window.ethereum);
    const signer = await provider.getSigner();
    if (session && signer.address.toLowerCase() !== session.user.evm_address.toLowerCase()) {
      throw new Error('当前 MetaMask 地址与已登录地址不一致，请切回已登录钱包后重试');
    }
    return new Contract(appConfig.localUsdcAddress, mockUsdcAbi, signer);
  }

  async function handleGenerateAddress(chainId: number) {
    try {
      setGeneratingChain(chainId);
      const next = await api.wallet.generateDepositAddress(chainId);
      setState((prev) => ({
        addresses: [...(prev?.addresses || []).filter((item) => item.chain_id !== chainId), next].sort((a, b) => a.chain_id - b.chain_id),
        deposits: prev?.deposits || [],
      }));
      setRevealed((current) => ({ ...current, [chainId]: true }));
      message.success('充值地址已生成');
    } catch (generateError) {
      setError(generateError);
    } finally {
      setGeneratingChain(null);
    }
  }

  async function handleMintNative(chainId: number) {
    try {
      setFundingNativeChain(chainId);
      await api.wallet.requestLocalNativeFaucet(chainId);
      message.success('已发放测试 ETH，请在钱包中确认余额');
    } catch (mintError) {
      setError(mintError);
    } finally {
      setFundingNativeChain(null);
    }
  }

  async function handleMintUsdc(chainId: number) {
    if (!session) {
      return;
    }
    try {
      setMintingChain(chainId);
      const usdc = await getUsdcContract(chainId);
      const tx = await usdc.mint(session.user.evm_address, parseUnits('1000', 6));
      await tx.wait();
      message.success('已向当前钱包 mint 1000 USDC');
    } catch (mintError) {
      setError(mintError);
    } finally {
      setMintingChain(null);
    }
  }

  async function handleQuickDeposit(chainId: number, address: string) {
    try {
      setQuickDepositingChain(chainId);
      const usdc = await getUsdcContract(chainId);
      const tx = await usdc.transfer(address, parseUnits('1000', 6));
      await tx.wait();
      message.success('链上转账已提交，等待 Indexer 入账');
      await new Promise((resolve) => window.setTimeout(resolve, 2500));
      await loadData(true);
    } catch (depositError) {
      setError(depositError);
    } finally {
      setQuickDepositingChain(null);
    }
  }

  return (
    <div className="rg-app-page rg-app-page--deposit">
      <Space direction="vertical" size={20} style={{ width: '100%' }}>
        <PageIntro
          eyebrow="Wallet"
          title="Deposit"
          description="充值地址按链、按需生成。前端默认隐藏完整地址；只有用户主动生成并展开后才展示。所有链的到账判定一致，只有 CREDITED 才表示链下账本已入账。"
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
          message="充值状态说明"
          description="DETECTED 表示已检测到链上转账，CONFIRMING 表示确认中，CREDITED 才代表账本已记账。Local Anvil 额外提供领取测试 ETH、领取测试 USDC 和便捷充值入口，便于本地联调。"
        />

        {loading ? <Spin size="large" /> : null}
        <ErrorAlert error={error} />

        <Row gutter={[16, 16]}>
          {appConfig.supportedChains.map((chain) => {
            const item = addressMap.get(chain.id);
            const addressVisible = !!revealed[chain.id];
            const displayAddress = item ? (addressVisible ? item.address : formatAddress(item.address, 6)) : '未生成';
            const isLocalChain = chain.id === appConfig.localChainId;

            return (
              <Col xs={24} md={12} xl={8} key={chain.id}>
                <Card className="surface-card" title={`${chain.name} / USDC`} extra={<StatusTag value={item ? 'ACTIVE' : 'PENDING'} />}>
                  <Space direction="vertical" size={12} style={{ width: '100%' }}>
                    <Text strong>充值地址</Text>
                    <Text code>{displayAddress}</Text>
                    <Text type="secondary">确认数要求: {item?.confirmations ?? chain.confirmations}</Text>
                    <Space wrap>
                      {item ? (
                        <>
                          <Button
                            icon={addressVisible ? <EyeInvisibleOutlined /> : <EyeOutlined />}
                            onClick={() => setRevealed((current) => ({ ...current, [chain.id]: !current[chain.id] }))}
                          >
                            {addressVisible ? '隐藏地址' : '显示地址'}
                          </Button>
                          <Button onClick={() => void handleCopyAddress(item.address)}>复制地址</Button>
                        </>
                      ) : (
                        <Button type="primary" loading={generatingChain === chain.id} onClick={() => void handleGenerateAddress(chain.id)}>
                          生成充值地址
                        </Button>
                      )}
                    </Space>
                    {isLocalChain ? (
                      <Space wrap>
                        <Button loading={fundingNativeChain === chain.id} onClick={() => void handleMintNative(chain.id)}>
                          领取测试 ETH
                        </Button>
                        <Button loading={mintingChain === chain.id} onClick={() => void handleMintUsdc(chain.id)}>
                          领取测试 USDC
                        </Button>
                        <Button
                          type="primary"
                          ghost
                          disabled={!item}
                          loading={quickDepositingChain === chain.id}
                          onClick={() => item && void handleQuickDeposit(chain.id, item.address)}
                        >
                          便捷充值 1000 USDC
                        </Button>
                      </Space>
                    ) : null}
                    <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                      每条链的充值流程一致：用户获取该链充值地址后，可从任意交易所或钱包向该地址转账。测试阶段仅 Local Anvil 提供便捷工具按钮。
                    </Paragraph>
                  </Space>
                </Card>
              </Col>
            );
          })}
        </Row>

        {state ? (
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
        ) : null}
      </Space>
    </div>
  );
}
