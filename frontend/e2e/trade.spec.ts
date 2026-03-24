import { expect, test } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

type LoginData = {
  access_token: string;
  refresh_token: string;
  expires_at?: string;
  user: {
    id: number;
    evm_address: string;
    status: string;
  };
};

type DepositAddress = {
  address: string;
};

type PositionItem = {
  position_id: string;
  symbol: string;
  side: string;
  qty: string;
  status: string;
};

type OrderItem = {
  order_id: string;
  client_order_id: string;
  symbol: string;
  side: string;
  position_effect: string;
  type: string;
  qty: string;
  status: string;
};

type FillItem = {
  fill_id: string;
  symbol: string;
};

type SymbolItem = {
  symbol: string;
  tick_size: string;
};

type TickerItem = {
  symbol: string;
  mark_price: string;
  stale: boolean;
};

type ExplorerEvent = {
  event_id: string;
  event_type: string;
  order_id?: string | null;
  fill_id?: string | null;
};

const apiBaseUrl = 'http://127.0.0.1:8080';
const contractsEnvPath = existsSync(join(process.cwd(), '..', 'deploy', 'env', 'local-chains.env'))
  ? join(process.cwd(), '..', 'deploy', 'env', 'local-chains.env')
  : join(process.cwd(), '..', '.local', 'contracts.env');
const adminPrivateKey = '0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80';
const userAddress = '0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC';
const userPrivateKey = '0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a';

function envMap(): Map<string, string> {
  const raw = readFileSync(contractsEnvPath, 'utf8');
  const map = new Map<string, string>();
  for (const line of raw.split('\n')) {
    const match = line.match(/^export\s+([A-Z0-9_]+)=(.+)$/);
    if (!match) {
      continue;
    }
    map.set(match[1], match[2]);
  }
  return map;
}

async function api<T>(path: string, init?: RequestInit, token?: string): Promise<T> {
  const headers = new Headers(init?.headers);
  if (!headers.has('Accept')) {
    headers.set('Accept', 'application/json');
  }
  if (init?.method && init.method !== 'GET' && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  headers.set('X-Trace-Id', `pw_trade_${Date.now()}`);
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  const response = await fetch(`${apiBaseUrl}${path}`, {
    ...init,
    headers,
  });
  const body = (await response.json()) as { message: string; data: T };
  if (!response.ok) {
    throw new Error(body.message);
  }
  return body.data;
}

async function login(address: string, privateKey: string, fingerprint: string): Promise<LoginData> {
  const challenge = await api<{ nonce: string; message: string }>('/api/v1/auth/challenge', {
    method: 'POST',
    body: JSON.stringify({ address, chain_id: 31337 }),
  });
  const signature = execFileSync('cast', ['wallet', 'sign', '--private-key', privateKey, challenge.message], {
    encoding: 'utf8',
  }).trim();
  return api<LoginData>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify({
      address,
      chain_id: 31337,
      nonce: challenge.nonce,
      signature,
      device_fingerprint: fingerprint,
    }),
  });
}

async function getWalletBalance(token: string): Promise<number> {
  const balances = await api<Array<{ account_code: string; asset: string; balance: string }>>('/api/v1/account/balances', undefined, token);
  const wallet = balances.find((item) => item.account_code === 'USER_WALLET' && item.asset === 'USDC');
  return Number(wallet?.balance || '0');
}

async function waitForWalletBalance(token: string, minimum: number): Promise<void> {
  for (let attempt = 0; attempt < 90; attempt += 1) {
    if ((await getWalletBalance(token)) >= minimum) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`wallet balance did not reach ${minimum}`);
}

async function generateDepositAddress(token: string): Promise<DepositAddress> {
  return api(`/api/v1/wallet/deposit-addresses/31337/generate`, { method: 'POST' }, token);
}

function castSend(args: string[]) {
  execFileSync('cast', args, { encoding: 'utf8' });
}

