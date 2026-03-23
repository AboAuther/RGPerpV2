package db

import (
	"context"
	"fmt"
	"sort"

	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
	"gorm.io/gorm"
)

func loadRiskTiersBySymbol(ctx context.Context, tx *gorm.DB, symbolIDs []uint64) (map[uint64][]RiskTierModel, error) {
	_ = ctx
	if len(symbolIDs) == 0 {
		return map[uint64][]RiskTierModel{}, nil
	}

	var tiers []RiskTierModel
	if err := tx.Where("symbol_id IN ?", symbolIDs).Order("symbol_id ASC, max_notional ASC, tier_level ASC").Find(&tiers).Error; err != nil {
		return nil, err
	}

	out := make(map[uint64][]RiskTierModel, len(symbolIDs))
	for _, tier := range tiers {
		out[tier.SymbolID] = append(out[tier.SymbolID], tier)
	}
	for symbolID := range out {
		sort.Slice(out[symbolID], func(i, j int) bool {
			left := decimalx.MustFromString(out[symbolID][i].MaxNotional)
			right := decimalx.MustFromString(out[symbolID][j].MaxNotional)
			if left.Equal(right) {
				return out[symbolID][i].TierLevel < out[symbolID][j].TierLevel
			}
			return left.LessThan(right)
		})
	}
	return out, nil
}

func selectRiskTierByNotional(tiers []RiskTierModel, notional decimalx.Decimal) (RiskTierModel, error) {
	if len(tiers) == 0 {
		return RiskTierModel{}, fmt.Errorf("%w: risk tier missing", errorsx.ErrNotFound)
	}
	target := notional.Abs()
	for _, tier := range tiers {
		maxNotional := decimalx.MustFromString(tier.MaxNotional)
		if !maxNotional.GreaterThan(decimalx.MustFromString("0")) || target.LessThanOrEqual(maxNotional) {
			return tier, nil
		}
	}
	return tiers[len(tiers)-1], nil
}
