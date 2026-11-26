package network

import (
	"testing"
)

// TestFormatPercentMatchesDecimalFormat pins formatPercent against
// DecimalFormat("#.###"): half-even rounding on the double's exact value,
// trailing zeros dropped, no leading zero before the point, "0" (signed
// for a negative value) once the value rounds away.
func TestFormatPercentMatchesDecimalFormat(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{0.0001, "0"},
		{-0.0001, "-0"},
		{0.5, ".5"},
		{-0.5, "-.5"},
		{1, "1"},
		{70, "70"},
		{100, "100"},
		{12.3456, "12.346"},
		// Exactly halfway in binary: half-even keeps the even digit.
		{0.0625, ".062"},
		{0.1875, ".188"},
		// 0.0015 is stored just above the halfway point.
		{0.0015, ".002"},
		{0.125, ".125"},
		{33.33333, "33.333"},
		{1e7, "10000000"},
	} {
		if got := formatPercent(tc.in); got != tc.want {
			t.Errorf("formatPercent(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestFormatIntArrayMatchesArraysToString pins formatIntArray to
// Arrays.toString(int[]).
func TestFormatIntArrayMatchesArraysToString(t *testing.T) {
	for _, tc := range []struct {
		in   []int
		want string
	}{
		{nil, "null"},
		{[]int{}, "[]"},
		{[]int{0, 0, 80, 120}, "[0, 0, 80, 120]"},
		{[]int{-1}, "[-1]"},
	} {
		if got := formatIntArray(tc.in); got != tc.want {
			t.Errorf("formatIntArray(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
