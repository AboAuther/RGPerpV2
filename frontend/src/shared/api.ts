import { appConfig, getChainOption } from './env';
import type {
  AccountSummary,
  ApiEnvelope,
  DepositAddressItem,
  DepositItem,
  ExplorerEvent,
  FillItem,
  FundingItem,
  LoginResponse,
  NonceResponse,
  OrderItem,
  PositionItem,
  RiskSnapshot,
  RuntimeProvider,
  SymbolItem,
  TickerItem,
  TransferItem,
  User,
  WithdrawItem,
  WithdrawRequest,
  BalanceItem,
} from './domain';

export class ApiError extends Error {
  traceId?: string;

  constructor(message: string, traceId?: string) {
    super(message);
    this.name = 'ApiError';
    this.traceId = traceId;
  }
}

let accessToken: string | undefined;

function buildTraceId(): string {
  return `trace_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`;
}

function buildId(prefix: string): string {
  return `${prefix}_${crypto.randomUUID().replace(/-/g, '').slice(0, 12)}`;
}

function nowIso(offsetMinutes = 0): string {
  return new Date(Date.now() + offsetMinutes * 60_000).toISOString();
}

function clone<T>(input: T): T {
  return JSON.parse(JSON.stringify(input)) as T;
}

async function sleep(ms = 280): Promise<void> {
  await new Promise((resolve) => window.setTimeout(resolve, ms));
}

