import { appConfig } from './env';
import type {
  AccountSummary,
  ApiEnvelope,
  BalanceItem,
  ChallengeResponse,
  DepositAddressItem,
  DepositItem,
  AdminHedgeIntentItem,
  ExplorerEvent,
  FillItem,
  FundingQuoteItem,
  FundingItem,
  LoginResponse,
  AdminWithdrawReviewItem,
  AdminLiquidationItem,
  AdminLiquidationActionResult,
  InsuranceFundTopUpRequest,
  InsuranceFundTopUpResult,
  OrderItem,
  OrderCreateRequest,
  LedgerOverview,
  LedgerAuditReport,
  RiskMonitorDashboard,
  RuntimeConfigPatchRequest,
  RuntimeConfigView,
  SystemHedgeSnapshotItem,
  PositionItem,
  RiskSnapshot,
  SymbolItem,
  TickerItem,
  TransferItem,
  TransferRequest,
  WithdrawItem,
  WithdrawRequest,
  SystemChainItem,
  User,
} from './domain';

export class ApiError extends Error {
  traceId?: string;
  status?: number;

  constructor(message: string, traceId?: string, status?: number) {
    super(message);
    this.name = 'ApiError';
    this.traceId = traceId;
    this.status = status;
  }
}

let accessToken: string | undefined;
let refreshInFlight: Promise<boolean> | null = null;

type SessionRefreshPayload = {
  accessToken: string;
  refreshToken: string;
  expiresAt?: string;
  user: User;
};

type AuthSessionHooks = {
  getRefreshToken: () => string | undefined;
  onSessionRefreshed: (session: SessionRefreshPayload) => void;
  onSessionInvalidated: () => void;
};

