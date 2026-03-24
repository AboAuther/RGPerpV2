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
	App        AppConfig
	API        APIConfig
	MySQL      MySQLConfig
	Redis      RedisConfig
	Auth       AuthConfig
	Admin      AdminConfig
	Chains     ChainConfigs
	MarketData MarketDataConfig
	Hedge      HedgeConfig
	Review     ReviewConfig
}

type AppConfig struct {
	Name              string
	Env               string
	Port              int
	LogLevel          string
	TimeZone          string
	RuntimeConfigPath string
}

type APIConfig struct {
	// WithdrawExecutorEnabled gates the in-process withdraw broadcast loop.
	// Keep it enabled on at most one api-server instance when the service is
	// scaled horizontally.
	WithdrawExecutorEnabled bool
}

type MySQLConfig struct {
	DSN                string
	MaxOpenConns       int
	MaxIdleConns       int
	ConnMaxLifetimeSec int
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type AuthConfig struct {
	Domain        string
	AccessSecret  string
	RefreshSecret string
}

type AdminConfig struct {
	Wallets []string
}

type MarketDataConfig struct {
	TwelveDataAPIKey string
}

type ChainConfig struct {
	Enabled        bool
	ChainID        int64
	DisplayName    string
	LocalTestnet   bool
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
	PrivateKey     string
}

type ReviewConfig struct {
	FaucetEnabled         bool
	MockMarketDataEnabled bool
	LocalMinterPrivateKey string
}

var knownEnvKeys = []string{
	"APP_NAME",
	"APP_ENV",
	"APP_PORT",
	"LOG_LEVEL",
	"TZ",
	"RUNTIME_CONFIG_PATH",
	"WITHDRAW_EXECUTOR_ENABLED",
	"MYSQL_DSN",
	"MYSQL_MAX_OPEN_CONNS",
	"MYSQL_MAX_IDLE_CONNS",
	"MYSQL_CONN_MAX_LIFETIME_SEC",
	"REDIS_ADDR",
	"REDIS_PASSWORD",
	"REDIS_DB",
	"AUTH_DOMAIN",
	"JWT_ACCESS_SECRET",
	"JWT_REFRESH_SECRET",
	"ADMIN_WALLETS",
	"TWELVE_DATA_API_KEY",
	"ETH_ENABLED",
	"ETH_CHAIN_ID",
	"ETH_DISPLAY_NAME",
	"ETH_LOCAL_TESTNET",
	"ETH_RPC_URL",
	"ETH_CONFIRMATIONS",
	"ETH_USDC_ADDRESS",
	"ETH_VAULT_ADDRESS",
	"ETH_FACTORY_ADDRESS",
	"ARB_ENABLED",
	"ARB_CHAIN_ID",
	"ARB_DISPLAY_NAME",
	"ARB_LOCAL_TESTNET",
	"ARB_RPC_URL",
	"ARB_CONFIRMATIONS",
	"ARB_USDC_ADDRESS",
	"ARB_VAULT_ADDRESS",
	"ARB_FACTORY_ADDRESS",
	"BASE_ENABLED",
	"BASE_CHAIN_ID",
	"BASE_DISPLAY_NAME",
	"BASE_LOCAL_TESTNET",
	"BASE_RPC_URL",
	"BASE_CONFIRMATIONS",
	"BASE_USDC_ADDRESS",
	"BASE_VAULT_ADDRESS",
	"BASE_FACTORY_ADDRESS",
	"HL_API_URL",
	"HL_API_KEY",
	"HL_API_SECRET",
	"HL_ACCOUNT_ADDRESS",
	"HL_PRIVATE_KEY",
	"REVIEW_FAUCET_ENABLED",
	"REVIEW_MOCK_MARKET_DATA_ENABLED",
	"LOCAL_ANVIL_ADMIN_PRIVATE_KEY",
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
		API: APIConfig{
			WithdrawExecutorEnabled: getBool(getenv, "WITHDRAW_EXECUTOR_ENABLED", true),
		},
		MySQL: MySQLConfig{
			DSN:                strings.TrimSpace(getenv("MYSQL_DSN")),
			MaxOpenConns:       getInt(getenv, "MYSQL_MAX_OPEN_CONNS", 50),
			MaxIdleConns:       getInt(getenv, "MYSQL_MAX_IDLE_CONNS", 25),
			ConnMaxLifetimeSec: getInt(getenv, "MYSQL_CONN_MAX_LIFETIME_SEC", 300),
		},
		Redis: RedisConfig{
			Addr:     strings.TrimSpace(getenv("REDIS_ADDR")),
			Password: getenv("REDIS_PASSWORD"),
			DB:       getInt(getenv, "REDIS_DB", 0),
		},
		Auth: AuthConfig{
			Domain:        strings.TrimSpace(getenv("AUTH_DOMAIN")),
			AccessSecret:  getenv("JWT_ACCESS_SECRET"),
			RefreshSecret: getenv("JWT_REFRESH_SECRET"),
		},
		Admin: AdminConfig{
			Wallets: splitCSV(getenv("ADMIN_WALLETS")),
		},
		MarketData: MarketDataConfig{
			TwelveDataAPIKey: strings.TrimSpace(getenv("TWELVE_DATA_API_KEY")),
		},
		Chains: ChainConfigs{
			Ethereum: ChainConfig{
				Enabled:        getBool(getenv, "ETH_ENABLED", strings.TrimSpace(getenv("ETH_RPC_URL")) != ""),
				ChainID:        getInt64(getenv, "ETH_CHAIN_ID", 0),
				DisplayName:    strings.TrimSpace(getenv("ETH_DISPLAY_NAME")),
				LocalTestnet:   getBool(getenv, "ETH_LOCAL_TESTNET", false),
				RPCURL:         strings.TrimSpace(getenv("ETH_RPC_URL")),
				Confirmations:  getInt(getenv, "ETH_CONFIRMATIONS", 0),
				USDCAddress:    strings.TrimSpace(getenv("ETH_USDC_ADDRESS")),
				VaultAddress:   strings.TrimSpace(getenv("ETH_VAULT_ADDRESS")),
				FactoryAddress: strings.TrimSpace(getenv("ETH_FACTORY_ADDRESS")),
			},
			Arbitrum: ChainConfig{
				Enabled:        getBool(getenv, "ARB_ENABLED", strings.TrimSpace(getenv("ARB_RPC_URL")) != ""),
				ChainID:        getInt64(getenv, "ARB_CHAIN_ID", 0),
				DisplayName:    strings.TrimSpace(getenv("ARB_DISPLAY_NAME")),
				LocalTestnet:   getBool(getenv, "ARB_LOCAL_TESTNET", false),
				RPCURL:         strings.TrimSpace(getenv("ARB_RPC_URL")),
				Confirmations:  getInt(getenv, "ARB_CONFIRMATIONS", 0),
				USDCAddress:    strings.TrimSpace(getenv("ARB_USDC_ADDRESS")),
				VaultAddress:   strings.TrimSpace(getenv("ARB_VAULT_ADDRESS")),
				FactoryAddress: strings.TrimSpace(getenv("ARB_FACTORY_ADDRESS")),
			},
			Base: ChainConfig{
				Enabled:        getBool(getenv, "BASE_ENABLED", strings.TrimSpace(getenv("BASE_RPC_URL")) != ""),
				ChainID:        getInt64(getenv, "BASE_CHAIN_ID", 0),
				DisplayName:    strings.TrimSpace(getenv("BASE_DISPLAY_NAME")),
				LocalTestnet:   getBool(getenv, "BASE_LOCAL_TESTNET", false),
				RPCURL:         strings.TrimSpace(getenv("BASE_RPC_URL")),
				Confirmations:  getInt(getenv, "BASE_CONFIRMATIONS", 0),
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
			PrivateKey:     strings.TrimSpace(firstNonEmpty(getenv("HL_PRIVATE_KEY"), getenv("HL_API_SECRET"))),
		},
		Review: ReviewConfig{
			FaucetEnabled:         getBool(getenv, "REVIEW_FAUCET_ENABLED", false),
			MockMarketDataEnabled: getBool(getenv, "REVIEW_MOCK_MARKET_DATA_ENABLED", false),
			LocalMinterPrivateKey: strings.TrimSpace(getenv("LOCAL_ANVIL_ADMIN_PRIVATE_KEY")),
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
	if c.MySQL.MaxOpenConns <= 0 {
		errs = append(errs, fmt.Errorf("%w: MYSQL_MAX_OPEN_CONNS must be positive", errorsx.ErrInvalidArgument))
	}
	if c.MySQL.MaxIdleConns <= 0 {
		errs = append(errs, fmt.Errorf("%w: MYSQL_MAX_IDLE_CONNS must be positive", errorsx.ErrInvalidArgument))
	}
	if c.MySQL.MaxIdleConns > c.MySQL.MaxOpenConns {
		errs = append(errs, fmt.Errorf("%w: MYSQL_MAX_IDLE_CONNS must not exceed MYSQL_MAX_OPEN_CONNS", errorsx.ErrInvalidArgument))
	}
	if c.MySQL.ConnMaxLifetimeSec <= 0 {
		errs = append(errs, fmt.Errorf("%w: MYSQL_CONN_MAX_LIFETIME_SEC must be positive", errorsx.ErrInvalidArgument))
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
		if !chain.cfg.Enabled {
			continue
		}
		if chain.cfg.ChainID <= 0 {
			errs = append(errs, fmt.Errorf("%w: %s chain_id must be positive", errorsx.ErrInvalidArgument, chain.name))
		}
		if strings.TrimSpace(chain.cfg.DisplayName) == "" {
			errs = append(errs, fmt.Errorf("%w: %s display name is required when chain is enabled", errorsx.ErrInvalidArgument, chain.name))
		}
		if chain.cfg.RPCURL == "" {
			errs = append(errs, fmt.Errorf("%w: %s rpc url is required when chain is enabled", errorsx.ErrInvalidArgument, chain.name))
			continue
		}
		if chain.cfg.Confirmations <= 0 {
			errs = append(errs, fmt.Errorf("%w: %s confirmations must be positive", errorsx.ErrInvalidArgument, chain.name))
		}
		if chain.cfg.USDCAddress == "" || chain.cfg.VaultAddress == "" || chain.cfg.FactoryAddress == "" {
			errs = append(errs, fmt.Errorf("%w: %s chain config is incomplete", errorsx.ErrInvalidArgument, chain.name))
		}
	}
	seenChainIDs := make(map[int64]string, 3)
	for _, chain := range EnabledChains(c) {
		if existing, ok := seenChainIDs[chain.ChainID]; ok {
			errs = append(errs, fmt.Errorf("%w: duplicate enabled chain_id %d for %s and %s", errorsx.ErrInvalidArgument, chain.ChainID, existing, chain.Key))
			continue
		}
		seenChainIDs[chain.ChainID] = chain.Key
	}

	if c.App.Env == "prod" {
		if c.Review.FaucetEnabled {
			errs = append(errs, fmt.Errorf("%w: faucet must be disabled in prod", errorsx.ErrForbidden))
		}
		if c.Review.MockMarketDataEnabled {
			errs = append(errs, fmt.Errorf("%w: mock market data must be disabled in prod", errorsx.ErrForbidden))
		}
	}
	if (c.App.Env == "review" || c.App.Env == "dev") && c.Review.FaucetEnabled && c.Review.LocalMinterPrivateKey == "" {
		errs = append(errs, fmt.Errorf("%w: LOCAL_ANVIL_ADMIN_PRIVATE_KEY is required when review faucet is enabled", errorsx.ErrInvalidArgument))
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

	chainEnvPath := strings.TrimSpace(getenv("CHAIN_ENV_FILE"))
	if chainEnvPath == "" {
		chainEnvPath = defaultChainEnvPath(rootDir)
	} else {
		chainEnvPath = normalizeConfigPath(rootDir, chainEnvPath)
	}
	if chainEnvPath != "" {
		chainValues, err := parseEnvFile(chainEnvPath)
		if err != nil {
			return nil, err
		}
		for key, value := range chainValues {
			merged[key] = value
		}
	}

	for _, key := range knownEnvKeys {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			merged[key] = value
		}
	}

	for _, key := range []string{"REDIS_PASSWORD", "JWT_ACCESS_SECRET", "JWT_REFRESH_SECRET", "HL_API_SECRET", "HL_PRIVATE_KEY"} {
		if value := getenv(key); value != "" {
			merged[key] = value
		}
	}

	return merged, nil
}

func staticDefaults() map[string]string {
	return map[string]string{
		"APP_PORT":  "8080",
		"LOG_LEVEL": "info",
		"TZ":        "UTC",
		"REDIS_DB":  "0",
	}
}

func defaultChainEnvPath(rootDir string) string {
	if rootDir == "" {
		return ""
	}
	candidates := []string{
		filepath.Join(rootDir, "deploy", "env", "local-chains.env"),
		filepath.Join(rootDir, ".local", "contracts.env"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
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

func getInt64(getenv func(string) string, key string, fallback int64) int64 {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
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
