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

export interface TransferRequest {
  to_address: string;
  asset: string;
  amount: string;
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
  status: string;
  stale: boolean;
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
  created_at: string;
}

export interface OrderCreateRequest {
  client_order_id: string;
  symbol: string;
  side: 'BUY' | 'SELL';
  position_effect: 'OPEN' | 'REDUCE' | 'CLOSE';
  type: 'MARKET' | 'LIMIT' | 'STOP_MARKET' | 'TAKE_PROFIT_MARKET';
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

export interface LedgerAssetOverview {
  asset: string;
  user_wallet: string;
  user_order_margin: string;
  user_position_margin: string;
  user_withdraw_hold: string;
  user_margin: string;
  user_liability: string;
  system_pool: string;
  trading_fee_account: string;
  withdraw_fee_account: string;
  penalty_account: string;
  funding_pool: string;
  insurance_fund: string;
  rounding_diff_account: string;
  deposit_pending_confirm: string;
  withdraw_in_transit: string;
  sweep_in_transit: string;
  custody_hot: string;
  custody_warm: string;
  custody_cold: string;
  test_faucet_pool: string;
  platform_revenue: string;
  risk_buffer: string;
  in_flight: string;
  custody_mirror: string;
  net_balance: string;
}

export interface LedgerOverview {
  scope_asset: string;
  generated_at: string;
  notes: string[];
  assets: LedgerAssetOverview[];
}

export interface LedgerChainBalance {
  row_type: 'CHAIN' | 'TOTAL';
  chain_id: number;
  chain_key: string;
  chain_name: string;
  asset: string;
  vault_address: string;
  onchain_balance: string;
  custody_mirror: string;
  delta: string;
  status: 'PASS' | 'FAIL';
}

export interface LedgerAuditCheck {
  check_key: string;
  label: string;
  status: 'PASS' | 'FAIL';
  value: string;
  summary: string;
  sample_refs?: string[];
}

export interface LedgerAuditReport {
  audit_report_id: string;
  scope_asset: string;
  status: 'PASS' | 'FAIL';
  executed_by: string;
  started_at: string;
  finished_at: string;
  overview: LedgerAssetOverview[];
  chain_balances?: LedgerChainBalance[];
  checks: LedgerAuditCheck[];
}

export interface SymbolNetExposureItem {
  symbol: string;
  status: string;
  mark_price: string;
  long_qty: string;
  short_qty: string;
  net_qty: string;
  net_notional: string;
  hard_limit_notional: string;
  utilization_ratio: string;
  blocked_open_side?: 'BUY' | 'SELL' | null;
  buy_adjustment_bps: number;
  sell_adjustment_bps: number;
}

export interface RiskMonitorDashboard {
  generated_at: string;
  hard_limit_notional: string;
  max_dynamic_slippage_bps: number;
  items: SymbolNetExposureItem[];
}

export interface RuntimeConfigSnapshotView {
  system_mode: string;
  read_only: boolean;
  reduce_only: boolean;
  trace_header_required: boolean;
  risk_global_buffer_ratio: string;
  risk_mark_price_stale_sec: number;
  risk_force_reduce_only_on_stale_price: boolean;
  risk_liquidation_penalty_rate: string;
  risk_liquidation_extra_slippage_bps: number;
  risk_max_open_orders_per_user_per_symbol: number;
  risk_net_exposure_hard_limit: string;
  risk_max_exposure_slippage_bps: number;
  hedge_enabled: boolean;
  hedge_soft_threshold_ratio: string;
  hedge_hard_threshold_ratio: string;
}

export interface RuntimeConfigHistoryItem {
  config_key: string;
  scope_type: string;
  scope_value: string;
  version: number;
  value: unknown;
  status: string;
  created_by: string;
  approved_by?: string | null;
  reason: string;
  effective_at: string;
  created_at: string;
}

export interface RuntimeConfigView {
  snapshot: RuntimeConfigSnapshotView;
  generated_at: string;
  history: RuntimeConfigHistoryItem[];
}

export interface RuntimeConfigPatchRequest {
  reason: string;
  global?: {
    read_only?: boolean;
    reduce_only?: boolean;
    trace_header_required?: boolean;
  };
  risk?: {
    global_buffer_ratio?: string;
    mark_price_stale_sec?: number;
    force_reduce_only_on_stale_price?: boolean;
    liquidation_penalty_rate?: string;
    liquidation_extra_slippage_bps?: number;
    max_open_orders_per_user_per_symbol?: number;
    net_exposure_hard_limit?: string;
    max_exposure_slippage_bps?: number;
  };
  hedge?: {
    enabled?: boolean;
    soft_threshold_ratio?: string;
    hard_threshold_ratio?: string;
  };
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
  direction: 'IN' | 'OUT' | 'SELF' | 'UNKNOWN';
  counterparty_address: string;
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
