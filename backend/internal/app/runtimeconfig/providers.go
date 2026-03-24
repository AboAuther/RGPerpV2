package runtimeconfig

import (
	"strings"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/config"
	fundingdomain "github.com/xiaobao/rgperp/backend/internal/domain/funding"
	liquidationdomain "github.com/xiaobao/rgperp/backend/internal/domain/liquidation"
	orderdomain "github.com/xiaobao/rgperp/backend/internal/domain/order"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
)

type ServiceRuntimeProvider struct {
	store *config.RuntimeConfigStore
}

func NewServiceRuntimeProvider(store *config.RuntimeConfigStore) *ServiceRuntimeProvider {
	return &ServiceRuntimeProvider{store: store}
}

func (p *ServiceRuntimeProvider) CurrentOrderRuntimeConfig(symbol string) orderdomain.RuntimeConfig {
	if p == nil || p.store == nil {
		return orderdomain.RuntimeConfig{}
	}
	current := p.store.Current()
	pair := pairOverride(current, symbol)
	return orderdomain.RuntimeConfig{
		GlobalReadOnly:               current.Global.ReadOnly,
		GlobalReduceOnly:             current.Global.ReduceOnly,
		MaxMarketDataAge:             time.Duration(current.Market.MaxSourceAgeSec) * time.Second,
		NetExposureHardLimit:         current.Risk.NetExposureHardLimit,
		MaxExposureSlippageBps:       current.Risk.MaxExposureSlippageBps,
		TakerFeeRate:                 pairString(pair.TakerFeeRate, current.Market.TakerFeeRate),
		MakerFeeRate:                 pairString(pair.MakerFeeRate, current.Market.MakerFeeRate),
		DefaultMaxSlippageBps:        pairInt(pair.DefaultMaxSlippageBps, current.Market.DefaultMaxSlippageBps),
		MaxLeverage:                  pairString(pair.MaxLeverage, ""),
		SessionPolicy:                pairString(pair.SessionPolicy, ""),
		LiquidationPenaltyRate:       pairString(pair.LiquidationPenaltyRate, current.Risk.LiquidationPenaltyRate),
		LiquidationExtraSlippageBps:  current.Risk.LiquidationExtraSlippageBps,
		MaintenanceMarginUpliftRatio: pairString(pair.MaintenanceMarginUpliftRatio, current.Risk.MaintenanceMarginUpliftRatio),
	}
}

func (p *ServiceRuntimeProvider) CurrentRiskRuntimeConfig() riskdomain.ServiceConfig {
	if p == nil || p.store == nil {
		return riskdomain.ServiceConfig{}
	}
	current := p.store.Current()
	return riskdomain.ServiceConfig{
		RiskBufferRatio:             current.Risk.GlobalBufferRatio,
		HedgeEnabled:                current.Hedge.Enabled,
		SoftThresholdRatio:          current.Hedge.SoftThresholdRatio,
		HardThresholdRatio:          current.Hedge.HardThresholdRatio,
		MarkPriceStaleSec:           current.Risk.MarkPriceStaleSec,
		ForceReduceOnlyOnStalePrice: current.Risk.ForceReduceOnlyOnStalePrice,
		TakerFeeRate:                current.Market.TakerFeeRate,
	}
}

func (p *ServiceRuntimeProvider) CurrentLiquidationRuntimeConfig(symbol string) liquidationdomain.ServiceConfig {
	if p == nil || p.store == nil {
		return liquidationdomain.ServiceConfig{}
	}
	current := p.store.Current()
	pair := pairOverride(current, symbol)
	return liquidationdomain.ServiceConfig{
		Asset:            "USDC",
		PenaltyRate:      pairString(pair.LiquidationPenaltyRate, current.Risk.LiquidationPenaltyRate),
		ExtraSlippageBps: current.Risk.LiquidationExtraSlippageBps,
	}
}

func (p *ServiceRuntimeProvider) CurrentWalletRuntimeConfig() walletdomain.RuntimeConfig {
	if p == nil || p.store == nil {
		return walletdomain.RuntimeConfig{}
	}
	current := p.store.Current()
	return walletdomain.RuntimeConfig{
		GlobalReadOnly: current.Global.ReadOnly,
	}
}

func (p *ServiceRuntimeProvider) CurrentFundingRuntimeConfig(symbol string) fundingdomain.ServiceConfig {
	if p == nil || p.store == nil {
		return fundingdomain.ServiceConfig{}
	}
	current := p.store.Current()
	pair := pairOverride(current, symbol)
	return fundingdomain.ServiceConfig{
		SettlementIntervalSec: pairInt(pair.FundingIntervalSec, current.Funding.IntervalSec),
		CapRatePerHour:        current.Funding.CapRatePerHour,
		MinValidSourceCount:   current.Funding.MinValidSourceCount,
		DefaultCryptoModel:    current.Funding.DefaultModelCrypto,
	}
}

func pairOverride(current config.RuntimeConfigSnapshot, symbol string) config.PairRuntimeConfig {
	key := strings.ToUpper(strings.TrimSpace(symbol))
	if key == "" || current.Pairs == nil {
		return config.PairRuntimeConfig{}
	}
	return current.Pairs[key]
}

func pairString(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return strings.TrimSpace(*value)
}

func pairInt(value *int, fallback int) int {
	if value == nil || *value <= 0 {
		return fallback
	}
	return *value
}