async function listPositions(token: string): Promise<PositionItem[]> {
  return api('/api/v1/positions', undefined, token);
}

async function listOrders(token: string): Promise<OrderItem[]> {
  return api('/api/v1/orders', undefined, token);
}

async function listFills(token: string): Promise<FillItem[]> {
  return api('/api/v1/fills', undefined, token);
}

async function listSymbols(): Promise<SymbolItem[]> {
  return api('/api/v1/markets/symbols');
}

async function listTickers(): Promise<TickerItem[]> {
  return api('/api/v1/markets/tickers');
}

async function waitForFreshTicker(symbol: string): Promise<TickerItem> {
  for (let attempt = 0; attempt < 30; attempt += 1) {
    const ticker = (await listTickers()).find((item) => item.symbol === symbol);
    if (ticker && !ticker.stale) {
      return ticker;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`ticker ${symbol} remained stale`);
}

async function listExplorerEvents(token: string): Promise<ExplorerEvent[]> {
  return api('/api/v1/explorer/events', undefined, token);
}

async function cancelOrder(token: string, orderId: string): Promise<void> {
  await api(`/api/v1/orders/${orderId}/cancel`, { method: 'POST' }, token);
}

async function createOrder(token: string, payload: Record<string, unknown>): Promise<void> {
  await api('/api/v1/orders', {
    method: 'POST',
    body: JSON.stringify(payload),
  }, token);
}

function decimalScale(input: string): number {
  const normalized = input.trim();
  if (!normalized) {
    return 0;
  }
  const [, fraction = ''] = normalized.split('.');
  return fraction.length;
}

function decimalToScaledBigInt(input: string, scale: number): bigint | null {
  const normalized = input.trim();
  if (!/^\d+(\.\d+)?$/.test(normalized)) {
    return null;
  }
  const [integerPart, fractionPart = ''] = normalized.split('.');
  const paddedFraction = `${fractionPart}${'0'.repeat(scale)}`.slice(0, scale);
  return BigInt(`${integerPart}${paddedFraction}`);
}

function scaledBigIntToDecimal(value: bigint, scale: number): string {
  if (scale <= 0) {
    return value.toString();
  }
  const digits = value.toString().padStart(scale + 1, '0');
  const integerPart = digits.slice(0, digits.length - scale) || '0';
  const fractionPart = digits.slice(digits.length - scale).replace(/0+$/, '');
  return fractionPart ? `${integerPart}.${fractionPart}` : integerPart;
}

function alignDownToStep(value: string, step: string): string {
  const scale = Math.max(decimalScale(value), decimalScale(step));
  const scaledValue = decimalToScaledBigInt(value, scale);
  const scaledStep = decimalToScaledBigInt(step, scale);
  if (scaledValue === null || scaledStep === null || scaledStep <= 0n) {
    throw new Error(`cannot align value=${value} step=${step}`);
  }
  const aligned = (scaledValue / scaledStep) * scaledStep;
  return scaledBigIntToDecimal(aligned, scale);
}

test('trade page submits open, resting limit, cancel and close against real backend', async ({ page }) => {
  const env = envMap();
  const rpcUrl = env.get('BASE_RPC_URL_HOST') || env.get('BASE_RPC_URL') || 'http://127.0.0.1:8545';
  const usdcAddress = env.get('BASE_USDC_ADDRESS');
  if (!usdcAddress) {
    throw new Error('BASE_USDC_ADDRESS missing');
  }

  const session = await login(userAddress, userPrivateKey, 'pw-trade-user');
  const initialWallet = await getWalletBalance(session.access_token);
  const depositAddress = await generateDepositAddress(session.access_token);
  castSend(['send', usdcAddress, 'mint(address,uint256)', depositAddress.address, '3000000000', '--rpc-url', rpcUrl, '--private-key', adminPrivateKey]);
  castSend(['send', depositAddress.address, 'forward()', '--rpc-url', rpcUrl, '--private-key', adminPrivateKey]);
  await waitForWalletBalance(session.access_token, initialWallet + 3000);

  for (const order of await listOrders(session.access_token)) {
    if (order.status === 'RESTING' || order.status === 'TRIGGER_WAIT') {
      await cancelOrder(session.access_token, order.order_id);
    }
  }
  for (const position of await listPositions(session.access_token)) {
    if (position.status !== 'OPEN') {
      continue;
    }
    await createOrder(session.access_token, {
      client_order_id: `pw_cleanup_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`,
      symbol: position.symbol,
      side: position.side === 'LONG' ? 'SELL' : 'BUY',
      position_effect: 'CLOSE',
      type: 'MARKET',
      qty: position.qty,
      reduce_only: true,
    });
  }

  const existingOrders = new Set((await listOrders(session.access_token)).map((item) => item.order_id));
  const existingFills = new Set((await listFills(session.access_token)).map((item) => item.fill_id));

  await page.addInitScript((storedSession) => {
    window.sessionStorage.setItem('rgperp.session', JSON.stringify(storedSession));
  }, {
    accessToken: session.access_token,
    refreshToken: session.refresh_token,
    expiresAt: session.expires_at,
    user: session.user,
  });

  await page.goto('/trade');
  await expect(page.getByText('Trade Console')).toBeVisible();
  await expect(page.getByText(/^Order Entry · /)).toBeVisible();

  await waitForFreshTicker('BTC-USDC');
  await createOrder(session.access_token, {
    client_order_id: `pw_market_open_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`,
    symbol: 'BTC-USDC',
    side: 'BUY',
    position_effect: 'OPEN',
    type: 'MARKET',
    qty: '0.001',
    reduce_only: false,
    max_slippage_bps: 100,
  });

  await expect.poll(async () => {
    const positions = await listPositions(session.access_token);
    return positions.some((item) => item.symbol === 'BTC-USDC' && item.status === 'OPEN');
  }, { timeout: 15000 }).toBeTruthy();

  await page.getByRole('button', { name: '刷新交易数据' }).click();
  await expect(page.getByText('LONG').first()).toBeVisible();

  const btcSymbol = (await listSymbols()).find((item) => item.symbol === 'BTC-USDC');
  const btcTicker = await waitForFreshTicker('BTC-USDC');
  if (!btcSymbol || !btcTicker) {
    throw new Error('BTC-USDC market metadata missing');
  }
  const restingLimitPrice = alignDownToStep((Number(btcTicker.mark_price) * 0.5).toFixed(18), btcSymbol.tick_size);
  await createOrder(session.access_token, {
    client_order_id: `pw_limit_open_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`,
    symbol: 'BTC-USDC',
    side: 'BUY',
    position_effect: 'OPEN',
    type: 'LIMIT',
    qty: '0.002',
    price: restingLimitPrice,
    reduce_only: false,
    time_in_force: 'GTC',
    max_slippage_bps: 100,
  });

  let createdLimitOrder = '';
  await expect.poll(async () => {
    const orders = await listOrders(session.access_token);
    createdLimitOrder = orders.find((item) => !existingOrders.has(item.order_id) && item.symbol === 'BTC-USDC' && item.status === 'RESTING')?.order_id || '';
    return createdLimitOrder;
  }, { timeout: 15000 }).not.toBe('');

  await page.getByRole('tab', { name: 'Open Orders' }).click();
  await page.getByRole('button', { name: '刷新交易数据' }).click();
  await expect(page.getByText('Limit').first()).toBeVisible();
  await expect(page.getByText('BTC-USDC').first()).toBeVisible();
  const cancelButton = page.getByRole('button', { name: /撤\s*单|Cancel/ }).first();
  await expect(cancelButton).toBeEnabled({ timeout: 15000 });
  await cancelButton.click();

  await expect.poll(async () => {
    const orders = await listOrders(session.access_token);
    return orders.find((item) => item.order_id === createdLimitOrder)?.status || '';
  }, { timeout: 15000 }).toBe('CANCELED');
  await page.getByRole('tab', { name: 'Order History' }).click();
  await page.getByRole('button', { name: '刷新交易数据' }).click();
  await expect(page.getByText('CANCELED').first()).toBeVisible();

  const stopTriggerPrice = alignDownToStep((Number(btcTicker.mark_price) * 1.5).toFixed(18), btcSymbol.tick_size);
  await createOrder(session.access_token, {
    client_order_id: `pw_stop_open_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`,
    symbol: 'BTC-USDC',
    side: 'BUY',
    position_effect: 'OPEN',
    type: 'STOP_MARKET',
    qty: '0.002',
    trigger_price: stopTriggerPrice,
    reduce_only: false,
    max_slippage_bps: 100,
  });

  let createdTriggerOrder = '';
  await expect.poll(async () => {
    const orders = await listOrders(session.access_token);
    createdTriggerOrder =
      orders.find((item) => !existingOrders.has(item.order_id) && item.symbol === 'BTC-USDC' && item.status === 'TRIGGER_WAIT' && item.type === 'STOP_MARKET')
        ?.order_id || '';
    return createdTriggerOrder;
  }, { timeout: 15000 }).not.toBe('');

  await page.getByRole('tab', { name: 'Open Orders' }).click();
  await page.getByRole('button', { name: '刷新交易数据' }).click();
  await expect(page.getByText('Stop Market').first()).toBeVisible();
  await expect(page.getByText('BTC-USDC').first()).toBeVisible();
  const triggerCancelButton = page.getByRole('button', { name: /撤\s*单|Cancel/ }).first();
  await expect(triggerCancelButton).toBeEnabled({ timeout: 15000 });
  await triggerCancelButton.click();

  await expect.poll(async () => {
    const orders = await listOrders(session.access_token);
    return orders.find((item) => item.order_id === createdTriggerOrder)?.status || '';
  }, { timeout: 15000 }).toBe('CANCELED');

  const openPosition = (await listPositions(session.access_token)).find((item) => item.symbol === 'BTC-USDC' && item.status === 'OPEN');
  if (!openPosition) {
    throw new Error('expected open position before close');
  }
  await createOrder(session.access_token, {
    client_order_id: `pw_market_close_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`,
    symbol: 'BTC-USDC',
    side: openPosition.side === 'LONG' ? 'SELL' : 'BUY',
    position_effect: 'CLOSE',
    type: 'MARKET',
    qty: openPosition.qty,
    reduce_only: true,
    max_slippage_bps: 100,
  });

  await expect.poll(async () => {
    const positions = await listPositions(session.access_token);
    return positions.some((item) => item.symbol === 'BTC-USDC' && item.status === 'OPEN');
  }, { timeout: 15000 }).toBeFalsy();

  await expect.poll(async () => {
    const fills = await listFills(session.access_token);
    return fills.filter((item) => !existingFills.has(item.fill_id) && item.symbol === 'BTC-USDC').length;
  }).toBeGreaterThanOrEqual(2);

  await page.goto('/history/positions');
  await expect(page.getByText('Positions')).toBeVisible();
  await expect(page.getByText('BTC-USDC').first()).toBeVisible();

  const explorerEvents = await listExplorerEvents(session.access_token);
  const tradeAcceptedEvent = explorerEvents.find((item) => item.event_type === 'trade.order.accepted' && item.order_id === createdLimitOrder);
  expect(tradeAcceptedEvent).toBeTruthy();

  await page.goto('/explorer');
  await expect(page.getByText('Event Explorer')).toBeVisible();
  await page.getByPlaceholder(/搜索 event_id/).fill(createdLimitOrder);
  await expect(page.getByText(createdLimitOrder.slice(0, 6), { exact: false }).first()).toBeVisible();
});
