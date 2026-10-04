package commons

import (
	"math"
	"testing"
)

// The expected strings are what the reference's String.valueOf(double)
// prints for each value.
func TestJavaDouble(t *testing.T) {
	// A sum taken at run time, unlike a constant one, rounds as the
	// reference's double arithmetic does.
	base, bonus := 77.3, 1.1
	for _, tc := range []struct {
		v    float64
		want string
	}{
		{60, "60.0"},
		{70.345, "70.345"},
		{base + bonus, "78.39999999999999"},
		{0, "0.0"},
		{-1.5, "-1.5"},
		{0.001, "0.001"},
		{0.0001, "1.0E-4"},
		{9999999, "9999999.0"},
		{1e7, "1.0E7"},
		{1.25e10, "1.25E10"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
	} {
		if got := JavaDouble(tc.v); got != tc.want {
			t.Errorf("JavaDouble(%v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}
