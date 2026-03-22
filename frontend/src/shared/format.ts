import type { SystemChainItem } from './domain';

function toNumber(input: string | number | null | undefined): number {
  if (input == null) {
    return 0;
  }
  const parsed = typeof input === 'number' ? input : Number(input);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function formatUsd(input: string | number | null | undefined, digits = 2): string {
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(toNumber(input));
}

export function formatDecimal(input: string | number | null | undefined, digits = 4): string {
  return new Intl.NumberFormat('zh-CN', {
    minimumFractionDigits: 0,
    maximumFractionDigits: digits,
  }).format(toNumber(input));
}

export function formatSignedUsd(input: string | number | null | undefined, digits = 2): string {
  const value = toNumber(input);
  const prefix = value > 0 ? '+' : '';
  return `${prefix}${formatUsd(value, digits)}`;
}

export function formatPercent(input: string | number | null | undefined, digits = 2): string {
  const value = toNumber(input);
  return `${(value * 100).toFixed(digits)}%`;
}

export function formatAddress(address: string | null | undefined, size = 6): string {
  if (!address) {
    return '--';
  }
  if (address.length <= size * 2) {
    return address;
  }
  return `${address.slice(0, size)}...${address.slice(-size)}`;
}

export function formatDateTime(input: string | undefined): string {
  if (!input) {
    return '--';
  }
  const date = new Date(input);
  if (Number.isNaN(date.getTime())) {
    return '--';
  }
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date);
}

export function formatChainName(chainId: number, chains?: SystemChainItem[]): string {
  return chains?.find((chain) => chain.chain_id === chainId)?.name ?? `Chain ${chainId}`;
}

export function parseAmount(input: string | undefined): number {
  return toNumber(input ?? 0);
}
