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
	appendIfConfigured(EffectiveBaseChainID(cfg.App.Env), baseChainKey(cfg.App.Env), cfg.Chains.Base)
	return out
}

func baseChainKey(appEnv string) string {
	if appEnv == "review" || appEnv == "dev" {
		return "local"
	}
	return "base"
}

func ChainDisplayName(chain EnabledChain) string {
	switch chain.Key {
	case "local":
		return "Local Anvil"
	case "ethereum":
		return "Ethereum"
	case "arbitrum":
		return "Arbitrum"
	case "base":
		return "Base"
	default:
		return chain.Key
	}
}

func EnabledChainConfirmations(cfg StaticConfig) map[int64]int {
	out := make(map[int64]int)
	for _, chain := range EnabledChains(cfg) {
		out[chain.ChainID] = chain.Confirmations
	}
	return out
}
