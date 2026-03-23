package runtimeconfig

import (
	"time"

	"github.com/xiaobao/rgperp/backend/internal/config"
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

func (p *ServiceRuntimeProvider) CurrentOrderRuntimeConfig() orderdomain.RuntimeConfig {
	if p == nil || p.store == nil {
		return orderdomain.RuntimeConfig{}
	}
	current := p.store.Current()
	return orderdomain.RuntimeConfig{
		GlobalReadOnly:         current.Global.ReadOnly,
		GlobalReduceOnly:       current.Global.ReduceOnly,
		MaxMarketDataAge:       time.Duration(current.Market.MaxSourceAgeSec) * time.Second,
		NetExposureHardLimit:   current.Risk.NetExposureHardLimit,
		MaxExposureSlippageBps: current.Risk.MaxExposureSlippageBps,
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

func (p *ServiceRuntimeProvider) CurrentLiquidationRuntimeConfig() liquidationdomain.ServiceConfig {
	if p == nil || p.store == nil {
		return liquidationdomain.ServiceConfig{}
	}
	current := p.store.Current()
	return liquidationdomain.ServiceConfig{
		Asset:            "USDC",
		PenaltyRate:      current.Risk.LiquidationPenaltyRate,
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
