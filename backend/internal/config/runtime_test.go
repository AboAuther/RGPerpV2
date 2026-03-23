package config

import (
	"path/filepath"
	"testing"
)

func TestLoadRuntimeConfigSnapshot_Success(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.yaml")
	writeTestFile(t, path, `
version: 1
global:
  system_mode: review
  read_only: false
  reduce_only: false
  trace_header_required: true
auth:
  nonce_ttl_sec: 300
  access_ttl_sec: 3600
  refresh_ttl_sec: 2592000
  max_failed_login_per_ip_per_hour: 60
market:
  poll_interval_ms: 2000
  resting_execution_batch: 100
  max_source_age_sec: 10
  max_deviation_bps: "50"
  min_healthy_sources: 1
  mark_price_clamp_bps: "20"
  taker_fee_rate: "0.0006"
  maker_fee_rate: "0.0002"
  default_max_slippage_bps: 100
  source_weights:
    binance: "0.4"
    hyperliquid: "0.4"
    coinbase: "0.2"
  source_health_enabled:
    binance: true
    hyperliquid: true
    coinbase: true
wallet:
  deposit_confirmations:
    ethereum: 12
    arbitrum: 20
    base: 20
  withdraw_fee_usdc: "1"
  withdraw_circuit_mode: "NORMAL"
  withdraw_manual_review_threshold: "10000"
  withdraw_daily_limit_per_user: "50000"
  hot_wallet_min_balance: "10000"
  hot_wallet_max_balance: "200000"
risk:
  global_buffer_ratio: "0.002"
  mark_price_stale_sec: 3
  force_reduce_only_on_stale_price: true
  liquidation_penalty_rate: "0.01"
  liquidation_extra_slippage_bps: 20
  max_open_orders_per_user_per_symbol: 20
  net_exposure_hard_limit: "250000"
  max_exposure_slippage_bps: 40
funding:
  interval_sec: 3600
  source_poll_interval_sec: 60
  cap_rate_per_hour: "0.0075"
  min_valid_source_count: 1
  default_model_crypto: "EXTERNAL_AVG"
hedge:
  enabled: true
  soft_threshold_ratio: "0.2"
  hard_threshold_ratio: "0.4"
  max_retry: 5
  degrade_to_reduce_only_on_failure: true
review:
  faucet:
    enabled: true
    amount_per_request: "10000"
    max_requests_per_day: 10
  mock_market_data:
    enabled: true
`)

	cfg, err := LoadRuntimeConfigSnapshot(path)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if cfg.Auth.NonceTTLSec != 300 {
		t.Fatalf("unexpected nonce ttl: %d", cfg.Auth.NonceTTLSec)
	}
	if cfg.Market.PollIntervalMS != 2000 {
		t.Fatalf("unexpected market poll interval: %d", cfg.Market.PollIntervalMS)
	}
	if cfg.Market.RestingExecutionBatch != 100 {
		t.Fatalf("unexpected market execution batch: %d", cfg.Market.RestingExecutionBatch)
	}
	if cfg.Funding.IntervalSec != 3600 {
		t.Fatalf("unexpected funding interval: %d", cfg.Funding.IntervalSec)
	}
	if cfg.Funding.SourcePollIntervalSec != 60 {
		t.Fatalf("unexpected funding poll interval: %d", cfg.Funding.SourcePollIntervalSec)
	}
	if cfg.Risk.GlobalBufferRatio != "0.002" {
		t.Fatalf("unexpected risk buffer ratio: %s", cfg.Risk.GlobalBufferRatio)
	}
	if cfg.Risk.NetExposureHardLimit != "250000" {
		t.Fatalf("unexpected exposure limit: %s", cfg.Risk.NetExposureHardLimit)
	}
	if !cfg.Review.Faucet.Enabled {
		t.Fatal("expected faucet enabled")
	}
}

func TestLoadRuntimeConfigSnapshot_Invalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	writeTestFile(t, path, `
version: 0
global:
  system_mode: ""
`)

	_, err := LoadRuntimeConfigSnapshot(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
}
