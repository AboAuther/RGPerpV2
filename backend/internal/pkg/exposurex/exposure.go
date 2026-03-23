package exposurex

import (
	"fmt"

	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
)

func SignedNetQty(longQty string, shortQty string) decimalx.Decimal {
	return decimalx.MustFromString(longQty).Sub(decimalx.MustFromString(shortQty))
}

func SignedNetNotional(longQty string, shortQty string, markPrice string, contractMultiplier string) decimalx.Decimal {
	return SignedNetQty(longQty, shortQty).
		Mul(decimalx.MustFromString(markPrice)).
		Mul(decimalx.MustFromString(contractMultiplier))
}

func SignedDeltaNotional(side string, qty string, referencePrice string, contractMultiplier string) decimalx.Decimal {
	sign := decimalx.MustFromString("-1")
	if side == "BUY" {
		sign = decimalx.MustFromString("1")
	}
	return sign.
		Mul(decimalx.MustFromString(qty)).
		Mul(decimalx.MustFromString(referencePrice)).
		Mul(decimalx.MustFromString(contractMultiplier))
}

func WorsensExposure(currentSigned decimalx.Decimal, deltaSigned decimalx.Decimal) bool {
	return currentSigned.Add(deltaSigned).Abs().GreaterThan(currentSigned.Abs())
}

func ExceedsHardLimit(currentSigned decimalx.Decimal, deltaSigned decimalx.Decimal, hardLimit string) bool {
	limit := decimalx.MustFromString(hardLimit)
	if !limit.GreaterThan(decimalx.MustFromString("0")) {
		return false
	}
	post := currentSigned.Add(deltaSigned)
	return post.Abs().GreaterThan(limit) && WorsensExposure(currentSigned, deltaSigned)
}

func DirectionAdjustmentBps(currentSigned decimalx.Decimal, orderSide string, hardLimit string, maxBps int) int {
	if maxBps <= 0 || currentSigned.IsZero() {
		return 0
	}
	limit := decimalx.MustFromString(hardLimit)
	if !limit.GreaterThan(decimalx.MustFromString("0")) {
		return 0
	}
	utilization := currentSigned.Abs().Div(limit)
	if utilization.GreaterThan(decimalx.MustFromString("1")) {
		utilization = decimalx.MustFromString("1")
	}
	bpsValue := utilization.Mul(decimalx.MustFromString(fmt.Sprintf("%d", maxBps)))
	bps := int(bpsValue.IntPart())
	if currentSigned.GreaterThan(decimalx.MustFromString("0")) {
		if orderSide == "BUY" {
			return bps
		}
		return -bps
	}
	if orderSide == "SELL" {
		return bps
	}
	return -bps
}
