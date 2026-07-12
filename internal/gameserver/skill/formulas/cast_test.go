package formulas

import (
	"math"
	"testing"
)

func TestCastBreakRateAndRoll(t *testing.T) {
	rate := CastBreakRate(100, 30, nil)
	const want = 16.055512754639892
	if math.Abs(rate-want) > 0.000000000001 {
		t.Fatalf("CastBreakRate() = %.15f, want %.15f", rate, want)
	}

	doubled := CastBreakRate(100, 30, func(base float64) float64 { return base * 2 })
	if math.Abs(doubled-want*2) > 0.000000000001 {
		t.Fatalf("CastBreakRate() with attack-cancel stat = %.15f, want %.15f", doubled, want*2)
	}

	if !CastBreaks(1, 0) {
		t.Fatal("CastBreaks(1, 0) = false, want true")
	}
	if CastBreaks(99, 99) {
		t.Fatal("CastBreaks(99, 99) = true, want false")
	}
	if !CastBreaks(1000, 98) {
		t.Fatal("CastBreaks clamps high rates to 99 and should beat roll 98")
	}
}
