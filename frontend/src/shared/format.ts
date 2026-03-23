import type { SystemChainItem } from './domain';

function toNumber(input: string | number | null | undefined): number {
  if (input == null) {
    return 0;
  }
  const parsed = typeof input === 'number' ? input : Number(input);
  return Number.isFinite(parsed) ? parsed : 0;
}

function normalizeNumericInput(input: string | number | null | undefined): string {
  if (input == null) {
    return '0';
  }
  if (typeof input === 'string') {
    const normalized = input.trim();
    return normalized || '0';
  }
  if (!Number.isFinite(input)) {
    return '0';
  }
  if (Number.isInteger(input)) {
    return input.toString();
  }
  return input.toFixed(12).replace(/0+$/, '').replace(/\.$/, '');
}

function resolveUsdFractionDigits(input: string | number | null | undefined, minimumDigits: number, maximumDigits: number): number {
  const normalized = normalizeNumericInput(input);
  const [, rawFraction = ''] = normalized.split('.');
  const fraction = rawFraction.replace(/0+$/, '');
  if (!fraction) {
    return minimumDigits;
  }
  return Math.min(maximumDigits, Math.max(minimumDigits, fraction.length));
}

export function formatUsd(input: string | number | null | undefined, maximumDigits = 6): string {
  const fractionDigits = resolveUsdFractionDigits(input, 2, maximumDigits);
  return new Intl.NumberFormat('zh-CN', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: fractionDigits,
  }).format(toNumber(input));
}

export function formatUsdAdaptive(input: string | number | null | undefined, maxDigits = 6): string {
  return formatUsd(input, maxDigits);
}

export function formatDecimal(input: string | number | null | undefined, digits = 4): string {
  return new Intl.NumberFormat('zh-CN', {
    minimumFractionDigits: 0,
    maximumFractionDigits: digits,
  }).format(toNumber(input));
}

export function formatDecimalAdaptive(input: string | number | null | undefined, maximumDigits = 8, minimumDigits = 0): string {
  const fractionDigits = resolveUsdFractionDigits(input, minimumDigits, maximumDigits);
  return new Intl.NumberFormat('zh-CN', {
    minimumFractionDigits: minimumDigits,
    maximumFractionDigits: fractionDigits,
  }).format(toNumber(input));
}

export function formatSignedUsd(input: string | number | null | undefined, maximumDigits = 6): string {
  const value = toNumber(input);
  const prefix = value > 0 ? '+' : '';
  return `${prefix}${formatUsd(input, maximumDigits)}`;
}

export function formatSignedUsdAdaptive(input: string | number | null | undefined, maxDigits = 6): string {
  const value = toNumber(input);
  const prefix = value > 0 ? '+' : '';
  return `${prefix}${formatUsdAdaptive(value, maxDigits)}`;
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
