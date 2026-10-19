package commons

import (
	"math"
	"testing"
)

func TestTruncInt32Sat(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want int32
	}{
		{1.9, 1},
		{-1.9, -1},
		{math.NaN(), 0},
		{math.Inf(1), math.MaxInt32},
		{math.Inf(-1), math.MinInt32},
		{3e9, math.MaxInt32},
		{-3e9, math.MinInt32},
		{math.MaxInt32, math.MaxInt32},
	} {
		if got := TruncInt32Sat(tc.in); got != tc.want {
			t.Errorf("TruncInt32Sat(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestTruncInt64Sat(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want int64
	}{
		{1.9, 1},
		{-1.9, -1},
		{math.NaN(), 0},
		{math.Inf(1), math.MaxInt64},
		{math.Inf(-1), math.MinInt64},
		{1e19, math.MaxInt64},
		{-1e19, math.MinInt64},
		{3e9, 3_000_000_000},
	} {
		if got := TruncInt64Sat(tc.in); got != tc.want {
			t.Errorf("TruncInt64Sat(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestRoundHalfUp pins Math.round(double): half up, toward positive
// infinity for negative halves, exact just under a half, saturating.
func TestRoundHalfUp(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want int64
	}{
		{2.5, 3},
		{2.4, 2},
		{-2.5, -2},
		{-2.6, -3},
		{0.49999999999999994, 0},
		{4503599627370497, 4503599627370497},
		{math.NaN(), 0},
		{math.Inf(1), math.MaxInt64},
		{math.Inf(-1), math.MinInt64},
		{1e19, math.MaxInt64},
	} {
		if got := RoundHalfUp(tc.in); got != tc.want {
			t.Errorf("RoundHalfUp(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
