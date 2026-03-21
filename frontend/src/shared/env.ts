import type { ApiProvider, AppEnv, ChainOption } from './domain';

const fallbackChains: ChainOption[] = [
  { id: 1, key: 'ethereum', name: 'Ethereum', confirmations: 12 },
  { id: 42161, key: 'arbitrum', name: 'Arbitrum', confirmations: 20 },
  { id: 8453, key: 'base', name: 'Base', confirmations: 20 },
];

function parseProvider(value: string | undefined): ApiProvider {
  if (value === 'http' || value === 'auto' || value === 'mock') {
    return value;
  }
  return 'mock';
}

function parseBool(value: string | undefined, fallback = false): boolean {
  if (value == null || value === '') {
    return fallback;
  }
  return value === 'true';
}

function parseEnv(value: string | undefined): AppEnv {
  if (value === 'dev' || value === 'staging' || value === 'prod' || value === 'review') {
    return value;
  }
  return 'review';
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
  appEnv: parseEnv(import.meta.env.VITE_APP_ENV),
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080',
  wsBaseUrl: import.meta.env.VITE_WS_BASE_URL || 'ws://localhost:8080/ws',
  apiProvider: parseProvider(import.meta.env.VITE_API_PROVIDER),
  disableRouteGuard: parseBool(import.meta.env.VITE_DISABLE_ROUTE_GUARD, false),
  reviewFaucetEnabled: parseBool(import.meta.env.VITE_REVIEW_FAUCET_ENABLED, true),
  supportedChains: parseChains(import.meta.env.VITE_SUPPORTED_CHAINS),
};

export function getChainOption(chainId: number): ChainOption | undefined {
  return appConfig.supportedChains.find((chain) => chain.id === chainId);
}
