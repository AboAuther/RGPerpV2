import type { ApiProvider, AppEnv, ChainOption } from './domain';

const fallbackChains: ChainOption[] = [
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

function parseProvider(value: string): ApiProvider {
  if (value === 'http' || value === 'auto' || value === 'mock') {
    return value;
  }
  throw new Error(`不支持的 VITE_API_PROVIDER=${value}`);
}

function parseBool(value: string | undefined, fallback = false): boolean {
  if (value == null || value === '') {
    return fallback;
  }
  return value === 'true';
}

function parseEnv(value: string): AppEnv {
  if (value === 'dev' || value === 'staging' || value === 'prod' || value === 'review') {
    return value;
  }
  throw new Error(`不支持的 VITE_APP_ENV=${value}`);
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

function isReviewLikeEnv(env: AppEnv): boolean {
  return env === 'dev' || env === 'review';
}

const appEnv = parseEnv(requireEnv('VITE_APP_ENV'));
const apiProvider = parseProvider(requireEnv('VITE_API_PROVIDER'));
const reviewFeaturesEnabled = isReviewLikeEnv(appEnv);

if (apiProvider === 'mock' && !reviewFeaturesEnabled) {
  throw new Error('VITE_API_PROVIDER=mock 仅允许在 dev/review 环境启用');
}

const disableRouteGuard = parseBool(import.meta.env.VITE_DISABLE_ROUTE_GUARD, false);
if (disableRouteGuard && !reviewFeaturesEnabled) {
  throw new Error('VITE_DISABLE_ROUTE_GUARD=true 仅允许在 dev/review 环境启用');
}

const reviewFaucetEnabled = reviewFeaturesEnabled && parseBool(import.meta.env.VITE_REVIEW_FAUCET_ENABLED, false);

export const appConfig = {
  appEnv,
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080',
  wsBaseUrl: import.meta.env.VITE_WS_BASE_URL || 'ws://localhost:8080/ws',
  apiProvider,
  disableRouteGuard,
  reviewFaucetEnabled,
  reviewFeaturesEnabled,
  mockSessionPersistenceEnabled: reviewFeaturesEnabled && apiProvider === 'mock',
  supportedChains: parseChains(import.meta.env.VITE_SUPPORTED_CHAINS),
};

export function getChainOption(chainId: number): ChainOption | undefined {
  return appConfig.supportedChains.find((chain) => chain.id === chainId);
}
