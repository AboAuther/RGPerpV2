package config

type EnabledChain struct {
	Key            string
	DisplayName    string
	ChainID        int64
	Asset          string
	LocalTestnet   bool
	Confirmations  int
	RPCURL         string
	USDCAddress    string
	VaultAddress   string
	FactoryAddress string
}

func EnabledChains(cfg StaticConfig) []EnabledChain {
	out := make([]EnabledChain, 0, 3)
	appendIfEnabled := func(key string, chainCfg ChainConfig) {
		if !chainCfg.Enabled {
			return
		}
		out = append(out, EnabledChain{
			Key:            key,
			DisplayName:    chainCfg.DisplayName,
			ChainID:        chainCfg.ChainID,
			Asset:          "USDC",
			LocalTestnet:   chainCfg.LocalTestnet,
			Confirmations:  chainCfg.Confirmations,
			RPCURL:         chainCfg.RPCURL,
			USDCAddress:    chainCfg.USDCAddress,
			VaultAddress:   chainCfg.VaultAddress,
			FactoryAddress: chainCfg.FactoryAddress,
		})
	}

	appendIfEnabled("ethereum", cfg.Chains.Ethereum)
	appendIfEnabled("arbitrum", cfg.Chains.Arbitrum)
	appendIfEnabled("base", cfg.Chains.Base)
	return out
}

func ChainDisplayName(chain EnabledChain) string {
	if chain.DisplayName != "" {
		return chain.DisplayName
	}
	return chain.Key
}

func EnabledChainConfirmations(cfg StaticConfig) map[int64]int {
	out := make(map[int64]int)
	for _, chain := range EnabledChains(cfg) {
		out[chain.ChainID] = chain.Confirmations
	}
	return out
}
