import type { AppEnv, ChainOption } from './domain';

const fallbackChains: ChainOption[] = [
  { id: 31337, key: 'local', name: 'Local Anvil', confirmations: 1 },
  { id: 1, key: 'ethereum', name: 'Ethereum', confirmations: 12 },
  { id: 42161, key: 'arbitrum', name: 'Arbitrum', confirmations: 20 },
  { id: 8453, key: 'base', name: 'Base', confirmations: 20 },
];

function requireEnv(name: string): string {
  const value = import.meta.env[name];
  if (typeof value === 'string' && value.trim()) {
    return value.trim();
  }
  throw new Error(`缺少必填前端环境变量 ${name}`);
}

function parseEnv(value: string): AppEnv {
  if (value === 'dev' || value === 'staging' || value === 'prod') {
    return value;
  }
  throw new Error(`不支持的 VITE_APP_ENV=${value}`);
}

function parseIntEnv(name: string, fallback: number): number {
  const raw = import.meta.env[name];
  if (raw == null || raw === '') {
    return fallback;
  }
  const value = Number.parseInt(raw, 10);
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`环境变量 ${name} 必须是正整数`);
  }
  return value;
}

function parseChains(value: string | undefined): ChainOption[] {
  if (!value) {
    return fallbackChains;
  }

  const allow = new Set(
    value
      .split(',')
      .map((item) => item.trim().toLowerCase())
      .filter(Boolean),
  );

  const selected = fallbackChains.filter((chain) => allow.has(chain.key));
  return selected.length > 0 ? selected : fallbackChains;
}

export const appConfig = {
  appEnv: parseEnv(requireEnv('VITE_APP_ENV')),
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || 'http://127.0.0.1:8080',
  wsBaseUrl: import.meta.env.VITE_WS_BASE_URL || 'ws://127.0.0.1:8080/ws',
  supportedChains: parseChains(import.meta.env.VITE_SUPPORTED_CHAINS),
  localChainId: parseIntEnv('VITE_LOCAL_CHAIN_ID', 31337),
  localUsdcAddress: (import.meta.env.VITE_LOCAL_USDC_ADDRESS || '').trim(),
};

export function getChainOption(chainId: number): ChainOption | undefined {
  return appConfig.supportedChains.find((chain) => chain.id === chainId);
}

