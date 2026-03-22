import { CopyOutlined, EyeInvisibleOutlined, EyeOutlined } from '@ant-design/icons';
import { App as AntdApp, Alert, Button, Card, Input, Select, Space, Spin, Table, Typography } from 'antd';
import { BrowserProvider, Contract, parseUnits } from 'ethers';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../../shared/api';
import { useAuth } from '../../shared/auth';
import { EmptyStateCard, ErrorAlert, LoginRequiredCard, PageIntro, StatusTag } from '../../shared/components';
import type { DepositAddressItem, DepositItem } from '../../shared/domain';
import { formatAddress, formatChainName, formatDateTime, formatUsd } from '../../shared/format';
import { useWindowRefetch } from '../../shared/refetch';
import { useSystemConfig } from '../../shared/system';

const { Paragraph, Text } = Typography;

const mintableUsdcAbi = [
  'function mint(address to, uint256 amount)',
  'function transfer(address to, uint256 amount) returns (bool)',
];

const depositRouterAbi = ['function forward()', 'function token() view returns (address)'];

interface DepositState {
  addresses: DepositAddressItem[];
  deposits: DepositItem[];
}

export function DepositPage() {
  const { message } = AntdApp.useApp();
  const { session } = useAuth();
  const { chains, loading: chainsLoading, error: chainsError, localChain } = useSystemConfig();
  const [state, setState] = useState<DepositState | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [revealed, setRevealed] = useState<Record<number, boolean>>({});
  const [generatingChain, setGeneratingChain] = useState<number | null>(null);
  const [mintingChain, setMintingChain] = useState<number | null>(null);
  const [fundingNativeChain, setFundingNativeChain] = useState<number | null>(null);
  const [quickDepositingChain, setQuickDepositingChain] = useState<number | null>(null);
  const [selectedChainId, setSelectedChainId] = useState<number | null>(null);
  const [depositAmount, setDepositAmount] = useState('1000');

  async function loadData(background = false) {
    if (!session) {
      setState({ addresses: [], deposits: [] });
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
  }, [session]);

  useWindowRefetch(() => {
    void loadData(true);
  }, !!state);

  useEffect(() => {
    if (chains.length === 0) {
      setSelectedChainId(null);
      return;
    }
    const fallbackChainID = localChain?.chain_id ?? chains[0]?.chain_id ?? null;
    if (selectedChainId == null || !chains.some((chain) => chain.chain_id === selectedChainId)) {
      setSelectedChainId(fallbackChainID);
    }
  }, [chains, localChain, selectedChainId]);

  const addressMap = useMemo(() => {
    const map = new Map<number, DepositAddressItem>();
    for (const item of state?.addresses || []) {
      map.set(item.chain_id, item);
    }
    return map;
  }, [state?.addresses]);

  const selectedChain = useMemo(
    () => (selectedChainId == null ? undefined : chains.find((chain) => chain.chain_id === selectedChainId)),
    [chains, selectedChainId],
  );
  const selectedAddress = selectedChain ? addressMap.get(selectedChain.chain_id) : undefined;
  const selectedAddressVisible = !!(selectedChain && revealed[selectedChain.chain_id]);
  const selectedDisplayAddress =
    selectedAddress && selectedChain
      ? selectedAddressVisible
        ? selectedAddress.address
        : formatAddress(selectedAddress.address, 6)
      : '未生成';
  const isLocalSelectedChain = !!selectedChain?.local_testnet;
  const normalizedDepositAmount = String(depositAmount || '').trim();
  const depositAmountError = useMemo(() => {
    if (normalizedDepositAmount === '') {
      return '请输入金额';
    }
    if (!/^\d+(\.\d{1,6})?$/.test(normalizedDepositAmount)) {
      return '请输入合法 USDC 金额，最多 6 位小数';
    }
    const amount = Number(normalizedDepositAmount);
    if (!Number.isFinite(amount) || amount <= 0) {
      return '金额必须大于 0';
    }
    return null;
  }, [normalizedDepositAmount]);

  async function handleCopy(value: string, successMessage: string) {
    try {
      await navigator.clipboard.writeText(value);
      message.success(successMessage);
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

  async function getChainSigner(chainId: number) {
    if (!window.ethereum) {
      throw new Error('未检测到 MetaMask 或兼容钱包');
    }

    await ensureWalletOnChain(chainId);
    const provider = new BrowserProvider(window.ethereum);
    const signer = await provider.getSigner();
    if (session && signer.address.toLowerCase() !== session.user.evm_address.toLowerCase()) {
      throw new Error('当前 MetaMask 地址与已登录地址不一致，请切回已登录钱包后重试');
    }
    return signer;
  }

  async function getUsdcContract(chainId: number, usdcAddress: string) {
    const signer = await getChainSigner(chainId);
    return new Contract(usdcAddress, mintableUsdcAbi, signer);
  }

  async function getValidatedRouterContract(chainId: number, address: string, usdcAddress: string) {
    const signer = await getChainSigner(chainId);
    const provider = signer.provider;
    if (!provider) {
      throw new Error('钱包 provider 不可用');
    }
    const code = await provider.getCode(address);
    if (!code || code === '0x') {
      throw new Error('当前充值地址不是有效 Router 合约，已阻止便捷充值');
    }
    const router = new Contract(address, depositRouterAbi, signer);
    const routerToken = String(await router.token()).toLowerCase();
    if (routerToken !== usdcAddress.toLowerCase()) {
      throw new Error('当前充值地址对应的 Router 资产与当前链配置不一致，已阻止便捷充值');
    }
    return router;
  }

  async function waitForDepositRecord(chainId: number, address: string, txHash: string) {
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
      const deposits = await api.wallet.getDeposits();
      const matched = deposits.find(
        (item) =>
          item.chain_id === chainId &&
          item.address.toLowerCase() === address.toLowerCase() &&
          item.tx_hash.toLowerCase() === txHash.toLowerCase(),
      );
      if (matched) {
        return matched;
      }
      await new Promise((resolve) => window.setTimeout(resolve, 1000));
    }
    throw new Error('forward 交易已提交，但超时仍未看到新的充值记录，请检查 Indexer 与链上事件');
  }

  async function handleGenerateAddress(chainId: number) {
    if (!session) {
      message.warning('请先登录后再生成充值地址');
      return;
    }
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
    if (!session) {
      message.warning('请先登录后再领取测试 ETH');
      return;
    }
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

  async function handleMintUsdc(chainId: number, usdcAddress?: string | null) {
    if (!session) {
      message.warning('请先登录后再领取测试 USDC');
      return;
    }
    if (!usdcAddress) {
      setError(new Error('当前链未配置 USDC 合约地址'));
      return;
    }
    if (depositAmountError) {
      setError(new Error(depositAmountError));
      return;
    }

    try {
      setMintingChain(chainId);
      const usdc = await getUsdcContract(chainId, usdcAddress);
      const tx = await usdc.mint(session.user.evm_address, parseUnits(normalizedDepositAmount, 6));
      await tx.wait();
      message.success(`已向当前钱包 mint ${normalizedDepositAmount} USDC`);
    } catch (mintError) {
      setError(mintError);
    } finally {
      setMintingChain(null);
    }
  }

  async function handleQuickDeposit(chainId: number, address: string, usdcAddress?: string | null) {
    if (!session) {
      message.warning('请先登录后再进行便捷充值');
      return;
    }
    if (!usdcAddress) {
      setError(new Error('当前链未配置 USDC 合约地址，无法执行便捷充值'));
      return;
    }
    if (depositAmountError) {
      setError(new Error(depositAmountError));
      return;
    }

    try {
      setQuickDepositingChain(chainId);
      const router = await getValidatedRouterContract(chainId, address, usdcAddress);
      const usdc = await getUsdcContract(chainId, usdcAddress);
      const transferTx = await usdc.transfer(address, parseUnits(normalizedDepositAmount, 6));
      await transferTx.wait();
      let forwardHash = '';
      try {
        const forwardTx = await router.forward();
        forwardHash = forwardTx.hash;
        await forwardTx.wait();
      } catch (forwardError) {
        throw new Error(
          `USDC 已转入 Router，但 forward 失败，资金可能暂留在 Router 中，需要人工检查。原始错误: ${forwardError instanceof Error ? forwardError.message : String(forwardError)}`,
        );
      }
      const deposit = await waitForDepositRecord(chainId, address, forwardHash);
      await loadData(true);
      message.success(`充值记录已创建，金额 ${normalizedDepositAmount} USDC，当前状态 ${deposit.status}`);
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
            <Space wrap>
              <Select
                value={selectedChain?.chain_id}
                style={{ minWidth: 220 }}
                placeholder={chains.length === 0 ? '后端未返回可用链' : '请选择链'}
                options={chains.map((chain) => ({
                  label: `${chain.name} (${chain.chain_id})`,
                  value: chain.chain_id,
                }))}
                onChange={(value) => setSelectedChainId(value)}
                disabled={chains.length === 0}
              />
              <Button onClick={() => void loadData(true)} loading={refreshing}>
                刷新状态
              </Button>
            </Space>
          }
        />

        <Alert
          showIcon
          type="info"
          message="充值状态说明"
          description="DETECTED 表示已检测到链上转账，CONFIRMING 表示确认中，CREDITED 才代表账本已记账。本地链工具按钮只在后端明确返回 local_testnet/local_tools_enabled 时展示。"
        />

        {loading || chainsLoading ? <Spin size="large" /> : null}
        <ErrorAlert error={chainsError || error} />
        {!session ? <LoginRequiredCard title="登录后使用充值功能" description="充值页允许未登录浏览，但生成充值地址、领取测试资产和便捷充值都需要先登录。" /> : null}

        {!chainsLoading && chains.length === 0 ? (
          <EmptyStateCard title="暂无可用充值链" description="后端当前未返回任何可用链配置，充值页不会自行伪造链列表或测试地址。" />
        ) : null}

        {selectedChain ? (
          <Card className="surface-card" title={`${selectedChain.name} / ${selectedChain.asset}`} extra={<StatusTag value={selectedAddress ? 'ACTIVE' : 'PENDING'} />}>
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              <Text strong>充值地址</Text>
              <Space wrap>
                <Text code>{selectedDisplayAddress}</Text>
                {selectedAddress ? (
                  <Button
                    type="text"
                    size="small"
                    icon={<CopyOutlined />}
                    onClick={() => void handleCopy(selectedAddress.address, '充值地址已复制')}
                  />
                ) : null}
              </Space>
              <Text type="secondary">确认数要求: {selectedAddress?.confirmations ?? selectedChain.confirmations}</Text>
              {isLocalSelectedChain && selectedChain.local_tools_enabled ? (
                <Space direction="vertical" size={6} style={{ maxWidth: 320 }}>
                  <Text strong>本地链充值金额</Text>
                  <Input
                    value={depositAmount}
                    onChange={(event) => setDepositAmount(event.target.value)}
                    placeholder="1000"
                    status={depositAmountError ? 'error' : ''}
                  />
                  <Text type={depositAmountError ? 'danger' : 'secondary'}>
                    {depositAmountError || '便捷充值与测试 USDC mint 将使用该金额'}
                  </Text>
                </Space>
              ) : null}
              <Space wrap>
                {selectedAddress ? (
                  <>
                    <Button
                      icon={selectedAddressVisible ? <EyeInvisibleOutlined /> : <EyeOutlined />}
                      onClick={() => setRevealed((current) => ({ ...current, [selectedChain.chain_id]: !current[selectedChain.chain_id] }))}
                    >
                      {selectedAddressVisible ? '隐藏地址' : '显示地址'}
                    </Button>
                    <Button onClick={() => void handleCopy(selectedAddress.address, '充值地址已复制')}>复制地址</Button>
                  </>
                ) : (
                  <Button
                    type="primary"
                    loading={generatingChain === selectedChain.chain_id}
                    onClick={() => void handleGenerateAddress(selectedChain.chain_id)}
                    disabled={!selectedChain.deposit_enabled}
                  >
                    生成充值地址
                  </Button>
                )}
              </Space>
              {!selectedChain.deposit_enabled ? (
                <Text type="secondary">当前链未在后端启动配置中启用充值分配能力。</Text>
              ) : null}
              {isLocalSelectedChain && selectedChain.local_tools_enabled ? (
                <Space wrap>
                  <Button loading={fundingNativeChain === selectedChain.chain_id} onClick={() => void handleMintNative(selectedChain.chain_id)}>
                    领取测试 ETH
                  </Button>
                  <Button loading={mintingChain === selectedChain.chain_id} onClick={() => void handleMintUsdc(selectedChain.chain_id, selectedChain.usdc_address)}>
                    领取测试 USDC
                  </Button>
                  <Button
                    type="primary"
                    ghost
                    disabled={!selectedAddress || !!depositAmountError}
                    loading={quickDepositingChain === selectedChain.chain_id}
                    onClick={() => selectedAddress && void handleQuickDeposit(selectedChain.chain_id, selectedAddress.address, selectedChain.usdc_address)}
                  >
                    便捷充值 {normalizedDepositAmount || '--'} USDC
                  </Button>
                </Space>
              ) : null}
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                每条链的充值流程一致：用户获取该链充值地址后，可从任意交易所或钱包向该地址转账。前端只展示后端配置中真实可用的链，不再写死任何链、地址或测试资产。
              </Paragraph>
            </Space>
          </Card>
        ) : null}

        {state ? (
          <Card className="table-card" title="Recent Deposits">
            <Table
              rowKey="deposit_id"
              dataSource={state.deposits}
              scroll={{ x: 980 }}
              pagination={false}
              columns={[
                { title: 'Time', dataIndex: 'detected_at', render: (value: string) => formatDateTime(value), width: 180 },
                { title: 'Chain', dataIndex: 'chain_id', render: (value: number) => formatChainName(value, chains), width: 120 },
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
                  render: (value: string) => (
                    <Space size={4}>
                      <Text type="secondary">{formatAddress(value, 8)}</Text>
                      <Button type="text" size="small" icon={<CopyOutlined />} onClick={() => void handleCopy(value, '交易哈希已复制')} />
                    </Space>
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
