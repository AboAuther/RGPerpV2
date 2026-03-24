package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStaticConfigFromEnv_Success(t *testing.T) {
	env := map[string]string{
		"APP_NAME":                        "rgperp",
		"APP_ENV":                         "review",
		"APP_PORT":                        "8080",
		"LOG_LEVEL":                       "info",
		"TZ":                              "UTC",
		"RUNTIME_CONFIG_PATH":             "deploy/config/runtime/review.yaml",
		"MYSQL_DSN":                       "mysql",
		"RABBITMQ_URL":                    "amqp://guest:guest@localhost:5672/",
		"AUTH_DOMAIN":                     "localhost",
		"JWT_ACCESS_SECRET":               "access",
		"JWT_REFRESH_SECRET":              "refresh",
		"ADMIN_WALLETS":                   "0xabc,0xdef",
		"ETH_CHAIN_ID":                    "31337",
		"ETH_DISPLAY_NAME":                "Local Ethereum",
		"ETH_RPC_URL":                     "http://eth",
		"ETH_CONFIRMATIONS":               "1",
		"ETH_USDC_ADDRESS":                "0x0000000000000000000000000000000000000001",
		"ETH_VAULT_ADDRESS":               "0x0000000000000000000000000000000000000002",
		"ETH_FACTORY_ADDRESS":             "0x0000000000000000000000000000000000000003",
		"REVIEW_FAUCET_ENABLED":           "true",
		"REVIEW_MOCK_MARKET_DATA_ENABLED": "true",
		"LOCAL_ANVIL_ADMIN_PRIVATE_KEY":   "0xabc123",
	}

	cfg, err := LoadStaticConfigFromEnv(func(key string) string {
		return env[key]
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if cfg.App.Env != "review" {
		t.Fatalf("unexpected env: %s", cfg.App.Env)
	}
	if cfg.App.RuntimeConfigPath != "deploy/config/runtime/review.yaml" {
		t.Fatalf("unexpected runtime config path: %s", cfg.App.RuntimeConfigPath)
	}
	if !cfg.Review.FaucetEnabled {
		t.Fatal("expected faucet enabled")
	}
	if len(cfg.Admin.Wallets) != 2 {
		t.Fatalf("expected admin wallets to load")
	}
	if !cfg.Chains.Ethereum.Enabled {
		t.Fatal("expected ethereum enabled")
	}
	if cfg.Chains.Ethereum.ChainID != 31337 {
		t.Fatalf("expected configured ethereum chain id, got %d", cfg.Chains.Ethereum.ChainID)
	}
}

func TestLoadStaticConfigWithOptions_LoadsCommonAndEnvSpecificFiles(t *testing.T) {
	rootDir := t.TempDir()
	writeTestFile(t, filepath.Join(rootDir, "deploy", "env", "common.env"), strings.Join([]string{
		"APP_NAME=rgperp",
		"APP_ENV=review",
		"APP_PORT=8080",
		"LOG_LEVEL=info",
		"TZ=UTC",
		"RUNTIME_CONFIG_PATH=deploy/config/runtime/review.yaml",
		"MYSQL_DSN=mysql-common",
		"RABBITMQ_URL=amqp://guest:guest@localhost:5672/",
		"AUTH_DOMAIN=localhost",
		"JWT_ACCESS_SECRET=access-common",
		"JWT_REFRESH_SECRET=refresh-common",
		"REDIS_ADDR=localhost:6379",
		"",
	}, "\n"))
	writeTestFile(t, filepath.Join(rootDir, "deploy", "env", "review.env"), strings.Join([]string{
		"APP_PORT=18080",
		"JWT_ACCESS_SECRET=access-review",
		"REVIEW_FAUCET_ENABLED=true",
		"REVIEW_MOCK_MARKET_DATA_ENABLED=true",
		"LOCAL_ANVIL_ADMIN_PRIVATE_KEY=0xabc123",
		"",
	}, "\n"))
	writeTestFile(t, filepath.Join(rootDir, "deploy", "env", "local-chains.env"), strings.Join([]string{
		"ETH_ENABLED=true",
		"ETH_CHAIN_ID=31337",
		"ETH_DISPLAY_NAME=Local Ethereum",
		"ETH_LOCAL_TESTNET=true",
		"ETH_RPC_URL=http://127.0.0.1:8545",
		"ETH_CONFIRMATIONS=1",
		"ETH_USDC_ADDRESS=0x0000000000000000000000000000000000000001",
		"ETH_VAULT_ADDRESS=0x0000000000000000000000000000000000000002",
		"ETH_FACTORY_ADDRESS=0x0000000000000000000000000000000000000003",
		"",
	}, "\n"))

	cfg, err := LoadStaticConfigWithOptions(LoadOptions{
		RootDir: rootDir,
		Getenv: func(key string) string {
			if key == "JWT_REFRESH_SECRET" {
				return "refresh-override"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if cfg.App.Port != 18080 {
		t.Fatalf("expected env-specific APP_PORT override, got %d", cfg.App.Port)
	}
	expectedRuntimePath := filepath.Join(rootDir, "deploy", "config", "runtime", "review.yaml")
	if cfg.App.RuntimeConfigPath != expectedRuntimePath {
		t.Fatalf("expected normalized runtime config path %s, got %s", expectedRuntimePath, cfg.App.RuntimeConfigPath)
	}
	if cfg.Auth.AccessSecret != "access-review" {
		t.Fatalf("expected review access secret, got %s", cfg.Auth.AccessSecret)
	}
	if cfg.Auth.RefreshSecret != "refresh-override" {
		t.Fatalf("expected process env override, got %s", cfg.Auth.RefreshSecret)
	}
	if cfg.Chains.Ethereum.ChainID != 31337 || cfg.Chains.Ethereum.Confirmations != 1 {
		t.Fatalf("expected chain config to load from local env file, got %+v", cfg.Chains.Ethereum)
	}
}

func TestLoadStaticConfigFromEnv_MissingRequired(t *testing.T) {
	_, err := LoadStaticConfigFromEnv(func(key string) string { return "" })
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadStaticConfigFromEnv_ProdDisablesReviewFeatures(t *testing.T) {
	env := map[string]string{
		"APP_NAME":              "rgperp",
		"APP_ENV":               "prod",
		"LOG_LEVEL":             "info",
		"TZ":                    "UTC",
		"RUNTIME_CONFIG_PATH":   "deploy/config/runtime/prod.yaml",
		"MYSQL_DSN":             "mysql",
		"RABBITMQ_URL":          "amqp://guest:guest@localhost:5672/",
		"AUTH_DOMAIN":           "localhost",
		"JWT_ACCESS_SECRET":     "access",
		"JWT_REFRESH_SECRET":    "refresh",
		"REVIEW_FAUCET_ENABLED": "true",
	}

	_, err := LoadStaticConfigFromEnv(func(key string) string {
		return env[key]
	})
	if err == nil {
		t.Fatal("expected prod validation error")
	}
}

func TestLoadStaticConfigFromEnv_RejectsDuplicateEnabledChainIDs(t *testing.T) {
	env := map[string]string{
		"APP_NAME":            "rgperp",
		"APP_ENV":             "dev",
		"LOG_LEVEL":           "info",
		"TZ":                  "UTC",
		"RUNTIME_CONFIG_PATH": "deploy/config/runtime/dev.yaml",
		"MYSQL_DSN":           "mysql",
		"RABBITMQ_URL":        "amqp://guest:guest@localhost:5672/",
		"AUTH_DOMAIN":         "localhost",
		"JWT_ACCESS_SECRET":   "access",
		"JWT_REFRESH_SECRET":  "refresh",
		"ETH_ENABLED":         "true",
		"ETH_CHAIN_ID":        "31337",
		"ETH_DISPLAY_NAME":    "Local Ethereum",
		"ETH_CONFIRMATIONS":   "1",
		"ETH_RPC_URL":         "http://eth",
		"ETH_USDC_ADDRESS":    "0x0000000000000000000000000000000000000001",
		"ETH_VAULT_ADDRESS":   "0x0000000000000000000000000000000000000002",
		"ETH_FACTORY_ADDRESS": "0x0000000000000000000000000000000000000003",
		"ARB_ENABLED":         "true",
		"ARB_CHAIN_ID":        "31337",
		"ARB_DISPLAY_NAME":    "Local Arbitrum",
		"ARB_CONFIRMATIONS":   "1",
		"ARB_RPC_URL":         "http://arb",
		"ARB_USDC_ADDRESS":    "0x0000000000000000000000000000000000000011",
		"ARB_VAULT_ADDRESS":   "0x0000000000000000000000000000000000000012",
		"ARB_FACTORY_ADDRESS": "0x0000000000000000000000000000000000000013",
	}

	_, err := LoadStaticConfigFromEnv(func(key string) string {
		return env[key]
	})
	if err == nil {
		t.Fatal("expected duplicate chain id validation error")
	}
}

func TestParseEnvFile_InvalidLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.env")
	writeTestFile(t, path, "BROKEN_LINE")

	_, err := parseEnvFile(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadStaticConfigFromEnv_EnabledChainRequiresExplicitConfig(t *testing.T) {
	env := map[string]string{
		"APP_NAME":            "rgperp",
		"APP_ENV":             "dev",
		"LOG_LEVEL":           "info",
		"TZ":                  "UTC",
		"RUNTIME_CONFIG_PATH": "deploy/config/runtime/dev.yaml",
		"MYSQL_DSN":           "mysql",
		"RABBITMQ_URL":        "amqp://guest:guest@localhost:5672/",
		"AUTH_DOMAIN":         "localhost",
		"JWT_ACCESS_SECRET":   "access",
		"JWT_REFRESH_SECRET":  "refresh",
		"ETH_ENABLED":         "true",
		"ETH_RPC_URL":         "http://eth",
		"ETH_USDC_ADDRESS":    "0x0000000000000000000000000000000000000001",
		"ETH_VAULT_ADDRESS":   "0x0000000000000000000000000000000000000002",
		"ETH_FACTORY_ADDRESS": "0x0000000000000000000000000000000000000003",
	}

	_, err := LoadStaticConfigFromEnv(func(key string) string {
		return env[key]
	})
	if err == nil {
		t.Fatal("expected validation error for missing explicit chain config")
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
