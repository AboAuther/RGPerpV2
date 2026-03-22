import { appConfig } from './env';
import type {
  AccountSummary,
  ApiEnvelope,
  BalanceItem,
  ChallengeResponse,
  DepositAddressItem,
  DepositItem,
  ExplorerEvent,
  FillItem,
  FundingItem,
  LoginResponse,
  AdminWithdrawReviewItem,
  OrderItem,
  PositionItem,
  RiskSnapshot,
  SymbolItem,
  TickerItem,
  TransferItem,
  WithdrawItem,
  WithdrawRequest,
  SystemChainItem,
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

function buildRequestHeaders(init?: RequestInit): Headers {
  const headers = new Headers(init?.headers);

  if (!headers.has('Accept')) {
    headers.set('Accept', 'application/json');
  }
  if (init?.method && init.method !== 'GET' && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (accessToken && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${accessToken}`);
  }

  return headers;
}

async function requestJson<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${appConfig.apiBaseUrl.replace(/\/$/, '')}${path}`, {
    ...init,
    headers: buildRequestHeaders(init),
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

export const api = {
  system: {
    getChains(): Promise<SystemChainItem[]> {
      return requestJson<SystemChainItem[]>('/api/v1/system/chains');
    },
  },

  auth: {
    challenge(address: string, chainId: number): Promise<ChallengeResponse> {
      return requestJson<ChallengeResponse>('/api/v1/auth/challenge', {
        method: 'POST',
        body: JSON.stringify({ address, chain_id: chainId }),
        headers: { 'X-Trace-Id': buildTraceId() },
      });
    },

    login(input: {
      address: string;
      chainId: number;
      nonce: string;
      signature: string;
    }): Promise<LoginResponse> {
      return requestJson<LoginResponse>('/api/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify({
          address: input.address,
          chain_id: input.chainId,
          nonce: input.nonce,
          signature: input.signature,
        }),
        headers: { 'X-Trace-Id': buildTraceId() },
      });
    },
  },

  account: {
    getSummary(): Promise<AccountSummary> {
      return requestJson<AccountSummary>('/api/v1/account/summary');
    },
    getBalances(): Promise<BalanceItem[]> {
      return requestJson<BalanceItem[]>('/api/v1/account/balances');
    },
    getRisk(): Promise<RiskSnapshot> {
      return requestJson<RiskSnapshot>('/api/v1/account/risk');
    },
  },

  wallet: {
    getDepositAddresses(): Promise<DepositAddressItem[]> {
      return requestJson<DepositAddressItem[]>('/api/v1/wallet/deposit-addresses');
    },
    generateDepositAddress(chainId: number): Promise<DepositAddressItem> {
      return requestJson<DepositAddressItem>(`/api/v1/wallet/deposit-addresses/${chainId}/generate`, {
        method: 'POST',
        headers: { 'X-Trace-Id': buildTraceId() },
      });
    },
    getDeposits(): Promise<DepositItem[]> {
      return requestJson<DepositItem[]>('/api/v1/wallet/deposits');
    },
    getWithdrawals(): Promise<WithdrawItem[]> {
      return requestJson<WithdrawItem[]>('/api/v1/wallet/withdrawals');
    },
    createWithdrawal(input: WithdrawRequest): Promise<void> {
      return requestJson<void>('/api/v1/wallet/withdrawals', {
        method: 'POST',
        body: JSON.stringify(input),
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('idem'),
        },
      });
    },
    requestLocalNativeFaucet(chainId: number): Promise<{ tx_hash: string }> {
      return requestJson<{ tx_hash: string }>('/api/v1/wallet/local-faucet/native', {
        method: 'POST',
        body: JSON.stringify({ chain_id: chainId }),
        headers: { 'X-Trace-Id': buildTraceId() },
      });
    },
  },

  market: {
    getSymbols(): Promise<SymbolItem[]> {
      return requestJson<SymbolItem[]>('/api/v1/markets/symbols');
    },
    getTickers(): Promise<TickerItem[]> {
      return requestJson<TickerItem[]>('/api/v1/markets/tickers');
    },
  },

  orders: {
    getOrders(): Promise<OrderItem[]> {
      return requestJson<OrderItem[]>('/api/v1/orders');
    },
  },

  fills: {
    getFills(): Promise<FillItem[]> {
      return requestJson<FillItem[]>('/api/v1/fills');
    },
  },

  positions: {
    getPositions(): Promise<PositionItem[]> {
      return requestJson<PositionItem[]>('/api/v1/positions');
    },
  },

  explorer: {
    getEvents(): Promise<ExplorerEvent[]> {
      return requestJson<ExplorerEvent[]>('/api/v1/explorer/events');
    },
  },

  admin: {
    getWithdrawals(): Promise<AdminWithdrawReviewItem[]> {
      return requestJson<AdminWithdrawReviewItem[]>('/api/v1/admin/withdrawals');
    },
    approveWithdrawal(withdrawId: string): Promise<{ withdraw_id: string; status: string }> {
      return requestJson<{ withdraw_id: string; status: string }>(`/api/v1/admin/withdrawals/${withdrawId}/approve`, {
        method: 'POST',
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('idem'),
        },
      });
    },
  },

  funding: {
    getHistory(): Promise<FundingItem[]> {
      return requestJson<FundingItem[]>('/api/v1/account/funding');
    },
  },

  transfers: {
    getHistory(): Promise<TransferItem[]> {
      return requestJson<TransferItem[]>('/api/v1/account/transfers');
    },
  },
};
