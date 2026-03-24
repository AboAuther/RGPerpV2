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

type WithdrawItem = {
  withdraw_id: string;
  amount: string;
  fee_amount: string;
  status: string;
  tx_hash?: string | null;
};

type DepositAddress = {
  address: string;
};

const apiBaseUrl = 'http://127.0.0.1:8080';
const contractsEnvPath = existsSync(join(process.cwd(), '..', 'deploy', 'env', 'local-chains.env'))
  ? join(process.cwd(), '..', 'deploy', 'env', 'local-chains.env')
  : join(process.cwd(), '..', '.local', 'contracts.env');
const adminAddress = '0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266';
const adminPrivateKey = '0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80';
const userAddress = '0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC';
const userPrivateKey = '0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a';
const recipientAddress = '0x90F79bf6EB2c4f870365E785982E1f101E93b906';

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
  headers.set('X-Trace-Id', `pw_${Date.now()}`);
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

async function getBalances(token: string): Promise<Array<{ account_code: string; asset: string; balance: string }>> {
  return api('/api/v1/account/balances', undefined, token);
}

async function getWalletBalance(token: string): Promise<number> {
  const balances = await getBalances(token);
  const item = balances.find((entry) => entry.account_code === 'USER_WALLET' && entry.asset === 'USDC');
  return Number(item?.balance || '0');
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

function usdcBalanceOf(rpcUrl: string, tokenAddress: string, holder: string): bigint {
  const raw = execFileSync(
    'cast',
    ['call', tokenAddress, 'balanceOf(address)(uint256)', holder, '--rpc-url', rpcUrl],
    { encoding: 'utf8' },
  ).trim().split(/\s+/)[0];
  return BigInt(raw);
}

async function listWithdrawals(token: string): Promise<WithdrawItem[]> {
  return api('/api/v1/wallet/withdrawals', undefined, token);
}

async function waitForWithdrawStatus(token: string, withdrawId: string, status: string): Promise<WithdrawItem> {
  for (let attempt = 0; attempt < 120; attempt += 1) {
    const item = (await listWithdrawals(token)).find((entry) => entry.withdraw_id === withdrawId);
    if (item?.status === status) {
      return item;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`withdraw ${withdrawId} did not reach ${status}`);
}

async function approveWithdrawal(token: string, withdrawId: string): Promise<void> {
  await api(`/api/v1/admin/withdrawals/${withdrawId}/approve`, { method: 'POST' }, token);
}

async function explorerHasTx(token: string, txHash: string): Promise<void> {
  for (let attempt = 0; attempt < 60; attempt += 1) {
    const events = await api<Array<{ chain_tx_hash?: string | null }>>('/api/v1/explorer/events', undefined, token);
    if (events.some((event) => event.chain_tx_hash?.toLowerCase() === txHash.toLowerCase())) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`explorer did not index ${txHash}`);
}

test('withdrawal page completes auto and review paths against real backend', async ({ page }) => {
  const env = envMap();
  const rpcUrl = env.get('BASE_RPC_URL_HOST') || env.get('BASE_RPC_URL') || 'http://127.0.0.1:8545';
  const usdcAddress = env.get('BASE_USDC_ADDRESS');
  if (!usdcAddress) {
    throw new Error('BASE_USDC_ADDRESS missing');
  }

  const userSession = await login(userAddress, userPrivateKey, 'pw-user');
  const adminSession = await login(adminAddress, adminPrivateKey, 'pw-admin');
  const initialBalance = await getWalletBalance(userSession.access_token);
  const recipientBefore = usdcBalanceOf(rpcUrl, usdcAddress, recipientAddress);
  const existingWithdrawalIds = new Set((await listWithdrawals(userSession.access_token)).map((item) => item.withdraw_id));

  const depositAddress = await generateDepositAddress(userSession.access_token);
  castSend(['send', usdcAddress, 'mint(address,uint256)', depositAddress.address, '15000000000', '--rpc-url', rpcUrl, '--private-key', adminPrivateKey]);
  castSend(['send', depositAddress.address, 'forward()', '--rpc-url', rpcUrl, '--private-key', adminPrivateKey]);
  await waitForWalletBalance(userSession.access_token, initialBalance + 15000);

  await page.addInitScript((session) => {
    window.sessionStorage.setItem('rgperp.session', JSON.stringify(session));
  }, {
    accessToken: userSession.access_token,
    refreshToken: userSession.refresh_token,
    expiresAt: userSession.expires_at,
    user: userSession.user,
  });

  await page.goto('/wallet/withdraw');
  await expect(page.getByText('Create Withdrawal')).toBeVisible();
  await expect(page.getByText('USDC 可提现余额')).toBeVisible();

  const amountInput = page.getByPlaceholder('100.00');
  const addressInput = page.getByPlaceholder('0x...');
  const submitButton = page.getByRole('button', { name: '提交提现申请' });
  const refreshButton = page.getByRole('button', { name: '刷新状态' });

  await amountInput.fill('10');
  await addressInput.fill(recipientAddress);
  await submitButton.click();
  await expect(page.getByText('提现申请已提交')).toBeVisible();

  let autoWithdraw = await waitForWithdrawStatus(
    userSession.access_token,
    (await (async () => {
      for (let attempt = 0; attempt < 30; attempt += 1) {
        const newItem = (await listWithdrawals(userSession.access_token)).find((item) => !existingWithdrawalIds.has(item.withdraw_id));
        if (newItem) {
          existingWithdrawalIds.add(newItem.withdraw_id);
          return newItem.withdraw_id;
        }
        await new Promise((resolve) => setTimeout(resolve, 500));
      }
      throw new Error('auto withdraw record was not created');
    })()),
    'COMPLETED',
  );
  await refreshButton.click();
  await expect(page.getByText('COMPLETED', { exact: true }).first()).toBeVisible();
  if (!autoWithdraw.tx_hash) {
    throw new Error('auto withdraw missing tx hash');
  }
  await explorerHasTx(userSession.access_token, autoWithdraw.tx_hash);

  await amountInput.fill('10001');
  await addressInput.fill(recipientAddress);
  await submitButton.click();
  await expect(page.getByText('提现申请已提交')).toBeVisible();

  const reviewWithdrawId = await (async () => {
    for (let attempt = 0; attempt < 30; attempt += 1) {
      const newItem = (await listWithdrawals(userSession.access_token)).find((item) => !existingWithdrawalIds.has(item.withdraw_id));
      if (newItem) {
        existingWithdrawalIds.add(newItem.withdraw_id);
        return newItem.withdraw_id;
      }
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
    throw new Error('review withdraw record was not created');
  })();

  await waitForWithdrawStatus(userSession.access_token, reviewWithdrawId, 'RISK_REVIEW');
  await refreshButton.click();
  await expect(page.getByText('RISK_REVIEW', { exact: true }).first()).toBeVisible();

  await approveWithdrawal(adminSession.access_token, reviewWithdrawId);
  const reviewWithdraw = await waitForWithdrawStatus(userSession.access_token, reviewWithdrawId, 'COMPLETED');
  await refreshButton.click();
  await expect(page.getByText('COMPLETED', { exact: true }).first()).toBeVisible();
  if (!reviewWithdraw.tx_hash) {
    throw new Error('review withdraw missing tx hash');
  }
  await explorerHasTx(userSession.access_token, reviewWithdraw.tx_hash);

  const recipientAfter = usdcBalanceOf(rpcUrl, usdcAddress, recipientAddress);
  expect(recipientAfter - recipientBefore).toBe(10_009_000_000n);

  await page.goto('/explorer');
  await page.getByPlaceholder(/搜索 event_id/).fill(reviewWithdraw.tx_hash);
  await expect(page.getByText(reviewWithdraw.tx_hash.slice(0, 8), { exact: false }).first()).toBeVisible();
  await expect(page.getByText('10,001 USDC').first()).toBeVisible();
});
