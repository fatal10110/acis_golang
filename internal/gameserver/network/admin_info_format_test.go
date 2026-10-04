package network

import (
	"slices"
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

// TestJavaTokensSplitsOnSpacesOnly pins javaTokens to a StringTokenizer
// with a space delimiter: other whitespace stays inside a token.
func TestJavaTokensSplitsOnSpacesOnly(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"", []string{""}},
		{"admin_info", []string{"admin_info"}},
		{"admin_info  drop 2 ", []string{"admin_info", "drop", "2"}},
		{"admin_info\tdrop", []string{"admin_info\tdrop"}},
	} {
		if got := javaTokens(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("javaTokens(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestJavaIntArrayMatchesArraysToString pins javaIntArray to
// Arrays.toString(int[]).
func TestJavaIntArrayMatchesArraysToString(t *testing.T) {
	for _, tc := range []struct {
		in   []int
		want string
	}{
		{nil, "null"},
		{[]int{}, "[]"},
		{[]int{0, 0, 80, 120}, "[0, 0, 80, 120]"},
		{[]int{-1}, "[-1]"},
	} {
		if got := javaIntArray(tc.in); got != tc.want {
			t.Errorf("javaIntArray(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
