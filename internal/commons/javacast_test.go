package commons

import (
	"math"
	"testing"
)

func TestJavaInt(t *testing.T) {
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
		if got := JavaInt(tc.in); got != tc.want {
			t.Errorf("JavaInt(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestJavaLong(t *testing.T) {
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
		if got := JavaLong(tc.in); got != tc.want {
			t.Errorf("JavaLong(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestJavaRound pins Math.round(double): half up, toward positive
// infinity for negative halves, exact just under a half, saturating.
func TestJavaRound(t *testing.T) {
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
		if got := JavaRound(tc.in); got != tc.want {
			t.Errorf("JavaRound(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
