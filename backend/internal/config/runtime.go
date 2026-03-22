package config

import (
	"fmt"
	"os"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gopkg.in/yaml.v3"
)

type RuntimeConfigSnapshot struct {
	Version int                 `yaml:"version"`
	Global  GlobalRuntimeConfig `yaml:"global"`
	Auth    AuthRuntimeConfig   `yaml:"auth"`
	Market  MarketRuntimeConfig `yaml:"market"`
	Wallet  WalletRuntimeConfig `yaml:"wallet"`
	Hedge   HedgeRuntimeConfig  `yaml:"hedge"`
	Review  ReviewRuntimeConfig `yaml:"review"`
}

type GlobalRuntimeConfig struct {
	SystemMode          string `yaml:"system_mode"`
	ReadOnly            bool   `yaml:"read_only"`
	ReduceOnly          bool   `yaml:"reduce_only"`
	TraceHeaderRequired bool   `yaml:"trace_header_required"`
}

type AuthRuntimeConfig struct {
	NonceTTLSec                int `yaml:"nonce_ttl_sec"`
	AccessTTLSec               int `yaml:"access_ttl_sec"`
	RefreshTTLSec              int `yaml:"refresh_ttl_sec"`
	MaxFailedLoginPerIPPerHour int `yaml:"max_failed_login_per_ip_per_hour"`
}

type MarketRuntimeConfig struct {
	PollIntervalMS        int               `yaml:"poll_interval_ms"`
	MaxSourceAgeSec       int               `yaml:"max_source_age_sec"`
	MaxDeviationBps       string            `yaml:"max_deviation_bps"`
	MinHealthySources     int               `yaml:"min_healthy_sources"`
	MarkPriceClampBps     string            `yaml:"mark_price_clamp_bps"`
	TakerFeeRate          string            `yaml:"taker_fee_rate"`
	MakerFeeRate          string            `yaml:"maker_fee_rate"`
	DefaultMaxSlippageBps int               `yaml:"default_max_slippage_bps"`
	SourceWeights         map[string]string `yaml:"source_weights"`
	SourceHealthEnabled   map[string]bool   `yaml:"source_health_enabled"`
}

type WalletRuntimeConfig struct {
	DepositConfirmations          map[string]int `yaml:"deposit_confirmations"`
	WithdrawFeeUSDC               string         `yaml:"withdraw_fee_usdc"`
	WithdrawCircuitMode           string         `yaml:"withdraw_circuit_mode"`
	WithdrawManualReviewThreshold string         `yaml:"withdraw_manual_review_threshold"`
	WithdrawDailyLimitPerUser     string         `yaml:"withdraw_daily_limit_per_user"`
	HotWalletMinBalance           string         `yaml:"hot_wallet_min_balance"`
	HotWalletMaxBalance           string         `yaml:"hot_wallet_max_balance"`
}

type HedgeRuntimeConfig struct {
	Enabled                      bool   `yaml:"enabled"`
	SoftThresholdRatio           string `yaml:"soft_threshold_ratio"`
	HardThresholdRatio           string `yaml:"hard_threshold_ratio"`
	MaxRetry                     int    `yaml:"max_retry"`
	DegradeToReduceOnlyOnFailure bool   `yaml:"degrade_to_reduce_only_on_failure"`
}

type ReviewRuntimeConfig struct {
	Faucet         FaucetRuntimeConfig         `yaml:"faucet"`
	MockMarketData MockMarketDataRuntimeConfig `yaml:"mock_market_data"`
}

type FaucetRuntimeConfig struct {
	Enabled           bool   `yaml:"enabled"`
	AmountPerRequest  string `yaml:"amount_per_request"`
	MaxRequestsPerDay int    `yaml:"max_requests_per_day"`
}

type MockMarketDataRuntimeConfig struct {
	Enabled bool `yaml:"enabled"`
}

func LoadRuntimeConfigSnapshot(path string) (RuntimeConfigSnapshot, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return RuntimeConfigSnapshot{}, fmt.Errorf("read runtime config: %w", err)
	}

	var cfg RuntimeConfigSnapshot
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return RuntimeConfigSnapshot{}, fmt.Errorf("unmarshal runtime config: %w", err)
	}
	return cfg, cfg.Validate()
}

