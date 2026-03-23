package config

type MarketSymbolSeed struct {
	Symbol             string
	AssetClass         string
	BaseAsset          string
	QuoteAsset         string
	ContractMultiplier string
	TickSize           string
	StepSize           string
	MinNotional        string
	Status             string
	SessionPolicy      string
	BinanceSymbol      string
	HyperliquidSymbol  string
	CoinbaseSymbol     string
}

func DefaultMarketSymbolSeeds() []MarketSymbolSeed {
	bases := []string{
		"AAVE", "ADA", "ARB", "AVAX", "BCH", "BIO", "BNB", "BOME", "BTC", "CRV",
		"DOGE", "ENA", "ETH", "ETHFI", "FIL", "HBAR", "IP", "KAITO", "LINK", "LTC",
		"NEAR", "NEO", "ORDI", "PENGU", "PNUT", "SOL", "SUI", "TIA", "TRUMP", "UNI",
		"WIF", "WLD", "WLFI", "XRP", "ZEC", "kPEPE", "kSHIB", "kBONK",
	}
	seeds := make([]MarketSymbolSeed, 0, len(bases))
	for _, base := range bases {
		binance := base + "USDC"
		hyperliquid := base
		switch base {
		case "kPEPE":
			binance = "1000PEPEUSDC"
		case "kSHIB":
			binance = "1000SHIBUSDC"
		case "kBONK":
			binance = "1000BONKUSDC"
		}
		tickSize, stepSize := defaultSymbolPrecision(base)
		seeds = append(seeds, MarketSymbolSeed{
			Symbol:             base + "-USDC",
			AssetClass:         "CRYPTO",
			BaseAsset:          base,
			QuoteAsset:         "USDC",
			ContractMultiplier: "1",
			TickSize:           tickSize,
			StepSize:           stepSize,
			MinNotional:        "10",
			Status:             "TRADING",
			SessionPolicy:      "ALWAYS_OPEN",
			BinanceSymbol:      binance,
			HyperliquidSymbol:  hyperliquid,
			CoinbaseSymbol:     defaultCoinbaseSymbol(base),
		})
	}
	return seeds
}

func defaultSymbolPrecision(base string) (tickSize string, stepSize string) {
	switch base {
	case "BTC":
		return "0.1", "0.001"
	case "ETH":
		return "0.01", "0.001"
	case "BNB", "SOL", "AVAX", "BCH", "LTC", "AAVE", "LINK":
		return "0.01", "0.001"
	default:
		return "0.0001", "0.001"
	}
}

func defaultCoinbaseSymbol(base string) string {
	switch base {
	case "BTC", "ETH", "SOL", "ADA", "AVAX", "BCH", "DOGE", "LINK", "LTC", "NEAR", "UNI", "XRP":
		return base + "-USD"
	default:
		return ""
	}
}
