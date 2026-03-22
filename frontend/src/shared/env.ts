import type { AppEnv } from './domain';

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

function parseCSV(value: string | undefined): string[] {
  return String(value || '')
    .split(',')
    .map((item) => item.trim().toLowerCase())
    .filter(Boolean);
}

export const appConfig = {
  appEnv: parseEnv(requireEnv('VITE_APP_ENV')),
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || 'http://127.0.0.1:8080',
  wsBaseUrl: import.meta.env.VITE_WS_BASE_URL || 'ws://127.0.0.1:8080/ws',
  adminWallets: parseCSV(import.meta.env.VITE_ADMIN_WALLETS),
};
