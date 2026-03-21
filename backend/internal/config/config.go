package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

// StaticConfig is the startup-time configuration shared by backend processes.
type StaticConfig struct {
	App      AppConfig
	MySQL    MySQLConfig
	Redis    RedisConfig
	RabbitMQ RabbitMQConfig
	Auth     AuthConfig
	Chains   ChainConfigs
	Hedge    HedgeConfig
	Review   ReviewConfig
}

type AppConfig struct {
	Name string
	Env  string
	Port int
}

type MySQLConfig struct {
	DSN string
}

type RedisConfig struct {
	Addr     string
	Password string
}

type RabbitMQConfig struct {
	URL string
}

type AuthConfig struct {
	Domain        string
	AccessSecret  string
	RefreshSecret string
}

type ChainConfig struct {
	RPCURL         string
	Confirmations  int
	USDCAddress    string
	VaultAddress   string
	FactoryAddress string
}

type ChainConfigs struct {
	Ethereum ChainConfig
	Arbitrum ChainConfig
	Base     ChainConfig
}

type HedgeConfig struct {
	APIURL         string
	APIKey         string
	APISecret      string
	AccountAddress string
}

type ReviewConfig struct {
	FaucetEnabled         bool
	MockMarketDataEnabled bool
}

// LoadStaticConfigFromEnv loads static config from an injected env getter.
func LoadStaticConfigFromEnv(getenv func(string) string) (StaticConfig, error) {
	cfg := StaticConfig{
		App: AppConfig{
			Name: strings.TrimSpace(getenv("APP_NAME")),
			Env:  strings.TrimSpace(getenv("APP_ENV")),
			Port: getInt(getenv, "APP_PORT", 8080),
		},
		MySQL: MySQLConfig{
			DSN: strings.TrimSpace(getenv("MYSQL_DSN")),
		},
		Redis: RedisConfig{
			Addr:     strings.TrimSpace(getenv("REDIS_ADDR")),
			Password: getenv("REDIS_PASSWORD"),
		},
		RabbitMQ: RabbitMQConfig{
			URL: strings.TrimSpace(getenv("RABBITMQ_URL")),
		},
		Auth: AuthConfig{
			Domain:        strings.TrimSpace(getenv("AUTH_DOMAIN")),
			AccessSecret:  getenv("JWT_ACCESS_SECRET"),
			RefreshSecret: getenv("JWT_REFRESH_SECRET"),
		},
		Chains: ChainConfigs{
			Ethereum: ChainConfig{
				RPCURL:         strings.TrimSpace(getenv("ETH_RPC_URL")),
				Confirmations:  getInt(getenv, "ETH_CONFIRMATIONS", 12),
				USDCAddress:    strings.TrimSpace(getenv("ETH_USDC_ADDRESS")),
				VaultAddress:   strings.TrimSpace(getenv("ETH_VAULT_ADDRESS")),
				FactoryAddress: strings.TrimSpace(getenv("ETH_FACTORY_ADDRESS")),
			},
			Arbitrum: ChainConfig{
				RPCURL:         strings.TrimSpace(getenv("ARB_RPC_URL")),
				Confirmations:  getInt(getenv, "ARB_CONFIRMATIONS", 20),
				USDCAddress:    strings.TrimSpace(getenv("ARB_USDC_ADDRESS")),
				VaultAddress:   strings.TrimSpace(getenv("ARB_VAULT_ADDRESS")),
				FactoryAddress: strings.TrimSpace(getenv("ARB_FACTORY_ADDRESS")),
			},
			Base: ChainConfig{
				RPCURL:         strings.TrimSpace(getenv("BASE_RPC_URL")),
				Confirmations:  getInt(getenv, "BASE_CONFIRMATIONS", 20),
				USDCAddress:    strings.TrimSpace(getenv("BASE_USDC_ADDRESS")),
				VaultAddress:   strings.TrimSpace(getenv("BASE_VAULT_ADDRESS")),
				FactoryAddress: strings.TrimSpace(getenv("BASE_FACTORY_ADDRESS")),
			},
		},
		Hedge: HedgeConfig{
			APIURL:         strings.TrimSpace(getenv("HL_API_URL")),
			APIKey:         strings.TrimSpace(getenv("HL_API_KEY")),
			APISecret:      strings.TrimSpace(getenv("HL_API_SECRET")),
			AccountAddress: strings.TrimSpace(getenv("HL_ACCOUNT_ADDRESS")),
		},
		Review: ReviewConfig{
			FaucetEnabled:         getBool(getenv, "REVIEW_FAUCET_ENABLED", false),
			MockMarketDataEnabled: getBool(getenv, "REVIEW_MOCK_MARKET_DATA_ENABLED", false),
		},
	}
	return cfg, cfg.Validate()
}

// Validate enforces fail-closed startup rules for sensitive configuration.
func (c StaticConfig) Validate() error {
	var errs []error

	if c.App.Name == "" {
		errs = append(errs, fmt.Errorf("%w: APP_NAME is required", errorsx.ErrInvalidArgument))
	}
	if c.App.Env == "" {
		errs = append(errs, fmt.Errorf("%w: APP_ENV is required", errorsx.ErrInvalidArgument))
	}
	if c.MySQL.DSN == "" {
		errs = append(errs, fmt.Errorf("%w: MYSQL_DSN is required", errorsx.ErrInvalidArgument))
	}
	if c.RabbitMQ.URL == "" {
		errs = append(errs, fmt.Errorf("%w: RABBITMQ_URL is required", errorsx.ErrInvalidArgument))
	}
	if c.Auth.Domain == "" {
		errs = append(errs, fmt.Errorf("%w: AUTH_DOMAIN is required", errorsx.ErrInvalidArgument))
	}
	if c.Auth.AccessSecret == "" || c.Auth.RefreshSecret == "" {
		errs = append(errs, fmt.Errorf("%w: JWT secrets are required", errorsx.ErrInvalidArgument))
	}

	for _, chain := range []struct {
		name string
		cfg  ChainConfig
	}{
		{name: "ethereum", cfg: c.Chains.Ethereum},
		{name: "arbitrum", cfg: c.Chains.Arbitrum},
		{name: "base", cfg: c.Chains.Base},
	} {
		if chain.cfg.RPCURL == "" {
			continue
		}
		if chain.cfg.USDCAddress == "" || chain.cfg.VaultAddress == "" || chain.cfg.FactoryAddress == "" {
			errs = append(errs, fmt.Errorf("%w: %s chain config is incomplete", errorsx.ErrInvalidArgument, chain.name))
		}
	}

	if c.App.Env == "prod" {
		if c.Review.FaucetEnabled {
			errs = append(errs, fmt.Errorf("%w: faucet must be disabled in prod", errorsx.ErrForbidden))
		}
		if c.Review.MockMarketDataEnabled {
			errs = append(errs, fmt.Errorf("%w: mock market data must be disabled in prod", errorsx.ErrForbidden))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("config validation failed: %v", errs)
}

func getInt(getenv func(string) string, key string, fallback int) int {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func getBool(getenv func(string) string, key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
