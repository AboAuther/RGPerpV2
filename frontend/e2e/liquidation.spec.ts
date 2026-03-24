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

type PositionItem = {
  position_id: string;
  symbol: string;
  side: string;
  qty: string;
  status: string;
};

type OrderItem = {
  order_id: string;
  symbol: string;
  side: string;
  status: string;
  qty: string;
};

type RiskSnapshot = {
  risk_state: string;
  can_open_risk: boolean;
};

const apiBaseUrl = 'http://127.0.0.1:8080';
const rootDir = join(process.cwd(), '..');
const contractsEnvPath = existsSync(join(process.cwd(), '..', 'deploy', 'env', 'local-chains.env'))
  ? join(process.cwd(), '..', 'deploy', 'env', 'local-chains.env')
  : join(process.cwd(), '..', '.local', 'contracts.env');
const adminAddress = '0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266';
const adminPrivateKey = '0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80';
const liquidationUserAddress = '0x9965507D1a55bcC2695C58ba16FB37d819B0A4dc';
const liquidationUserPrivateKey = '0x8b3a350cf5c34c9194ca85829a2df0ec3153be0318b5e2d3348e872092edffba';

type DepositAddress = {
  address: string;
};

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
  headers.set('X-Trace-Id', `pw_liquidation_${Date.now()}`);
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

function mysqlExec(sql: string): string {
  return execFileSync(
    'docker',
    ['compose', 'exec', '-T', 'mysql', 'mysql', '-uroot', '-proot', 'rgperp', '-N', '-e', sql],
    { cwd: rootDir, encoding: 'utf8' },
  ).trim();
}

async function listPositions(token: string): Promise<PositionItem[]> {
  return api('/api/v1/positions', undefined, token);
}

async function listOrders(token: string): Promise<OrderItem[]> {
  return api('/api/v1/orders', undefined, token);
}

async function listRisk(token: string): Promise<RiskSnapshot> {
  return api('/api/v1/account/risk', undefined, token);
}

async function createOrder(token: string, payload: Record<string, unknown>): Promise<void> {
  await api('/api/v1/orders', {
    method: 'POST',
    headers: {
      'Idempotency-Key': String(payload.client_order_id || `pw_${Date.now()}`),
    },
    body: JSON.stringify(payload),
  }, token);
}

async function cancelOrder(token: string, orderId: string): Promise<void> {
  await api(`/api/v1/orders/${orderId}/cancel`, { method: 'POST' }, token);
}

async function cleanupAccount(token: string): Promise<void> {
  for (const order of await listOrders(token)) {
    if (order.status === 'RESTING' || order.status === 'TRIGGER_WAIT') {
      await cancelOrder(token, order.order_id);
    }
  }
  for (const position of await listPositions(token)) {
    if (position.status !== 'OPEN') {
      continue;
    }
    await createOrder(token, {
      client_order_id: `pw_liq_cleanup_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`,
      symbol: position.symbol,
      side: position.side === 'LONG' ? 'SELL' : 'BUY',
      position_effect: 'CLOSE',
      type: 'MARKET',
      qty: position.qty,
      reduce_only: true,
      max_slippage_bps: 100,
    });
  }
}

function injectManualMark(symbol: string, price: string) {
  mysqlExec(
    `INSERT INTO mark_price_snapshots (symbol_id,index_price,mark_price,calc_version,created_at)
     SELECT id,'${price}','${price}',9001,DATE_ADD(UTC_TIMESTAMP(), INTERVAL 1 HOUR)
     FROM symbols WHERE symbol='${symbol}';`,
  );
}

async function queueAdminRecalculation(adminToken: string, userId: number) {
  return api<{ recalculation_status?: string }>(`/api/v1/admin/risk/accounts/${userId}/recalculate`, { method: 'POST' }, adminToken);
}

