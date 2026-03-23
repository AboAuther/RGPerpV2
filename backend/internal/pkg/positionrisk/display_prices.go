package positionrisk

import (
	"strings"

	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
)

func ComputeDisplayPrices(side string, qty string, avgEntryPrice string, initialMargin string, maintenanceRate string, liquidationFeeRate string, contractMultiplier string) (string, string) {
	zero := decimalx.MustFromString("0")
	qtyDecimal := decimalx.MustFromString(defaultDecimal(qty)).Abs()
	entryPrice := decimalx.MustFromString(defaultDecimal(avgEntryPrice))
	initialMarginDecimal := decimalx.MustFromString(defaultDecimal(initialMargin))
	maintenanceRateDecimal := decimalx.MustFromString(defaultDecimal(maintenanceRate))
	liquidationFeeRateDecimal := decimalx.MustFromString(defaultDecimal(liquidationFeeRate))
	multiplierDecimal := decimalx.MustFromString(defaultDecimal(contractMultiplier))
	if !qtyDecimal.GreaterThan(zero) || !multiplierDecimal.GreaterThan(zero) {
		return "0", "0"
	}

	marginPerUnit := initialMarginDecimal.Div(qtyDecimal).Div(multiplierDecimal)
	thresholdRate := maintenanceRateDecimal.Add(liquidationFeeRateDecimal)

	switch side {
	case "SHORT":
		bankruptcy := entryPrice.Add(marginPerUnit)
		liquidationDenominator := decimalx.MustFromString("1").Add(thresholdRate)
		if !liquidationDenominator.GreaterThan(zero) {
			return "0", bankruptcy.String()
		}
		liquidation := entryPrice.Add(marginPerUnit).Div(liquidationDenominator)
		return liquidation.String(), bankruptcy.String()
	default:
		bankruptcy := entryPrice.Sub(marginPerUnit)
		if bankruptcy.LessThan(zero) {
			bankruptcy = zero
		}
		liquidationDenominator := decimalx.MustFromString("1").Sub(thresholdRate)
		if !liquidationDenominator.GreaterThan(zero) {
			return "0", bankruptcy.String()
		}
		liquidation := entryPrice.Sub(marginPerUnit).Div(liquidationDenominator)
		if liquidation.LessThan(zero) {
			liquidation = zero
		}
		return liquidation.String(), bankruptcy.String()
	}
}

func defaultDecimal(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return value
}