let authSessionHooks: AuthSessionHooks | undefined;

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
  if (!headers.has('X-Trace-Id')) {
    headers.set('X-Trace-Id', buildTraceId());
  }
  if (init?.method && init.method !== 'GET' && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (accessToken && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${accessToken}`);
  }

  return headers;
}

// Session refresh is serialized on purpose so multiple protected requests do
// not stampede the refresh endpoint with the same expired credentials.
async function tryRefreshSession(): Promise<boolean> {
  if (refreshInFlight) {
    return refreshInFlight;
  }
  if (!authSessionHooks) {
    return false;
  }
  const refreshToken = authSessionHooks.getRefreshToken();
  if (!refreshToken) {
    return false;
  }

  refreshInFlight = (async () => {
    try {
      const response = await fetch(`${appConfig.apiBaseUrl.replace(/\/$/, '')}/api/v1/auth/refresh`, {
        method: 'POST',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json',
          'X-Trace-Id': buildTraceId(),
        },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });
      const parsed = (await response.json()) as ApiEnvelope<{
        access_token: string;
        refresh_token: string;
        expires_at?: string;
        user: User;
      }>;
      if (!response.ok || !parsed?.data?.access_token || !parsed?.data?.refresh_token) {
        throw new Error(parsed?.message || `Refresh failed with status ${response.status}`);
      }
      const nextSession: SessionRefreshPayload = {
        accessToken: parsed.data.access_token,
        refreshToken: parsed.data.refresh_token,
        expiresAt: parsed.data.expires_at,
        user: parsed.data.user,
      };
      setApiAccessToken(nextSession.accessToken);
      authSessionHooks.onSessionRefreshed(nextSession);
      return true;
    } catch {
      setApiAccessToken(undefined);
      authSessionHooks.onSessionInvalidated();
      return false;
    } finally {
      refreshInFlight = null;
    }
  })();

  return refreshInFlight;
}

async function requestJson<T>(path: string, init?: RequestInit, retryAuth = true): Promise<T> {
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
    if (response.status === 401 && retryAuth && !path.startsWith('/api/v1/auth/')) {
      const refreshed = await tryRefreshSession();
      if (refreshed) {
        return requestJson<T>(path, init, false);
      }
    }
    throw new ApiError(parsed?.message || `Request failed with status ${response.status}`, parsed?.trace_id, response.status);
  }
  if (!parsed) {
    throw new ApiError('接口返回为空');
  }
  return parsed.data;
}

async function requestBlob(path: string, init?: RequestInit, retryAuth = true): Promise<Blob> {
  const response = await fetch(`${appConfig.apiBaseUrl.replace(/\/$/, '')}${path}`, {
    ...init,
    headers: buildRequestHeaders(init),
  });
  if (!response.ok) {
    if (response.status === 401 && retryAuth && !path.startsWith('/api/v1/auth/')) {
      const refreshed = await tryRefreshSession();
      if (refreshed) {
        return requestBlob(path, init, false);
      }
    }
    let parsed: ApiEnvelope<unknown> | null = null;
    try {
      parsed = (await response.json()) as ApiEnvelope<unknown>;
    } catch {
      parsed = null;
    }
    throw new ApiError(parsed?.message || `Request failed with status ${response.status}`, parsed?.trace_id, response.status);
  }
  return response.blob();
}

export function setApiAccessToken(token?: string) {
  accessToken = token;
}

export function configureAuthSessionHooks(hooks?: AuthSessionHooks) {
  authSessionHooks = hooks;
}

// The API facade centralizes auth, tracing, and envelope handling so feature
// pages can stay focused on operator and trader workflows.
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
    refresh(refreshToken: string): Promise<LoginResponse> {
      return requestJson<LoginResponse>('/api/v1/auth/refresh', {
        method: 'POST',
        body: JSON.stringify({ refresh_token: refreshToken }),
        headers: { 'X-Trace-Id': buildTraceId() },
      }, false);
    },
    logout(): Promise<{ status: string }> {
      return requestJson<{ status: string }>('/api/v1/auth/logout', {
        method: 'POST',
        headers: { 'X-Trace-Id': buildTraceId() },
      }, false);
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
    getFundingHistory(): Promise<FundingItem[]> {
      return requestJson<FundingItem[]>('/api/v1/account/funding');
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
    getFundingQuotes(): Promise<FundingQuoteItem[]> {
      return requestJson<FundingQuoteItem[]>('/api/v1/markets/funding');
    },
  },

  orders: {
    getOrders(): Promise<OrderItem[]> {
      return requestJson<OrderItem[]>('/api/v1/orders');
    },
    createOrder(input: OrderCreateRequest): Promise<OrderItem> {
      return requestJson<OrderItem>('/api/v1/orders', {
        method: 'POST',
        body: JSON.stringify(input),
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': input.client_order_id,
        },
      });
    },
    cancelOrder(orderId: string): Promise<{ status: string }> {
      return requestJson<{ status: string }>(`/api/v1/orders/${orderId}/cancel`, {
        method: 'POST',
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('cancel'),
        },
      });
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
    getEvents(filters?: {
      query?: string;
      eventType?: string;
      asset?: string;
      ledgerTxId?: string;
      chainTxHash?: string;
      orderId?: string;
      fillId?: string;
      positionId?: string;
      address?: string;
      fundingBatchId?: string;
      blockHeight?: string;
      limit?: number;
    }): Promise<ExplorerEvent[]> {
      const search = new URLSearchParams();
      if (filters?.query) {
        search.set('q', filters.query);
      }
      if (filters?.eventType) {
        search.set('event_type', filters.eventType);
      }
      if (filters?.asset) {
        search.set('asset', filters.asset);
      }
      if (filters?.ledgerTxId) {
        search.set('ledger_tx_id', filters.ledgerTxId);
      }
      if (filters?.chainTxHash) {
        search.set('chain_tx_hash', filters.chainTxHash);
      }
      if (filters?.orderId) {
        search.set('order_id', filters.orderId);
      }
      if (filters?.fillId) {
        search.set('fill_id', filters.fillId);
      }
      if (filters?.positionId) {
        search.set('position_id', filters.positionId);
      }
      if (filters?.address) {
        search.set('address', filters.address);
      }
      if (filters?.fundingBatchId) {
        search.set('funding_batch_id', filters.fundingBatchId);
      }
      if (filters?.blockHeight) {
        search.set('block_height', filters.blockHeight);
      }
      if (filters?.limit) {
        search.set('limit', String(filters.limit));
      }
      const suffix = search.toString();
      return requestJson<ExplorerEvent[]>(`/api/v1/explorer/events${suffix ? `?${suffix}` : ''}`);
    },
  },

  admin: {
    getWithdrawals(): Promise<AdminWithdrawReviewItem[]> {
      return requestJson<AdminWithdrawReviewItem[]>('/api/v1/admin/withdrawals');
    },
    getLiquidations(): Promise<AdminLiquidationItem[]> {
      return requestJson<AdminLiquidationItem[]>('/api/v1/admin/liquidations');
    },
    retryLiquidation(liquidationId: string): Promise<AdminLiquidationActionResult> {
      return requestJson<AdminLiquidationActionResult>(`/api/v1/admin/liquidations/${liquidationId}/retry`, {
        method: 'POST',
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('liqretry'),
        },
      });
    },
    closeLiquidation(liquidationId: string, reason: string): Promise<AdminLiquidationActionResult> {
      return requestJson<AdminLiquidationActionResult>(`/api/v1/admin/liquidations/${liquidationId}/close`, {
        method: 'POST',
        body: JSON.stringify({ reason }),
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('liqclose'),
        },
      });
    },
    getRiskMonitorDashboard(): Promise<RiskMonitorDashboard> {
      return requestJson<RiskMonitorDashboard>('/api/v1/admin/risk/net-exposures');
    },
    getHedgeIntents(limit = 50): Promise<AdminHedgeIntentItem[]> {
      return requestJson<AdminHedgeIntentItem[]>(`/api/v1/admin/hedges/intents?limit=${limit}`);
    },
    getHedgeSnapshots(limit = 50): Promise<SystemHedgeSnapshotItem[]> {
      return requestJson<SystemHedgeSnapshotItem[]>(`/api/v1/admin/hedges/snapshots?limit=${limit}`);
    },
    getRuntimeConfig(limit = 20): Promise<RuntimeConfigView> {
      return requestJson<RuntimeConfigView>(`/api/v1/admin/configs/runtime?limit=${limit}`);
    },
    updateRuntimeConfig(input: RuntimeConfigPatchRequest): Promise<RuntimeConfigView> {
      return requestJson<RuntimeConfigView>('/api/v1/admin/configs/runtime', {
        method: 'POST',
        body: JSON.stringify(input),
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('runtimecfg'),
        },
      });
    },
    getLedgerOverview(asset?: string): Promise<LedgerOverview> {
      const params = asset ? `?asset=${encodeURIComponent(asset)}` : '';
      return requestJson<LedgerOverview>(`/api/v1/admin/ledger/overview${params}`);
    },
    topUpInsuranceFund(input: InsuranceFundTopUpRequest): Promise<InsuranceFundTopUpResult> {
      return requestJson<InsuranceFundTopUpResult>('/api/v1/admin/ledger/insurance-fund/topups', {
        method: 'POST',
        body: JSON.stringify(input),
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('iftop'),
        },
      });
    },
    getLatestLedgerAudit(asset?: string): Promise<LedgerAuditReport> {
      const params = asset ? `?asset=${encodeURIComponent(asset)}` : '';
      return requestJson<LedgerAuditReport>(`/api/v1/admin/ledger/audits/latest${params}`);
    },
    runLedgerAudit(asset?: string): Promise<LedgerAuditReport> {
      const params = asset ? `?asset=${encodeURIComponent(asset)}` : '';
      return requestJson<LedgerAuditReport>(`/api/v1/admin/ledger/audits/run${params}`, {
        method: 'POST',
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('audit'),
        },
      });
    },
    downloadLatestLedgerAudit(format: 'json' | 'csv', asset?: string): Promise<Blob> {
      const search = new URLSearchParams();
      search.set('format', format);
      if (asset) {
        search.set('asset', asset);
      }
      return requestBlob(`/api/v1/admin/ledger/audits/latest/export?${search.toString()}`);
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
    returnWithdrawalToReview(withdrawId: string): Promise<{ withdraw_id: string; status: string }> {
      return requestJson<{ withdraw_id: string; status: string }>(`/api/v1/admin/withdrawals/${withdrawId}/review`, {
        method: 'POST',
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('wdreview'),
        },
      });
    },
    refundWithdrawal(withdrawId: string): Promise<{ withdraw_id: string; status: string }> {
      return requestJson<{ withdraw_id: string; status: string }>(`/api/v1/admin/withdrawals/${withdrawId}/refund`, {
        method: 'POST',
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('wdrefund'),
        },
      });
    },
  },

  funding: {
    getHistory(): Promise<FundingItem[]> {
      return api.account.getFundingHistory();
    },
  },

  transfers: {
    getHistory(): Promise<TransferItem[]> {
      return requestJson<TransferItem[]>('/api/v1/account/transfers');
    },
    create(input: TransferRequest): Promise<void> {
      return requestJson<void>('/api/v1/account/transfer', {
        method: 'POST',
        body: JSON.stringify(input),
        headers: {
          'X-Trace-Id': buildTraceId(),
          'Idempotency-Key': buildId('transfer'),
        },
      });
    },
  },
};
