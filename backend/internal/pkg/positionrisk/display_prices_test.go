package positionrisk

import (
	"testing"

	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
)

func TestComputeDisplayPrices_LongBecomesMoreConservativeWithExecutionSlippage(t *testing.T) {
	noSlippageLiq, noSlippageBk := ComputeDisplayPrices("LONG", "1", "100", "10", "0.05", "0.01", "1", 0)
	withSlippageLiq, withSlippageBk := ComputeDisplayPrices("LONG", "1", "100", "10", "0.05", "0.01", "1", 50)

	if !decimalx.MustFromString(withSlippageLiq).GreaterThan(decimalx.MustFromString(noSlippageLiq)) {
		t.Fatalf("expected long liquidation trigger to move earlier with slippage, no_slippage=%s with_slippage=%s", noSlippageLiq, withSlippageLiq)
	}
	if !decimalx.MustFromString(withSlippageBk).GreaterThan(decimalx.MustFromString(noSlippageBk)) {
		t.Fatalf("expected long bankruptcy trigger to move earlier with slippage, no_slippage=%s with_slippage=%s", noSlippageBk, withSlippageBk)
	}
}

func TestComputeDisplayPrices_ShortBecomesMoreConservativeWithExecutionSlippage(t *testing.T) {
	noSlippageLiq, noSlippageBk := ComputeDisplayPrices("SHORT", "1", "100", "10", "0.05", "0.01", "1", 0)
	withSlippageLiq, withSlippageBk := ComputeDisplayPrices("SHORT", "1", "100", "10", "0.05", "0.01", "1", 50)

	if !decimalx.MustFromString(withSlippageLiq).LessThan(decimalx.MustFromString(noSlippageLiq)) {
		t.Fatalf("expected short liquidation trigger to move earlier with slippage, no_slippage=%s with_slippage=%s", noSlippageLiq, withSlippageLiq)
	}
	if !decimalx.MustFromString(withSlippageBk).LessThan(decimalx.MustFromString(noSlippageBk)) {
		t.Fatalf("expected short bankruptcy trigger to move earlier with slippage, no_slippage=%s with_slippage=%s", noSlippageBk, withSlippageBk)
	}
}
