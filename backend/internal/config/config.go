package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type LoadOptions struct {
	RootDir string
	Getenv  func(string) string
}

// StaticConfig contains startup-time configuration shared by backend processes.
type StaticConfig struct {
	App      AppConfig
	MySQL    MySQLConfig
	Redis    RedisConfig
	RabbitMQ RabbitMQConfig
	Auth     AuthConfig
	Admin    AdminConfig
	Chains   ChainConfigs
	Hedge    HedgeConfig
	Review   ReviewConfig
}

type AppConfig struct {
	Name              string
	Env               string
	Port              int
	LogLevel          string
	TimeZone          string
	RuntimeConfigPath string
}

type MySQLConfig struct {
	DSN string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type RabbitMQConfig struct {
	URL string
}

type AuthConfig struct {
	Domain        string
	AccessSecret  string
	RefreshSecret string
}

type AdminConfig struct {
	Wallets []string
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

var knownEnvKeys = []string{
	"APP_NAME",
	"APP_ENV",
	"APP_PORT",
	"LOG_LEVEL",
	"TZ",
	"RUNTIME_CONFIG_PATH",
	"MYSQL_DSN",
	"REDIS_ADDR",
	"REDIS_PASSWORD",
	"REDIS_DB",
	"RABBITMQ_URL",
	"AUTH_DOMAIN",
	"JWT_ACCESS_SECRET",
	"JWT_REFRESH_SECRET",
	"ADMIN_WALLETS",
	"ETH_RPC_URL",
	"ETH_CONFIRMATIONS",
	"ETH_USDC_ADDRESS",
	"ETH_VAULT_ADDRESS",
	"ETH_FACTORY_ADDRESS",
	"ARB_RPC_URL",
	"ARB_CONFIRMATIONS",
	"ARB_USDC_ADDRESS",
	"ARB_VAULT_ADDRESS",
	"ARB_FACTORY_ADDRESS",
	"BASE_RPC_URL",
	"BASE_CONFIRMATIONS",
	"BASE_USDC_ADDRESS",
	"BASE_VAULT_ADDRESS",
	"BASE_FACTORY_ADDRESS",
	"HL_API_URL",
	"HL_API_KEY",
	"HL_API_SECRET",
	"HL_ACCOUNT_ADDRESS",
	"REVIEW_FAUCET_ENABLED",
	"REVIEW_MOCK_MARKET_DATA_ENABLED",
}

func LoadStaticConfig() (StaticConfig, error) {
	return LoadStaticConfigWithOptions(LoadOptions{})
}

func LoadStaticConfigWithOptions(opts LoadOptions) (StaticConfig, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	rootDir, err := resolveProjectRoot(opts.RootDir)
	if err != nil && opts.RootDir != "" {
		rootDir = opts.RootDir
	}

	values, err := loadMergedStaticEnv(rootDir, getenv)
	if err != nil {
		return StaticConfig{}, err
	}

	cfg := loadStaticConfigFromLookup(func(key string) string {
		return values[key]
	})
	cfg.App.RuntimeConfigPath = normalizeConfigPath(rootDir, cfg.App.RuntimeConfigPath)
	return cfg, cfg.Validate()
}

// LoadStaticConfigFromEnv keeps env-only loading for unit tests and minimal wiring.
func LoadStaticConfigFromEnv(getenv func(string) string) (StaticConfig, error) {
	cfg := loadStaticConfigFromLookup(getenv)
	return cfg, cfg.Validate()
}

func loadStaticConfigFromLookup(getenv func(string) string) StaticConfig {
	return StaticConfig{
		App: AppConfig{
			Name:              strings.TrimSpace(getenv("APP_NAME")),
			Env:               strings.TrimSpace(getenv("APP_ENV")),
			Port:              getInt(getenv, "APP_PORT", 8080),
			LogLevel:          strings.TrimSpace(getenv("LOG_LEVEL")),
			TimeZone:          strings.TrimSpace(getenv("TZ")),
			RuntimeConfigPath: strings.TrimSpace(getenv("RUNTIME_CONFIG_PATH")),
		},
		MySQL: MySQLConfig{
			DSN: strings.TrimSpace(getenv("MYSQL_DSN")),
		},
		Redis: RedisConfig{
			Addr:     strings.TrimSpace(getenv("REDIS_ADDR")),
			Password: getenv("REDIS_PASSWORD"),
			DB:       getInt(getenv, "REDIS_DB", 0),
		},
		RabbitMQ: RabbitMQConfig{
			URL: strings.TrimSpace(getenv("RABBITMQ_URL")),
		},
		Auth: AuthConfig{
			Domain:        strings.TrimSpace(getenv("AUTH_DOMAIN")),
			AccessSecret:  getenv("JWT_ACCESS_SECRET"),
			RefreshSecret: getenv("JWT_REFRESH_SECRET"),
		},
		Admin: AdminConfig{
			Wallets: splitCSV(getenv("ADMIN_WALLETS")),
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
	if c.App.LogLevel == "" {
		errs = append(errs, fmt.Errorf("%w: LOG_LEVEL is required", errorsx.ErrInvalidArgument))
	}
	if c.App.TimeZone == "" {
		errs = append(errs, fmt.Errorf("%w: TZ is required", errorsx.ErrInvalidArgument))
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
	if c.App.RuntimeConfigPath == "" {
		errs = append(errs, fmt.Errorf("%w: RUNTIME_CONFIG_PATH is required", errorsx.ErrInvalidArgument))
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

func loadMergedStaticEnv(rootDir string, getenv func(string) string) (map[string]string, error) {
	merged := staticDefaults()

	if rootDir != "" {
		commonPath := filepath.Join(rootDir, "deploy", "env", "common.env")
		commonValues, err := parseEnvFile(commonPath)
		if err != nil {
			return nil, err
		}
		for key, value := range commonValues {
			merged[key] = value
		}

		appEnv := firstNonEmpty(strings.TrimSpace(getenv("APP_ENV")), merged["APP_ENV"])
		if appEnv != "" {
			envPath := filepath.Join(rootDir, "deploy", "env", appEnv+".env")
			envValues, err := parseEnvFile(envPath)
			if err != nil {
				return nil, err
			}
			for key, value := range envValues {
				merged[key] = value
			}
		}
	}

	for _, key := range knownEnvKeys {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			merged[key] = value
		}
	}

	for _, key := range []string{"REDIS_PASSWORD", "JWT_ACCESS_SECRET", "JWT_REFRESH_SECRET", "HL_API_SECRET"} {
		if value := getenv(key); value != "" {
			merged[key] = value
		}
	}

	return merged, nil
}

func staticDefaults() map[string]string {
	return map[string]string{
		"APP_PORT":           "8080",
		"LOG_LEVEL":          "info",
		"TZ":                 "UTC",
		"REDIS_DB":           "0",
		"ETH_CONFIRMATIONS":  "12",
		"ARB_CONFIRMATIONS":  "20",
		"BASE_CONFIRMATIONS": "20",
	}
}

func resolveProjectRoot(startDir string) (string, error) {
	dir := strings.TrimSpace(startDir)
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = wd
	}

	for {
		if stat, err := os.Stat(filepath.Join(dir, "deploy", "env")); err == nil && stat.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func parseEnvFile(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read env file %s: %w", path, err)
	}

	values := make(map[string]string)
	lines := strings.Split(string(content), "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%w: malformed env line in %s", errorsx.ErrInvalidArgument, path)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("%w: empty env key in %s", errorsx.ErrInvalidArgument, path)
		}
		values[key] = trimEnvValue(value)
	}
	return values, nil
}

func trimEnvValue(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	return out
}

func normalizeConfigPath(rootDir string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) || rootDir == "" {
		return path
	}
	return filepath.Join(rootDir, path)
}

func getInt(getenv func(string) string, key string, fallback int) int {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
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
