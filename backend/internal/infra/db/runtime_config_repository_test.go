package db

import (
	"context"
	"testing"
	"time"

	"github.com/xiaobao/rgperp/backend/internal/config"
)

func TestRuntimeConfigRepository_LoadActiveConfigValuesKeepsLatestPerPairScope(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRuntimeConfigRepository(db)
	now := time.Now().UTC()

	items := []ConfigItemModel{
		{
			ConfigKey:   "market.max_leverage",
			ScopeType:   config.ConfigScopeTypePair,
			ScopeValue:  "BTC-USDC",
			Version:     1,
			ValueJSON:   `"25"`,
			EffectiveAt: now,
			Status:      "SUPERSEDED",
			CreatedBy:   "tester",
			Reason:      "old btc value",
			CreatedAt:   now,
		},
		{
			ConfigKey:   "market.max_leverage",
			ScopeType:   config.ConfigScopeTypePair,
			ScopeValue:  "BTC-USDC",
			Version:     2,
			ValueJSON:   `"50"`,
			EffectiveAt: now.Add(time.Second),
			Status:      "ACTIVE",
			CreatedBy:   "tester",
			Reason:      "new btc value",
			CreatedAt:   now.Add(time.Second),
		},
		{
			ConfigKey:   "market.max_leverage",
			ScopeType:   config.ConfigScopeTypePair,
			ScopeValue:  "ADA-USDC",
			Version:     1,
			ValueJSON:   `"100"`,
			EffectiveAt: now.Add(2 * time.Second),
			Status:      "ACTIVE",
			CreatedBy:   "tester",
			Reason:      "ada value",
			CreatedAt:   now.Add(2 * time.Second),
		},
		{
			ConfigKey:   "market.session_policy",
			ScopeType:   config.ConfigScopeTypePair,
			ScopeValue:  "ADA-USDC",
			Version:     1,
			ValueJSON:   `"ALWAYS_OPEN"`,
			EffectiveAt: now.Add(2 * time.Second),
			Status:      "ACTIVE",
			CreatedBy:   "tester",
			Reason:      "ada session",
			CreatedAt:   now.Add(2 * time.Second),
		},
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatalf("seed config items: %v", err)
	}

	values, err := repo.LoadActiveConfigValues(context.Background(), config.ConfigScopeTypePair, "")
	if err != nil {
		t.Fatalf("LoadActiveConfigValues: %v", err)
	}

	got := make(map[string]string, len(values))
	for _, item := range values {
		got[item.ScopeValue+"|"+item.Key] = string(item.ValueJSON)
	}

	if got["BTC-USDC|market.max_leverage"] != `"50"` {
		t.Fatalf("expected BTC latest max leverage, got %#v", got["BTC-USDC|market.max_leverage"])
	}
	if got["ADA-USDC|market.max_leverage"] != `"100"` {
		t.Fatalf("expected ADA max leverage, got %#v", got["ADA-USDC|market.max_leverage"])
	}
	if got["ADA-USDC|market.session_policy"] != `"ALWAYS_OPEN"` {
		t.Fatalf("expected ADA session policy, got %#v", got["ADA-USDC|market.session_policy"])
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 active scoped values, got %d: %#v", len(got), got)
	}
}
