package positionrisk

import (
	"fmt"
	"strings"

	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
)

func ComputeDisplayPrices(side string, qty string, avgEntryPrice string, initialMargin string, maintenanceRate string, liquidationFeeRate string, contractMultiplier string, extraSlippageBps int) (string, string) {
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
	slippageFactor := decimalx.MustFromString(fmt.Sprintf("%d", maxInt(extraSlippageBps, 0))).Div(decimalx.MustFromString("10000"))

	switch side {
	case "SHORT":
		executableBankruptcy := entryPrice.Add(marginPerUnit)
		bankruptcyDenominator := decimalx.MustFromString("1").Add(slippageFactor)
		if !bankruptcyDenominator.GreaterThan(zero) {
			return "0", "0"
		}
		bankruptcy := executableBankruptcy.Div(bankruptcyDenominator)
		liquidationDenominator := decimalx.MustFromString("1").Add(slippageFactor).Mul(decimalx.MustFromString("1").Add(thresholdRate))
		if !liquidationDenominator.GreaterThan(zero) {
			return "0", bankruptcy.String()
		}
		liquidation := entryPrice.Add(marginPerUnit).Div(liquidationDenominator)
		return liquidation.String(), bankruptcy.String()
	default:
		executableBankruptcy := entryPrice.Sub(marginPerUnit)
		if executableBankruptcy.LessThan(zero) {
			executableBankruptcy = zero
		}
		bankruptcyDenominator := decimalx.MustFromString("1").Sub(slippageFactor)
		if !bankruptcyDenominator.GreaterThan(zero) {
			return "0", executableBankruptcy.String()
		}
		bankruptcy := executableBankruptcy.Div(bankruptcyDenominator)
		liquidationDenominator := decimalx.MustFromString("1").Sub(slippageFactor).Mul(decimalx.MustFromString("1").Sub(thresholdRate))
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

func IsAtOrBeyondLiquidation(side string, markPrice string, liquidationPrice string) bool {
	liq := strings.TrimSpace(liquidationPrice)
	if liq == "" || liq == "0" {
		return false
	}
	mark := decimalx.MustFromString(defaultDecimal(markPrice))
	threshold := decimalx.MustFromString(defaultDecimal(liq))
	switch strings.ToUpper(strings.TrimSpace(side)) {
	case "SHORT":
		return mark.GreaterThanOrEqual(threshold)
	default:
		return mark.LessThanOrEqual(threshold)
	}
}

func defaultDecimal(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0"
	}
	return value
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
