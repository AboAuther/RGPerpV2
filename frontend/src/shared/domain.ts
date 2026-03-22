export type AppEnv = 'dev' | 'staging' | 'prod';

export interface ApiEnvelope<T> {
  code: string;
  message: string;
  trace_id: string;
  data: T;
}

export interface ChainOption {
  id: number;
  key: string;
  name: string;
  confirmations: number;
}

export interface SystemChainItem {
  chain_id: number;
  key: string;
  name: string;
  asset: string;
  confirmations: number;
  local_testnet: boolean;
  local_tools_enabled: boolean;
  deposit_enabled: boolean;
  withdraw_enabled: boolean;
  usdc_address?: string | null;
}

export interface User {
  id: number;
  evm_address: string;
  status: string;
  role?: string;
  is_admin?: boolean;
  capabilities?: string[];
}

export interface ChallengeResponse {
  nonce: string;
  message: string;
  domain: string;
  chain_id: number;
  expires_at: string;
}

export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  expires_at?: string;
  user: User;
}

export interface AccountSummary {
  equity: string;
  available_balance: string;
  total_initial_margin: string;
  total_maintenance_margin: string;
  unrealized_pnl: string;
  margin_ratio: string;
}

export interface BalanceItem {
  account_code: string;
  asset: string;
  balance: string;
}

export interface DepositAddressItem {
  chain_id: number;
  asset: string;
  address: string;
  confirmations: number;
}

export interface DepositItem {
  deposit_id: string;
  chain_id: number;
  asset: string;
  amount: string;
  tx_hash: string;
  confirmations: number;
  required_confirmations: number;
  status: 'DETECTED' | 'CONFIRMING' | 'CREDITED' | 'SWEEPING' | 'REORGED';
  address: string;
  detected_at: string;
}

export interface WithdrawRequest {
  chain_id: number;
  asset: string;
  amount: string;
  to_address: string;
}

export interface WithdrawItem {
  withdraw_id: string;
  chain_id: number;
  asset: string;
  amount: string;
  fee_amount: string;
  to_address: string;
  status:
    | 'REQUESTED'
    | 'HOLD'
    | 'RISK_REVIEW'
    | 'APPROVED'
    | 'SIGNING'
    | 'BROADCASTED'
    | 'CONFIRMING'
    | 'REJECTED'
    | 'CANCELED'
    | 'FAILED'
    | 'COMPLETED'
    | 'REFUNDED';
  tx_hash?: string | null;
  created_at?: string;
}

export interface SymbolItem {
  symbol: string;
  asset_class: string;
  tick_size: string;
  step_size: string;
  min_notional: string;
  status: string;
}

export interface TickerItem {
  symbol: string;
  index_price: string;
  mark_price: string;
  best_bid: string;
  best_ask: string;
  ts: string;
}

export interface OrderItem {
  order_id: string;
  client_order_id: string;
  symbol: string;
  side: string;
  position_effect: string;
  type: string;
  qty: string;
  filled_qty: string;
  avg_fill_price: string;
  price?: string | null;
  trigger_price?: string | null;
  reduce_only: boolean;
  status: string;
  reject_reason?: string | null;
}

export interface OrderCreateRequest {
  client_order_id: string;
  symbol: string;
  side: 'BUY' | 'SELL';
  position_effect: 'OPEN' | 'REDUCE' | 'CLOSE';
  type: 'MARKET' | 'LIMIT';
  qty: string;
  price?: string | null;
  trigger_price?: string | null;
  reduce_only: boolean;
  time_in_force?: 'GTC' | 'GTD';
  max_slippage_bps?: number;
}

export interface FillItem {
  fill_id: string;
  order_id: string;
  symbol: string;
  side: string;
  qty: string;
  price: string;
  fee_amount: string;
  created_at: string;
}

export interface PositionItem {
  position_id: string;
  symbol: string;
  side: string;
  qty: string;
  avg_entry_price: string;
  mark_price: string;
  initial_margin: string;
  maintenance_margin: string;
  realized_pnl: string;
  unrealized_pnl: string;
  funding_accrual: string;
  liquidation_price: string;
  status: string;
}

export interface ExplorerEvent {
  event_id: string;
  event_type: string;
  asset?: string | null;
  amount?: string | null;
  created_at: string;
  ledger_tx_id?: string | null;
  chain_tx_hash?: string | null;
  order_id?: string | null;
  fill_id?: string | null;
  position_id?: string | null;
  address?: string | null;
  payload: Record<string, unknown>;
}

export interface AdminWithdrawReviewItem {
  withdraw_id: string;
  user_id: number;
  user_address: string;
  chain_id: number;
  asset: string;
  amount: string;
  fee_amount: string;
  to_address: string;
  status: string;
  risk_flag?: string | null;
  tx_hash?: string | null;
  created_at: string;
}

export interface FundingItem {
  funding_id: string;
  symbol: string;
  direction: 'PAY' | 'RECEIVE';
  rate: string;
  amount: string;
  settled_at: string;
  batch_id: string;
}

export interface TransferItem {
  transfer_id: string;
  asset: string;
  amount: string;
  from_account: string;
  to_account: string;
  status: string;
  created_at: string;
}

export interface RiskSnapshot {
  account_status: string;
  risk_state: 'SAFE' | 'WATCH' | 'NO_NEW_RISK' | 'LIQUIDATING';
  mark_price_stale: boolean;
  can_open_risk: boolean;
  notes: string[];
}

export interface AuthenticatedSession {
  accessToken: string;
  refreshToken: string;
  expiresAt?: string;
  user: User;
}
