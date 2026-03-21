SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  evm_address VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_users_evm_address (evm_address)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS login_nonces (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  evm_address VARCHAR(64) NOT NULL,
  nonce VARCHAR(128) NOT NULL,
  chain_id BIGINT NOT NULL,
  domain VARCHAR(255) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  used_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_login_nonces_nonce (nonce),
  KEY idx_login_nonces_address_created (evm_address, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sessions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  access_jti VARCHAR(128) NOT NULL,
  refresh_jti VARCHAR(128) NOT NULL,
  device_fingerprint VARCHAR(255) NOT NULL,
  ip VARCHAR(64) NOT NULL,
  user_agent VARCHAR(512) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  revoked_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_sessions_access_jti (access_jti),
  UNIQUE KEY uk_sessions_refresh_jti (refresh_jti),
  KEY idx_sessions_user_id_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS admin_users (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  username VARCHAR(128) NOT NULL,
  role VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_admin_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS accounts (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NULL,
  account_code VARCHAR(64) NOT NULL,
  account_type VARCHAR(64) NOT NULL,
  asset VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_accounts_user_code_asset (user_id, account_code, asset),
  KEY idx_accounts_account_code (account_code),
  KEY idx_accounts_asset (asset)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ledger_tx (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  ledger_tx_id VARCHAR(64) NOT NULL,
  event_id VARCHAR(64) NOT NULL,
  biz_type VARCHAR(64) NOT NULL,
  biz_ref_id VARCHAR(64) NOT NULL,
  asset VARCHAR(32) NOT NULL,
  idempotency_key VARCHAR(128) NOT NULL,
  operator_type VARCHAR(32) NOT NULL,
  operator_id VARCHAR(64) NOT NULL,
  trace_id VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  metadata_json JSON NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_ledger_tx_ledger_tx_id (ledger_tx_id),
  UNIQUE KEY uk_ledger_tx_event_id (event_id),
  UNIQUE KEY uk_ledger_tx_idempotency_key (idempotency_key),
  KEY idx_ledger_tx_biz_type_ref (biz_type, biz_ref_id),
  KEY idx_ledger_tx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ledger_entries (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  ledger_tx_id VARCHAR(64) NOT NULL,
  account_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NULL,
  asset VARCHAR(32) NOT NULL,
  amount DECIMAL(38,18) NOT NULL,
  entry_type VARCHAR(64) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_ledger_entries_ledger_tx_id (ledger_tx_id),
  KEY idx_ledger_entries_account_created (account_id, created_at),
  KEY idx_ledger_entries_user_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS account_balance_snapshots (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  account_id BIGINT UNSIGNED NOT NULL,
  asset VARCHAR(32) NOT NULL,
  balance DECIMAL(38,18) NOT NULL,
  version BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_abs_account_asset (account_id, asset)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS outbox_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  event_id VARCHAR(64) NOT NULL,
  aggregate_type VARCHAR(64) NOT NULL,
  aggregate_id VARCHAR(64) NOT NULL,
  event_type VARCHAR(128) NOT NULL,
  payload_json JSON NOT NULL,
  status VARCHAR(32) NOT NULL,
  published_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_outbox_events_event_id (event_id),
  KEY idx_outbox_status_created (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS message_consumptions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  consumer_name VARCHAR(128) NOT NULL,
  event_id VARCHAR(64) NOT NULL,
  consumed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_message_consumptions_consumer_event (consumer_name, event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS vaults (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  chain_id BIGINT NOT NULL,
  contract_address VARCHAR(64) NOT NULL,
  asset VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_vaults_chain_contract (chain_id, contract_address)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS deposit_addresses (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  chain_id BIGINT NOT NULL,
  address VARCHAR(64) NOT NULL,
  asset VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_deposit_addresses_user_chain_asset (user_id, chain_id, asset),
  UNIQUE KEY uk_deposit_addresses_chain_address (chain_id, address)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS deposit_chain_txs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  deposit_id VARCHAR(64) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  chain_id BIGINT NOT NULL,
  tx_hash VARCHAR(128) NOT NULL,
  log_index BIGINT NOT NULL,
  from_address VARCHAR(64) NOT NULL,
  to_address VARCHAR(64) NOT NULL,
  token_address VARCHAR(64) NOT NULL,
  amount DECIMAL(38,18) NOT NULL,
  block_number BIGINT NOT NULL,
  confirmations INT NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL,
  credited_ledger_tx_id VARCHAR(64) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_deposit_chain_txs_chain_tx_log (chain_id, tx_hash, log_index),
  UNIQUE KEY uk_deposit_chain_txs_deposit_id (deposit_id),
  KEY idx_deposit_chain_txs_status (status),
  KEY idx_deposit_chain_txs_user_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS withdraw_requests (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  withdraw_id VARCHAR(64) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  chain_id BIGINT NOT NULL,
  asset VARCHAR(32) NOT NULL,
  amount DECIMAL(38,18) NOT NULL,
  fee_amount DECIMAL(38,18) NOT NULL,
  to_address VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  risk_flag VARCHAR(64) NULL,
  hold_ledger_tx_id VARCHAR(64) NOT NULL,
  broadcast_tx_hash VARCHAR(128) NULL,
  broadcast_nonce BIGINT NULL,
  completed_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_withdraw_requests_withdraw_id (withdraw_id),
  KEY idx_withdraw_requests_user_created (user_id, created_at),
  KEY idx_withdraw_requests_status_created (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS chain_cursors (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  chain_id BIGINT NOT NULL,
  cursor_type VARCHAR(64) NOT NULL,
  cursor_value VARCHAR(128) NOT NULL,
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_chain_cursors_chain_type (chain_id, cursor_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS symbols (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  symbol VARCHAR(64) NOT NULL,
  asset_class VARCHAR(32) NOT NULL,
  base_asset VARCHAR(32) NOT NULL,
  quote_asset VARCHAR(32) NOT NULL,
  contract_multiplier DECIMAL(38,18) NOT NULL,
  tick_size DECIMAL(38,18) NOT NULL,
  step_size DECIMAL(38,18) NOT NULL,
  min_notional DECIMAL(38,18) NOT NULL,
  status VARCHAR(32) NOT NULL,
  session_policy VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_symbols_symbol (symbol)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS symbol_mappings (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  symbol_id BIGINT UNSIGNED NOT NULL,
  source_name VARCHAR(64) NOT NULL,
  source_symbol VARCHAR(64) NOT NULL,
  price_scale DECIMAL(38,18) NOT NULL,
  qty_scale DECIMAL(38,18) NOT NULL,
  status VARCHAR(32) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_symbol_mappings_symbol_source (symbol_id, source_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS risk_tiers (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  symbol_id BIGINT UNSIGNED NOT NULL,
  tier_level INT NOT NULL,
  max_notional DECIMAL(38,18) NOT NULL,
  max_leverage DECIMAL(38,18) NOT NULL,
  imr DECIMAL(38,18) NOT NULL,
  mmr DECIMAL(38,18) NOT NULL,
  liquidation_fee_rate DECIMAL(38,18) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_risk_tiers_symbol_level (symbol_id, tier_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS market_price_snapshots (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  symbol_id BIGINT UNSIGNED NOT NULL,
  source_name VARCHAR(64) NOT NULL,
  bid DECIMAL(38,18) NOT NULL,
  ask DECIMAL(38,18) NOT NULL,
  last DECIMAL(38,18) NOT NULL,
  mid DECIMAL(38,18) NOT NULL,
  source_ts DATETIME(3) NOT NULL,
  received_ts DATETIME(3) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_market_price_snapshots_symbol_created (symbol_id, created_at),
  KEY idx_market_price_snapshots_source_ts (source_name, source_ts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mark_price_snapshots (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  symbol_id BIGINT UNSIGNED NOT NULL,
  index_price DECIMAL(38,18) NOT NULL,
  mark_price DECIMAL(38,18) NOT NULL,
  calc_version BIGINT NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_mark_price_snapshots_symbol_created (symbol_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  order_id VARCHAR(64) NOT NULL,
  client_order_id VARCHAR(128) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  symbol_id BIGINT UNSIGNED NOT NULL,
  side VARCHAR(16) NOT NULL,
  position_effect VARCHAR(16) NOT NULL,
  type VARCHAR(32) NOT NULL,
  time_in_force VARCHAR(16) NOT NULL,
  price DECIMAL(38,18) NULL,
  trigger_price DECIMAL(38,18) NULL,
  qty DECIMAL(38,18) NOT NULL,
  filled_qty DECIMAL(38,18) NOT NULL DEFAULT 0,
  avg_fill_price DECIMAL(38,18) NOT NULL DEFAULT 0,
  reduce_only TINYINT(1) NOT NULL DEFAULT 0,
  max_slippage_bps INT NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL,
  reject_reason VARCHAR(255) NULL,
  frozen_margin DECIMAL(38,18) NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_orders_order_id (order_id),
  UNIQUE KEY uk_orders_user_client_order (user_id, client_order_id),
  KEY idx_orders_user_status_created (user_id, status, created_at),
  KEY idx_orders_symbol_status_created (symbol_id, status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS fills (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  fill_id VARCHAR(64) NOT NULL,
  order_id VARCHAR(64) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  symbol_id BIGINT UNSIGNED NOT NULL,
  side VARCHAR(16) NOT NULL,
  qty DECIMAL(38,18) NOT NULL,
  price DECIMAL(38,18) NOT NULL,
  fee_amount DECIMAL(38,18) NOT NULL,
  execution_price_snapshot_id BIGINT UNSIGNED NULL,
  ledger_tx_id VARCHAR(64) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_fills_fill_id (fill_id),
  KEY idx_fills_order_id (order_id),
  KEY idx_fills_user_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS positions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  position_id VARCHAR(64) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  symbol_id BIGINT UNSIGNED NOT NULL,
  side VARCHAR(16) NOT NULL,
  qty DECIMAL(38,18) NOT NULL DEFAULT 0,
  avg_entry_price DECIMAL(38,18) NOT NULL DEFAULT 0,
  mark_price DECIMAL(38,18) NOT NULL DEFAULT 0,
  notional DECIMAL(38,18) NOT NULL DEFAULT 0,
  initial_margin DECIMAL(38,18) NOT NULL DEFAULT 0,
  maintenance_margin DECIMAL(38,18) NOT NULL DEFAULT 0,
  realized_pnl DECIMAL(38,18) NOT NULL DEFAULT 0,
  unrealized_pnl DECIMAL(38,18) NOT NULL DEFAULT 0,
  funding_accrual DECIMAL(38,18) NOT NULL DEFAULT 0,
  liquidation_price DECIMAL(38,18) NOT NULL DEFAULT 0,
  bankruptcy_price DECIMAL(38,18) NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL,
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_positions_position_id (position_id),
  UNIQUE KEY uk_positions_user_symbol_side (user_id, symbol_id, side)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS risk_snapshots (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id BIGINT UNSIGNED NOT NULL,
  equity DECIMAL(38,18) NOT NULL,
  available_balance DECIMAL(38,18) NOT NULL,
  maintenance_margin DECIMAL(38,18) NOT NULL,
  margin_ratio DECIMAL(38,18) NOT NULL,
  risk_level VARCHAR(32) NOT NULL,
  triggered_by VARCHAR(64) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  KEY idx_risk_snapshots_user_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS liquidations (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  liquidation_id VARCHAR(64) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  symbol_id BIGINT UNSIGNED NULL,
  mode VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  trigger_risk_snapshot_id BIGINT UNSIGNED NOT NULL,
  penalty_amount DECIMAL(38,18) NOT NULL DEFAULT 0,
  insurance_fund_used DECIMAL(38,18) NOT NULL DEFAULT 0,
  bankrupt_amount DECIMAL(38,18) NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_liquidations_liquidation_id (liquidation_id),
  KEY idx_liquidations_user_status_created (user_id, status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS liquidation_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  liquidation_id VARCHAR(64) NOT NULL,
  position_id VARCHAR(64) NOT NULL,
  liquidated_qty DECIMAL(38,18) NOT NULL,
  execution_price DECIMAL(38,18) NOT NULL,
  ledger_tx_id VARCHAR(64) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_liquidation_items_liq_pos (liquidation_id, position_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS funding_batches (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  funding_batch_id VARCHAR(64) NOT NULL,
  symbol_id BIGINT UNSIGNED NOT NULL,
  time_window_start DATETIME(3) NOT NULL,
  time_window_end DATETIME(3) NOT NULL,
  normalized_rate DECIMAL(38,18) NOT NULL,
  settlement_price DECIMAL(38,18) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_funding_batches_batch_id (funding_batch_id),
  UNIQUE KEY uk_funding_batches_symbol_window (symbol_id, time_window_start, time_window_end)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS funding_batch_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  funding_batch_id VARCHAR(64) NOT NULL,
  position_id VARCHAR(64) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  funding_fee DECIMAL(38,18) NOT NULL,
  ledger_tx_id VARCHAR(64) NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_funding_batch_items_batch_position (funding_batch_id, position_id),
  KEY idx_funding_batch_items_user_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hedge_intents (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  hedge_intent_id VARCHAR(64) NOT NULL,
  symbol_id BIGINT UNSIGNED NOT NULL,
  side VARCHAR(16) NOT NULL,
  target_qty DECIMAL(38,18) NOT NULL,
  current_net_exposure DECIMAL(38,18) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_hedge_intents_hedge_intent_id (hedge_intent_id),
  KEY idx_hedge_intents_symbol_status_created (symbol_id, status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hedge_orders (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  hedge_order_id VARCHAR(64) NOT NULL,
  hedge_intent_id VARCHAR(64) NOT NULL,
  venue VARCHAR(32) NOT NULL,
  venue_order_id VARCHAR(128) NULL,
  symbol VARCHAR(64) NOT NULL,
  side VARCHAR(16) NOT NULL,
  qty DECIMAL(38,18) NOT NULL,
  price DECIMAL(38,18) NULL,
  status VARCHAR(32) NOT NULL,
  error_code VARCHAR(64) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_hedge_orders_hedge_order_id (hedge_order_id),
  KEY idx_hedge_orders_intent_created (hedge_intent_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hedge_fills (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  hedge_fill_id VARCHAR(64) NOT NULL,
  hedge_order_id VARCHAR(64) NOT NULL,
  venue_fill_id VARCHAR(128) NOT NULL,
  qty DECIMAL(38,18) NOT NULL,
  price DECIMAL(38,18) NOT NULL,
  fee DECIMAL(38,18) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_hedge_fills_hedge_fill_id (hedge_fill_id),
  UNIQUE KEY uk_hedge_fills_venue_fill_id (venue_fill_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hedge_positions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  symbol VARCHAR(64) NOT NULL,
  side VARCHAR(16) NOT NULL,
  qty DECIMAL(38,18) NOT NULL,
  avg_entry_price DECIMAL(38,18) NOT NULL,
  realized_pnl DECIMAL(38,18) NOT NULL DEFAULT 0,
  unrealized_pnl DECIMAL(38,18) NOT NULL DEFAULT 0,
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_hedge_positions_symbol_side (symbol, side)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  audit_id VARCHAR(64) NOT NULL,
  actor_type VARCHAR(32) NOT NULL,
  actor_id VARCHAR(64) NOT NULL,
  action VARCHAR(128) NOT NULL,
  resource_type VARCHAR(64) NOT NULL,
  resource_id VARCHAR(64) NOT NULL,
  before_json JSON NULL,
  after_json JSON NULL,
  trace_id VARCHAR(64) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_audit_logs_audit_id (audit_id),
  KEY idx_audit_logs_resource_created (resource_type, resource_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS config_items (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  config_key VARCHAR(128) NOT NULL,
  scope_type VARCHAR(64) NOT NULL,
  scope_value VARCHAR(128) NOT NULL,
  version BIGINT NOT NULL,
  value_json JSON NOT NULL,
  effective_at DATETIME(3) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_by VARCHAR(64) NOT NULL,
  approved_by VARCHAR(64) NULL,
  reason VARCHAR(255) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_config_items_key_scope_version (config_key, scope_type, scope_value, version),
  KEY idx_config_items_key_scope_status (config_key, scope_type, scope_value, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS explorer_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  internal_seq BIGINT NOT NULL,
  event_id VARCHAR(64) NOT NULL,
  event_type VARCHAR(128) NOT NULL,
  user_id BIGINT UNSIGNED NULL,
  address VARCHAR(64) NULL,
  symbol VARCHAR(64) NULL,
  order_id VARCHAR(64) NULL,
  fill_id VARCHAR(64) NULL,
  position_id VARCHAR(64) NULL,
  ledger_tx_id VARCHAR(64) NULL,
  chain_tx_hash VARCHAR(128) NULL,
  block_number BIGINT NULL,
  payload_json JSON NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_explorer_events_internal_seq (internal_seq),
  UNIQUE KEY uk_explorer_events_event_id (event_id),
  KEY idx_explorer_events_address_created (address, created_at),
  KEY idx_explorer_events_order_id (order_id),
  KEY idx_explorer_events_chain_tx_hash (chain_tx_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

SET FOREIGN_KEY_CHECKS = 1;
