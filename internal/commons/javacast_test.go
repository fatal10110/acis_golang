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