func (c RuntimeConfigSnapshot) Validate() error {
	var errs []error

	if c.Version <= 0 {
		errs = append(errs, fmt.Errorf("%w: runtime config version is required", errorsx.ErrInvalidArgument))
	}
	if c.Global.SystemMode == "" {
		errs = append(errs, fmt.Errorf("%w: global.system_mode is required", errorsx.ErrInvalidArgument))
	}
	if c.Auth.NonceTTLSec <= 0 {
		errs = append(errs, fmt.Errorf("%w: auth.nonce_ttl_sec must be positive", errorsx.ErrInvalidArgument))
	}
	if c.Auth.AccessTTLSec <= 0 {
		errs = append(errs, fmt.Errorf("%w: auth.access_ttl_sec must be positive", errorsx.ErrInvalidArgument))
	}
	if c.Auth.RefreshTTLSec <= 0 {
		errs = append(errs, fmt.Errorf("%w: auth.refresh_ttl_sec must be positive", errorsx.ErrInvalidArgument))
	}
	if c.Auth.MaxFailedLoginPerIPPerHour <= 0 {
		errs = append(errs, fmt.Errorf("%w: auth.max_failed_login_per_ip_per_hour must be positive", errorsx.ErrInvalidArgument))
	}
	if c.Market.PollIntervalMS <= 0 {
		errs = append(errs, fmt.Errorf("%w: market.poll_interval_ms must be positive", errorsx.ErrInvalidArgument))
	}
	if c.Market.MaxSourceAgeSec <= 0 {
		errs = append(errs, fmt.Errorf("%w: market.max_source_age_sec must be positive", errorsx.ErrInvalidArgument))
	}
	if c.Market.MaxDeviationBps == "" {
		errs = append(errs, fmt.Errorf("%w: market.max_deviation_bps is required", errorsx.ErrInvalidArgument))
	}
	if c.Market.MinHealthySources <= 0 {
		errs = append(errs, fmt.Errorf("%w: market.min_healthy_sources must be positive", errorsx.ErrInvalidArgument))
	}
	if c.Market.MarkPriceClampBps == "" {
		errs = append(errs, fmt.Errorf("%w: market.mark_price_clamp_bps is required", errorsx.ErrInvalidArgument))
	}
	if c.Market.TakerFeeRate == "" {
		errs = append(errs, fmt.Errorf("%w: market.taker_fee_rate is required", errorsx.ErrInvalidArgument))
	}
	if c.Market.MakerFeeRate == "" {
		errs = append(errs, fmt.Errorf("%w: market.maker_fee_rate is required", errorsx.ErrInvalidArgument))
	}
	if c.Market.DefaultMaxSlippageBps <= 0 {
		errs = append(errs, fmt.Errorf("%w: market.default_max_slippage_bps must be positive", errorsx.ErrInvalidArgument))
	}
	if len(c.Market.SourceWeights) == 0 {
		errs = append(errs, fmt.Errorf("%w: market.source_weights is required", errorsx.ErrInvalidArgument))
	}
	if len(c.Wallet.DepositConfirmations) == 0 {
		errs = append(errs, fmt.Errorf("%w: wallet.deposit_confirmations is required", errorsx.ErrInvalidArgument))
	}
	if c.Wallet.WithdrawFeeUSDC == "" {
		errs = append(errs, fmt.Errorf("%w: wallet.withdraw_fee_usdc is required", errorsx.ErrInvalidArgument))
	}
	if c.Wallet.WithdrawCircuitMode == "" {
		errs = append(errs, fmt.Errorf("%w: wallet.withdraw_circuit_mode is required", errorsx.ErrInvalidArgument))
	}
	if c.Wallet.WithdrawManualReviewThreshold == "" {
		errs = append(errs, fmt.Errorf("%w: wallet.withdraw_manual_review_threshold is required", errorsx.ErrInvalidArgument))
	}
	if c.Wallet.WithdrawDailyLimitPerUser == "" {
		errs = append(errs, fmt.Errorf("%w: wallet.withdraw_daily_limit_per_user is required", errorsx.ErrInvalidArgument))
	}
	if c.Wallet.HotWalletMinBalance == "" || c.Wallet.HotWalletMaxBalance == "" {
		errs = append(errs, fmt.Errorf("%w: wallet hot wallet balance thresholds are required", errorsx.ErrInvalidArgument))
	}
	if c.Hedge.SoftThresholdRatio == "" || c.Hedge.HardThresholdRatio == "" {
		errs = append(errs, fmt.Errorf("%w: hedge threshold ratios are required", errorsx.ErrInvalidArgument))
	}
	if c.Hedge.MaxRetry <= 0 {
		errs = append(errs, fmt.Errorf("%w: hedge.max_retry must be positive", errorsx.ErrInvalidArgument))
	}
	if c.Review.Faucet.AmountPerRequest == "" {
		errs = append(errs, fmt.Errorf("%w: review.faucet.amount_per_request is required", errorsx.ErrInvalidArgument))
	}
	if c.Review.Faucet.MaxRequestsPerDay <= 0 {
		errs = append(errs, fmt.Errorf("%w: review.faucet.max_requests_per_day must be positive", errorsx.ErrInvalidArgument))
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("runtime config validation failed: %v", errs)
}
