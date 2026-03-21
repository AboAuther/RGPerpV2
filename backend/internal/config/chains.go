package config

const (
	ChainIDEthereum  int64 = 1
	ChainIDArbitrum  int64 = 42161
	ChainIDBase      int64 = 8453
	ChainIDLocalMock int64 = 31337
)

type EnabledChain struct {
	ChainID        int64
	Key            string
	Asset          string
	Confirmations  int
	RPCURL         string
	USDCAddress    string
	VaultAddress   string
	FactoryAddress string
}

func EffectiveBaseChainID(appEnv string) int64 {
	if appEnv == "review" || appEnv == "dev" {
		return ChainIDLocalMock
	}
	return ChainIDBase
}

func EnabledChains(cfg StaticConfig) []EnabledChain {
	out := make([]EnabledChain, 0, 3)
	appendIfConfigured := func(chainID int64, key string, chainCfg ChainConfig) {
		if chainCfg.RPCURL == "" {
			return
		}
		out = append(out, EnabledChain{
			ChainID:        chainID,
			Key:            key,
			Asset:          "USDC",
			Confirmations:  chainCfg.Confirmations,
			RPCURL:         chainCfg.RPCURL,
			USDCAddress:    chainCfg.USDCAddress,
			VaultAddress:   chainCfg.VaultAddress,
			FactoryAddress: chainCfg.FactoryAddress,
		})
	}

	appendIfConfigured(ChainIDEthereum, "ethereum", cfg.Chains.Ethereum)
	appendIfConfigured(ChainIDArbitrum, "arbitrum", cfg.Chains.Arbitrum)
	appendIfConfigured(EffectiveBaseChainID(cfg.App.Env), "local", cfg.Chains.Base)
	return out
}

func EnabledChainConfirmations(cfg StaticConfig) map[int64]int {
	out := make(map[int64]int)
	for _, chain := range EnabledChains(cfg) {
		out[chain.ChainID] = chain.Confirmations
	}
	return out
}