function latestLiquidationStatus(userId: number): string {
  return mysqlExec(
    `SELECT status
     FROM liquidations
     WHERE user_id=${userId}
     ORDER BY id DESC
     LIMIT 1;`,
  );
}

test('trade page reflects liquidation after forced mark-price shock', async ({ page }) => {
  const env = envMap();
  const rpcUrl = env.get('BASE_RPC_URL_HOST') || env.get('BASE_RPC_URL') || 'http://127.0.0.1:8545';
  const usdcAddress = env.get('BASE_USDC_ADDRESS');
  if (!usdcAddress) {
    throw new Error('BASE_USDC_ADDRESS missing');
  }

  const userSession = await login(liquidationUserAddress, liquidationUserPrivateKey, 'pw-liquidation-user');
  const adminSession = await login(adminAddress, adminPrivateKey, 'pw-liquidation-admin');

  await cleanupAccount(userSession.access_token);

  const initialWallet = await getWalletBalance(userSession.access_token);
  const depositAddress = await generateDepositAddress(userSession.access_token);
  castSend(['send', usdcAddress, 'mint(address,uint256)', depositAddress.address, '5000000000', '--rpc-url', rpcUrl, '--private-key', adminPrivateKey]);
  castSend(['send', depositAddress.address, 'forward()', '--rpc-url', rpcUrl, '--private-key', adminPrivateKey]);
  await waitForWalletBalance(userSession.access_token, initialWallet + 5000);

  await page.addInitScript((session) => {
    window.sessionStorage.setItem('rgperp.session', JSON.stringify(session));
  }, {
    accessToken: userSession.access_token,
    refreshToken: userSession.refresh_token,
    expiresAt: userSession.expires_at,
    user: userSession.user,
  });

  await page.goto('/trade');
  await expect(page.getByText('Trade Console')).toBeVisible();

  await createOrder(userSession.access_token, {
    client_order_id: `pw_liq_open_${crypto.randomUUID().replace(/-/g, '').slice(0, 16)}`,
    symbol: 'BTC-USDC',
    side: 'BUY',
    position_effect: 'OPEN',
    type: 'MARKET',
    qty: '0.14',
    leverage: '40',
    reduce_only: false,
    max_slippage_bps: 100,
  });

  await expect.poll(async () => {
    const positions = await listPositions(userSession.access_token);
    return positions.some((item) => item.symbol === 'BTC-USDC' && item.side === 'LONG' && item.status === 'OPEN');
  }, { timeout: 15_000 }).toBeTruthy();

  await page.getByRole('button', { name: '刷新交易数据' }).click();
  await expect(page.getByText('LONG').first()).toBeVisible();

  let liquidationStatus = '';
  for (let attempt = 0; attempt < 30; attempt += 1) {
    injectManualMark('BTC-USDC', '35900');
    await queueAdminRecalculation(adminSession.access_token, userSession.user.id);
    await new Promise((resolve) => setTimeout(resolve, 1000));

    const positions = await listPositions(userSession.access_token);
    liquidationStatus = latestLiquidationStatus(userSession.user.id);
    const stillOpen = positions.some((item) => item.symbol === 'BTC-USDC' && item.side === 'LONG' && item.status === 'OPEN');
    if (!stillOpen && liquidationStatus === 'EXECUTED') {
      break;
    }
  }

  expect(liquidationStatus).toBe('EXECUTED');

  await expect.poll(async () => {
    const positions = await listPositions(userSession.access_token);
    return positions.some((item) => item.symbol === 'BTC-USDC' && item.side === 'LONG' && item.status === 'OPEN');
  }, { timeout: 20_000 }).toBeFalsy();

  await expect.poll(async () => {
    const risk = await listRisk(userSession.access_token);
    return risk.risk_state;
  }, { timeout: 20_000 }).toBe('SAFE');

  await page.getByRole('button', { name: '刷新交易数据' }).click();
  await expect(page.getByText('No open positions yet')).toBeVisible();
});