async function requestJson<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${appConfig.apiBaseUrl.replace(/\/$/, '')}${path}`, {
    headers: {
      Accept: 'application/json',
      ...(init?.method && init.method !== 'GET' ? { 'Content-Type': 'application/json' } : {}),
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      ...init?.headers,
    },
    ...init,
  });

  let parsed: ApiEnvelope<T> | null = null;
  try {
    parsed = (await response.json()) as ApiEnvelope<T>;
  } catch {
    parsed = null;
  }

  if (!response.ok) {
    throw new ApiError(parsed?.message || `Request failed with status ${response.status}`, parsed?.trace_id);
  }

  if (!parsed) {
    throw new ApiError('接口返回为空');
  }

  return parsed.data;
}

export function setApiAccessToken(token?: string) {
  accessToken = token;
}

interface ProviderPolicy {
  featureName: string;
  allowExplicitMock: boolean;
  allowAutoMockFallback: boolean;
}

async function resolveProvider<T>(
  httpCall: () => Promise<T>,
  mockCall: () => Promise<T>,
  policy: ProviderPolicy,
): Promise<{ data: T; provider: RuntimeProvider }> {
  if (appConfig.apiProvider === 'mock') {
    if (!policy.allowExplicitMock || !appConfig.reviewFeaturesEnabled) {
      throw new ApiError(`${policy.featureName} 不允许使用 mock provider`);
    }
    return { data: await mockCall(), provider: 'mock' };
  }

  if (appConfig.apiProvider === 'http') {
    return { data: await httpCall(), provider: 'http' };
  }

  try {
    return { data: await httpCall(), provider: 'http' };
  } catch (error) {
    if (!policy.allowAutoMockFallback || !policy.allowExplicitMock || !appConfig.reviewFeaturesEnabled) {
      throw error;
    }
    return { data: await mockCall(), provider: 'mock' };
  }
}

const criticalPolicy: ProviderPolicy = {
  featureName: '关键接口',
  allowExplicitMock: true,
  allowAutoMockFallback: false,
};

const reviewOnlyPolicy: ProviderPolicy = {
  featureName: 'Review 功能',
  allowExplicitMock: true,
  allowAutoMockFallback: false,
};

const marketReadPolicy: ProviderPolicy = {
  featureName: '市场数据',
  allowExplicitMock: true,
  allowAutoMockFallback: appConfig.reviewFeaturesEnabled,
};

const mockUser: User = {
  id: 10001,
  evm_address: '0xA11CE4bD9Cb4a62a0f772E5B09b4dC2A0e2912c1',
  status: 'ACTIVE',
};

const mockSummary: AccountSummary = {
  equity: '24875.42',
  available_balance: '18320.17',
  total_initial_margin: '4210.13',
  total_maintenance_margin: '2560.45',
  unrealized_pnl: '384.92',
  margin_ratio: '0.1030',
};

const mockBalances: BalanceItem[] = [
  { account_code: 'USER_WALLET', asset: 'USDC', balance: '18320.17' },
  { account_code: 'USER_POSITION_MARGIN', asset: 'USDC', balance: '6155.25' },
  { account_code: 'USER_WITHDRAW_HOLD', asset: 'USDC', balance: '400.00' },
];

const mockDepositAddresses: DepositAddressItem[] = [
  {
    chain_id: 1,
    asset: 'USDC',
    address: '0xD3E7dE0E1F96A450c12B2015a6c5D7443c1B7031',
    confirmations: 12,
  },
  {
    chain_id: 42161,
    asset: 'USDC',
    address: '0xD3E7dE0E1F96A450c12B2015a6c5D7443c1B7031',
    confirmations: 20,
  },
  {
    chain_id: 8453,
    asset: 'USDC',
    address: '0xD3E7dE0E1F96A450c12B2015a6c5D7443c1B7031',
    confirmations: 20,
  },
];

const mockState = {
  deposits: [
    {
      deposit_id: 'dep_20260321001',
      chain_id: 1,
      asset: 'USDC',
      amount: '2500.00',
      tx_hash: '0x5ca18bcfd9f4e7a581c59e1df38065d04ddfdf7eb282e0ee6a6d2c68704b8a7f',
      confirmations: 12,
      required_confirmations: 12,
      status: 'CREDITED',
      address: mockDepositAddresses[0].address,
      detected_at: nowIso(-210),
    },
    {
      deposit_id: 'dep_20260321002',
      chain_id: 42161,
      asset: 'USDC',
      amount: '980.50',
      tx_hash: '0x89b987ae86a9ea1ca5b38adb74ff1fb1f7b30c2d5d52d54e1772d8f15f4c3342',
      confirmations: 7,
      required_confirmations: 20,
      status: 'CONFIRMING',
      address: mockDepositAddresses[1].address,
      detected_at: nowIso(-35),
    },
  ] as DepositItem[],
  withdrawals: [
    {
      withdraw_id: 'wd_20260321001',
      chain_id: 8453,
      asset: 'USDC',
      amount: '400.00',
      fee_amount: '1.00',
      to_address: '0x17F2bB9388d53A3f0eA3DccE7B286cA4668898F3',
      status: 'MANUAL_REVIEW',
      tx_hash: null,
      created_at: nowIso(-180),
    },
    {
      withdraw_id: 'wd_20260321002',
      chain_id: 1,
      asset: 'USDC',
      amount: '120.00',
      fee_amount: '1.00',
      to_address: '0x7C9F0A77c575C7DA8c5f7383C0Ac83b83D3077f1',
      status: 'CONFIRMING',
      tx_hash: '0x9ab5c347fca62deff2ad23a4ef552ec85d2b9c3ab09ac61757f52edb6ea18fc0',
      created_at: nowIso(-75),
    },
  ] as WithdrawItem[],
  symbols: [
    { symbol: 'BTC-PERP', asset_class: 'CRYPTO', tick_size: '0.1', step_size: '0.001', min_notional: '10', status: 'TRADING' },
    { symbol: 'ETH-PERP', asset_class: 'CRYPTO', tick_size: '0.01', step_size: '0.001', min_notional: '10', status: 'REDUCE_ONLY' },
    { symbol: 'XAUUSD-PERP', asset_class: 'COMMODITY', tick_size: '0.1', step_size: '0.01', min_notional: '20', status: 'HALTED' },
  ] as SymbolItem[],
  tickers: [
    { symbol: 'BTC-PERP', index_price: '84210.35', mark_price: '84228.64', best_bid: '84222.1', best_ask: '84233.4', ts: nowIso(-1) },
    { symbol: 'ETH-PERP', index_price: '2038.44', mark_price: '2039.01', best_bid: '2038.80', best_ask: '2039.21', ts: nowIso(-1) },
    { symbol: 'XAUUSD-PERP', index_price: '2198.60', mark_price: '2198.60', best_bid: '2198.30', best_ask: '2198.90', ts: nowIso(-4) },
  ] as TickerItem[],
  orders: [
    {
      order_id: 'ord_20260321001',
      client_order_id: 'cli_20260321001',
      symbol: 'BTC-PERP',
      side: 'BUY',
      position_effect: 'OPEN',
      type: 'LIMIT',
      qty: '0.050',
      filled_qty: '0.020',
      avg_fill_price: '84196.40',
      price: '84200.00',
      trigger_price: null,
      reduce_only: false,
      status: 'PARTIALLY_FILLED',
      reject_reason: null,
    },
    {
      order_id: 'ord_20260321002',
      client_order_id: 'cli_20260321002',
      symbol: 'ETH-PERP',
      side: 'SELL',
      position_effect: 'REDUCE',
      type: 'STOP_MARKET',
      qty: '1.200',
      filled_qty: '0.000',
      avg_fill_price: '0',
      price: null,
      trigger_price: '1985.00',
      reduce_only: true,
      status: 'TRIGGER_WAIT',
      reject_reason: null,
    },
  ] as OrderItem[],
  fills: [
    {
      fill_id: 'fill_20260321001',
      order_id: 'ord_20260321001',
      symbol: 'BTC-PERP',
      side: 'BUY',
      qty: '0.020',
      price: '84196.40',
      fee_amount: '0.84',
      created_at: nowIso(-88),
    },
    {
      fill_id: 'fill_20260321002',
      order_id: 'ord_20260320998',
      symbol: 'ETH-PERP',
      side: 'SELL',
      qty: '0.800',
      price: '2045.21',
      fee_amount: '0.82',
      created_at: nowIso(-300),
    },
  ] as FillItem[],
  positions: [
    {
      position_id: 'pos_20260321001',
      symbol: 'BTC-PERP',
      side: 'LONG',
      qty: '0.120',
      avg_entry_price: '83540.00',
      mark_price: '84228.64',
      initial_margin: '2030.32',
      maintenance_margin: '1218.19',
      realized_pnl: '42.18',
      unrealized_pnl: '82.64',
      funding_accrual: '-4.12',
      liquidation_price: '77832.55',
      status: 'OPEN',
    },
    {
      position_id: 'pos_20260321002',
      symbol: 'ETH-PERP',
      side: 'SHORT',
      qty: '1.800',
      avg_entry_price: '2058.10',
      mark_price: '2039.01',
      initial_margin: '920.18',
      maintenance_margin: '582.40',
      realized_pnl: '120.44',
      unrealized_pnl: '34.36',
      funding_accrual: '1.87',
      liquidation_price: '2232.18',
      status: 'OPEN',
    },
  ] as PositionItem[],
  explorerEvents: [
    {
      event_id: 'evt_20260321001',
      event_type: 'wallet.deposit.detected',
      ledger_tx_id: null,
      chain_tx_hash: '0x89b987ae86a9ea1ca5b38adb74ff1fb1f7b30c2d5d52d54e1772d8f15f4c3342',
      order_id: null,
      fill_id: null,
      position_id: null,
      address: mockUser.evm_address,
      payload: { status: 'CONFIRMING', chain_id: 42161, amount: '980.50' },
    },
    {
      event_id: 'evt_20260321002',
      event_type: 'trade.fill.created',
      ledger_tx_id: 'lgtx_20260321008',
      chain_tx_hash: null,
      order_id: 'ord_20260321001',
      fill_id: 'fill_20260321001',
      position_id: 'pos_20260321001',
      address: mockUser.evm_address,
      payload: { price: '84196.40', qty: '0.020', symbol: 'BTC-PERP' },
    },
  ] as ExplorerEvent[],
  fundingHistory: [
    {
      funding_id: 'fund_20260321001',
      symbol: 'BTC-PERP',
      direction: 'PAY',
      rate: '0.000117',
      amount: '-3.88',
      settled_at: nowIso(-130),
      batch_id: 'fund_batch_2026032102',
    },
    {
      funding_id: 'fund_20260321002',
      symbol: 'ETH-PERP',
      direction: 'RECEIVE',
      rate: '0.000064',
      amount: '1.87',
      settled_at: nowIso(-70),
      batch_id: 'fund_batch_2026032103',
    },
  ] as FundingItem[],
  transfers: [
    {
      transfer_id: 'trf_20260321001',
      asset: 'USDC',
      amount: '250.00',
      from_account: 'USER_WALLET',
      to_account: 'USER_POSITION_MARGIN',
      status: 'COMPLETED',
      created_at: nowIso(-240),
    },
    {
      transfer_id: 'trf_20260321002',
      asset: 'USDC',
      amount: '180.00',
      from_account: 'USER_POSITION_MARGIN',
      to_account: 'USER_WALLET',
      status: 'COMPLETED',
      created_at: nowIso(-180),
    },
  ] as TransferItem[],
  risk: {
    account_status: 'ACTIVE',
    risk_state: 'WATCH',
    mark_price_stale: false,
    can_open_risk: true,
    notes: ['ETH-PERP 当前为 REDUCE_ONLY，只允许减仓。', '风险率接近预警阈值，请关注保证金补充。'],
  } as RiskSnapshot,
};

const mockApi = {
  async issueNonce(address: string, chainId: number): Promise<NonceResponse> {
    await sleep();
    return {
      nonce: buildId('challenge'),
      domain: 'review.rgperp.local',
      chain_id: chainId,
      expires_at: nowIso(5),
    };
  },

  async login(): Promise<LoginResponse> {
    await sleep();
    return {
      access_token: buildId('mock_access'),
      refresh_token: buildId('mock_refresh'),
      expires_at: nowIso(60),
      user: clone(mockUser),
    };
  },

  async getSummary(): Promise<AccountSummary> {
    await sleep();
    return clone(mockSummary);
  },

  async getBalances(): Promise<BalanceItem[]> {
    await sleep();
    return clone(mockBalances);
  },

  async getDepositAddresses(): Promise<DepositAddressItem[]> {
    await sleep();
    return clone(mockDepositAddresses);
  },

  async getDeposits(): Promise<DepositItem[]> {
    await sleep();
    return clone(mockState.deposits);
  },

  async getWithdrawals(): Promise<WithdrawItem[]> {
    await sleep();
    return clone(mockState.withdrawals);
  },

  async createWithdrawal(input: WithdrawRequest): Promise<void> {
    await sleep(420);
    const chain = getChainOption(input.chain_id);
    mockState.withdrawals.unshift({
      withdraw_id: buildId('wd'),
      chain_id: input.chain_id,
      asset: input.asset,
      amount: input.amount,
      fee_amount: '1.00',
      to_address: input.to_address,
      status: Number(input.amount) >= 10000 ? 'MANUAL_REVIEW' : 'REQUESTED',
      tx_hash: null,
      created_at: nowIso(0),
    });

    mockState.explorerEvents.unshift({
      event_id: buildId('evt'),
      event_type: 'wallet.withdraw.requested',
      ledger_tx_id: buildId('lgtx'),
      chain_tx_hash: null,
      address: mockUser.evm_address,
      payload: {
        chain: chain?.name ?? input.chain_id,
        amount: input.amount,
        to_address: input.to_address,
      },
    });
  },

  async requestFaucet(address: string, chainId: number): Promise<void> {
    await sleep(500);
    const chain = getChainOption(chainId);
    const addressEntry = mockDepositAddresses.find((item) => item.chain_id === chainId);
    mockState.deposits.unshift({
      deposit_id: buildId('dep'),
      chain_id: chainId,
      asset: 'USDC',
      amount: '10000.00',
      tx_hash: `0x${crypto.randomUUID().replace(/-/g, '')}${crypto.randomUUID().replace(/-/g, '').slice(0, 24)}`,
      confirmations: 0,
      required_confirmations: chain?.confirmations ?? 12,
      status: 'DETECTED',
      address: addressEntry?.address ?? address,
      detected_at: nowIso(0),
    });
  },

  async getSymbols(): Promise<SymbolItem[]> {
    await sleep();
    return clone(mockState.symbols);
  },

  async getTickers(): Promise<TickerItem[]> {
    await sleep();
    return clone(mockState.tickers);
  },

  async getOrders(): Promise<OrderItem[]> {
    await sleep();
    return clone(mockState.orders);
  },

  async getFills(): Promise<FillItem[]> {
    await sleep();
    return clone(mockState.fills);
  },

  async getPositions(): Promise<PositionItem[]> {
    await sleep();
    return clone(mockState.positions);
  },

  async getExplorerEvents(): Promise<ExplorerEvent[]> {
    await sleep();
    return clone(mockState.explorerEvents);
  },

  async getFundingHistory(): Promise<FundingItem[]> {
    await sleep();
    return clone(mockState.fundingHistory);
  },

  async getTransfers(): Promise<TransferItem[]> {
    await sleep();
    return clone(mockState.transfers);
  },

  async getRisk(): Promise<RiskSnapshot> {
    await sleep();
    return clone(mockState.risk);
  },
};

export function buildLoginMessage(domain: string, chainId: number, nonce: string): string {
  return `RGPerp Login\nDomain: ${domain}\nChain ID: ${chainId}\nNonce: ${nonce}`;
}

export const api = {
  auth: {
    async issueNonce(address: string, chainId: number): Promise<NonceResponse & { provider: RuntimeProvider }> {
      const result = await resolveProvider(
        () =>
          requestJson<NonceResponse>('/api/v1/auth/nonce', {
            method: 'POST',
            body: JSON.stringify({ address, chain_id: chainId }),
            headers: { 'X-Trace-Id': buildTraceId() },
          }),
        () => mockApi.issueNonce(address, chainId),
        { ...criticalPolicy, featureName: '登录挑战' },
      );
      return { ...result.data, provider: result.provider };
    },

    async login(input: {
      address: string;
      chainId: number;
      nonce: string;
      signature: string;
    }): Promise<LoginResponse & { provider: RuntimeProvider }> {
      const result = await resolveProvider(
        () =>
          requestJson<LoginResponse>('/api/v1/auth/login', {
            method: 'POST',
            body: JSON.stringify({
              address: input.address,
              chain_id: input.chainId,
              nonce: input.nonce,
              signature: input.signature,
            }),
            headers: { 'X-Trace-Id': buildTraceId() },
          }),
        () => mockApi.login(),
        { ...criticalPolicy, featureName: '登录验签' },
      );
      return { ...result.data, provider: result.provider };
    },
  },

  account: {
    async getSummary(): Promise<AccountSummary> {
      const result = await resolveProvider(
        () => requestJson<AccountSummary>('/api/v1/account/summary'),
        () => mockApi.getSummary(),
        { ...criticalPolicy, featureName: '账户总览' },
      );
      return result.data;
    },

    async getBalances(): Promise<BalanceItem[]> {
      const result = await resolveProvider(
        () => requestJson<BalanceItem[]>('/api/v1/account/balances'),
        () => mockApi.getBalances(),
        { ...criticalPolicy, featureName: '账户余额' },
      );
      return result.data;
    },

    async getRisk(): Promise<RiskSnapshot> {
      const result = await resolveProvider(
        () => requestJson<RiskSnapshot>('/api/v1/account/risk'),
        () => mockApi.getRisk(),
        { ...criticalPolicy, featureName: '风险快照' },
      );
      return result.data;
    },
  },

  wallet: {
    async getDepositAddresses(): Promise<DepositAddressItem[]> {
      const result = await resolveProvider(
        () => requestJson<DepositAddressItem[]>('/api/v1/wallet/deposit-addresses'),
        () => mockApi.getDepositAddresses(),
        { ...criticalPolicy, featureName: '充值地址' },
      );
      return result.data;
    },

    async getDeposits(): Promise<DepositItem[]> {
      const result = await resolveProvider(
        () => requestJson<DepositItem[]>('/api/v1/wallet/deposits'),
        () => mockApi.getDeposits(),
        { ...criticalPolicy, featureName: '充值记录' },
      );
      return result.data;
    },

    async getWithdrawals(): Promise<WithdrawItem[]> {
      const result = await resolveProvider(
        () => requestJson<WithdrawItem[]>('/api/v1/wallet/withdrawals'),
        () => mockApi.getWithdrawals(),
        { ...criticalPolicy, featureName: '提现记录' },
      );
      return result.data;
    },

    async createWithdrawal(input: WithdrawRequest): Promise<void> {
      const traceId = buildTraceId();
      const idemKey = buildId('idem');

      const result = await resolveProvider(
        () =>
          requestJson<void>('/api/v1/wallet/withdrawals', {
            method: 'POST',
            body: JSON.stringify(input),
            headers: {
              'X-Trace-Id': traceId,
              'Idempotency-Key': idemKey,
            },
          }),
        () => mockApi.createWithdrawal(input),
        { ...criticalPolicy, featureName: '提现申请' },
      );
      return result.data;
    },

    async requestFaucet(input: { address: string; chainId: number }): Promise<void> {
      if (!appConfig.reviewFaucetEnabled) {
        throw new ApiError('当前环境未启用 review faucet');
      }

      const result = await resolveProvider(
        () =>
          requestJson<void>('/api/v1/review/faucet', {
            method: 'POST',
            body: JSON.stringify({
              address: input.address,
              chain_id: input.chainId,
            }),
            headers: { 'X-Trace-Id': buildTraceId() },
          }),
        () => mockApi.requestFaucet(input.address, input.chainId),
        { ...reviewOnlyPolicy, featureName: 'Review Faucet' },
      );
      return result.data;
    },
  },

  market: {
    async getSymbols(): Promise<SymbolItem[]> {
      const result = await resolveProvider(
        () => requestJson<SymbolItem[]>('/api/v1/markets/symbols'),
        () => mockApi.getSymbols(),
        marketReadPolicy,
      );
      return result.data;
    },

    async getTickers(): Promise<TickerItem[]> {
      const result = await resolveProvider(
        () => requestJson<TickerItem[]>('/api/v1/markets/tickers'),
        () => mockApi.getTickers(),
        marketReadPolicy,
      );
      return result.data;
    },
  },

  orders: {
    async getOrders(): Promise<OrderItem[]> {
      const result = await resolveProvider(
        () => requestJson<OrderItem[]>('/api/v1/orders'),
        () => mockApi.getOrders(),
        { ...criticalPolicy, featureName: '订单历史' },
      );
      return result.data;
    },
  },

  fills: {
    async getFills(): Promise<FillItem[]> {
      const result = await resolveProvider(
        () => requestJson<FillItem[]>('/api/v1/fills'),
        () => mockApi.getFills(),
        { ...criticalPolicy, featureName: '成交历史' },
      );
      return result.data;
    },
  },

  positions: {
    async getPositions(): Promise<PositionItem[]> {
      const result = await resolveProvider(
        () => requestJson<PositionItem[]>('/api/v1/positions'),
        () => mockApi.getPositions(),
        { ...criticalPolicy, featureName: '持仓数据' },
      );
      return result.data;
    },
  },

  explorer: {
    async getEvents(): Promise<ExplorerEvent[]> {
      const result = await resolveProvider(
        () => requestJson<ExplorerEvent[]>('/api/v1/explorer/events'),
        () => mockApi.getExplorerEvents(),
        { ...criticalPolicy, featureName: 'Explorer 事件流' },
      );
      return result.data;
    },
  },

  funding: {
    async getHistory(): Promise<FundingItem[]> {
      const result = await resolveProvider(
        () => requestJson<FundingItem[]>('/api/v1/account/funding'),
        () => mockApi.getFundingHistory(),
        { ...criticalPolicy, featureName: '资金费历史' },
      );
      return result.data;
    },
  },

  transfers: {
    async getHistory(): Promise<TransferItem[]> {
      const result = await resolveProvider(
        () => requestJson<TransferItem[]>('/api/v1/account/transfers'),
        () => mockApi.getTransfers(),
        { ...criticalPolicy, featureName: '划转历史' },
      );
      return result.data;
    },
  },
};
