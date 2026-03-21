package config

import "testing"

func TestLoadStaticConfigFromEnv_Success(t *testing.T) {
	env := map[string]string{
		"APP_NAME":                        "rgperp",
		"APP_ENV":                         "review",
		"APP_PORT":                        "8080",
		"MYSQL_DSN":                       "mysql",
		"RABBITMQ_URL":                    "amqp://guest:guest@localhost:5672/",
		"AUTH_DOMAIN":                     "localhost",
		"JWT_ACCESS_SECRET":               "access",
		"JWT_REFRESH_SECRET":              "refresh",
		"ETH_RPC_URL":                     "http://eth",
		"ETH_USDC_ADDRESS":                "0x0000000000000000000000000000000000000001",
		"ETH_VAULT_ADDRESS":               "0x0000000000000000000000000000000000000002",
		"ETH_FACTORY_ADDRESS":             "0x0000000000000000000000000000000000000003",
		"REVIEW_FAUCET_ENABLED":           "true",
		"REVIEW_MOCK_MARKET_DATA_ENABLED": "true",
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
	if !cfg.Review.FaucetEnabled {
		t.Fatal("expected faucet enabled")
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
