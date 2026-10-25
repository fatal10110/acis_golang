package queenant

import (
	"math"
	"testing"
)

// TestSaturatedInt checks the double-to-int cast the hate weights use:
// truncation toward zero, clamping at both ends of the int32 range, and
// NaN as 0.
func TestSaturatedInt(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want int32
	}{
		{2.73, 2},
		{-2.73, -2},
		{float64(math.MaxInt32) * 2.73, math.MaxInt32},
		{float64(math.MinInt32) * 2.73, math.MinInt32},
		{math.NaN(), 0},
	} {
		if got := saturatedInt(c.in); got != c.want {
			t.Errorf("saturatedInt(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
