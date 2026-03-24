package config

import (
	"encoding/json"
	"testing"
)

func TestApplyDynamicConfigValues_PairOverrides(t *testing.T) {
	base := RuntimeConfigSnapshot{
		Version: 1,
		Global:  GlobalRuntimeConfig{SystemMode: "dev"},
		Auth: AuthRuntimeConfig{
			NonceTTLSec:                300,
			AccessTTLSec:               3600,
			RefreshTTLSec:              7200,
			MaxFailedLoginPerIPPerHour: 60,
		},
		Market: MarketRuntimeConfig{
			PollIntervalMS:        1000,
			RestingExecutionBatch: 100,
			MaxSourceAgeSec:       5,
			MaxDeviationBps:       "50",
			MinHealthySources:     1,
			MarkPriceClampBps:     "20",
			TakerFeeRate:          "0.0006",
			MakerFeeRate:          "0.0002",
			DefaultMaxSlippageBps: 100,
			SourceWeights:         map[string]string{"binance": "1"},
			SourceHealthEnabled:   map[string]bool{"binance": true},
		},
		Wallet: WalletRuntimeConfig{
			DepositConfirmations:          map[string]int{"base": 1},
			WithdrawFeeUSDC:               "1",
			WithdrawCircuitMode:           "NORMAL",
			WithdrawManualReviewThreshold: "10000",
			WithdrawDailyLimitPerUser:     "50000",
			HotWalletMinBalance:           "0",
			HotWalletMaxBalance:           "100000",
		},
		Risk: RiskRuntimeConfig{
			GlobalBufferRatio:             "0.002",
			MarkPriceStaleSec:             3,
			ForceReduceOnlyOnStalePrice:   true,
			LiquidationPenaltyRate:        "0.01",
			MaintenanceMarginUpliftRatio:  "0",
			LiquidationExtraSlippageBps:   20,
			MaxOpenOrdersPerUserPerSymbol: 20,
			NetExposureHardLimit:          "250000",
			MaxExposureSlippageBps:        40,
		},
		Funding: FundingRuntimeConfig{
			IntervalSec:           3600,
			SourcePollIntervalSec: 60,
			CapRatePerHour:        "0.0075",
			MinValidSourceCount:   1,
			DefaultModelCrypto:    "EXTERNAL_AVG",
		},
		Hedge: HedgeRuntimeConfig{
			Enabled:            true,
			SoftThresholdRatio: "0.2",
			HardThresholdRatio: "0.4",
			MaxRetry:           5,
		},
		Review: ReviewRuntimeConfig{
			Faucet: FaucetRuntimeConfig{
				AmountPerRequest:  "1000",
				MaxRequestsPerDay: 10,
			},
		},
	}

	stringJSON := func(value string) json.RawMessage {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal string: %v", err)
		}
		return raw
	}
	intJSON := func(value int) json.RawMessage {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal int: %v", err)
		}
		return raw
	}

	cfg, err := ApplyDynamicConfigValues(base, []DynamicConfigValue{
		{Key: "market.taker_fee_rate", ScopeType: ConfigScopeTypeGlobal, ScopeValue: ConfigScopeValueGlobal, ValueJSON: stringJSON("0.0007")},
		{Key: "market.max_leverage", ScopeType: ConfigScopeTypePair, ScopeValue: "btc-usdc", ValueJSON: stringJSON("25")},
		{Key: "market.session_policy", ScopeType: ConfigScopeTypePair, ScopeValue: "BTC-USDC", ValueJSON: stringJSON("US_EQUITY_REGULAR")},
		{Key: "market.default_max_slippage_bps", ScopeType: ConfigScopeTypePair, ScopeValue: "BTC-USDC", ValueJSON: intJSON(60)},
		{Key: "risk.liquidation_penalty_rate", ScopeType: ConfigScopeTypePair, ScopeValue: "BTC-USDC", ValueJSON: stringJSON("0.015")},
		{Key: "risk.maintenance_margin_uplift_ratio", ScopeType: ConfigScopeTypePair, ScopeValue: "BTC-USDC", ValueJSON: stringJSON("0.10")},
		{Key: "funding.interval_sec", ScopeType: ConfigScopeTypePair, ScopeValue: "BTC-USDC", ValueJSON: intJSON(1800)},
	})
	if err != nil {
		t.Fatalf("ApplyDynamicConfigValues: %v", err)
	}

	if cfg.Market.TakerFeeRate != "0.0007" {
		t.Fatalf("expected global taker fee override, got %s", cfg.Market.TakerFeeRate)
	}
	pairCfg, ok := cfg.Pairs["BTC-USDC"]
	if !ok {
		t.Fatalf("expected BTC-USDC pair config")
	}
	if pairCfg.MaxLeverage == nil || *pairCfg.MaxLeverage != "25" {
		t.Fatalf("unexpected pair max leverage: %#v", pairCfg.MaxLeverage)
	}
	if pairCfg.SessionPolicy == nil || *pairCfg.SessionPolicy != "US_EQUITY_REGULAR" {
		t.Fatalf("unexpected pair session policy: %#v", pairCfg.SessionPolicy)
	}
	if pairCfg.DefaultMaxSlippageBps == nil || *pairCfg.DefaultMaxSlippageBps != 60 {
		t.Fatalf("unexpected pair slippage: %#v", pairCfg.DefaultMaxSlippageBps)
	}
	if pairCfg.LiquidationPenaltyRate == nil || *pairCfg.LiquidationPenaltyRate != "0.015" {
		t.Fatalf("unexpected pair liquidation penalty: %#v", pairCfg.LiquidationPenaltyRate)
	}
	if pairCfg.MaintenanceMarginUpliftRatio == nil || *pairCfg.MaintenanceMarginUpliftRatio != "0.10" {
		t.Fatalf("unexpected pair maintenance uplift: %#v", pairCfg.MaintenanceMarginUpliftRatio)
	}
	if pairCfg.FundingIntervalSec == nil || *pairCfg.FundingIntervalSec != 1800 {
		t.Fatalf("unexpected pair funding interval: %#v", pairCfg.FundingIntervalSec)
	}
}
