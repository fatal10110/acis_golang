package commons

import (
	"math"
	"testing"
)

// The expected strings are what the reference's String.format with
// Locale.ENGLISH prints for each value.
func TestJavaFixed(t *testing.T) {
	cases := []struct {
		f        float64
		decimals int
		want     string
	}{
		{1.25, 1, "1.3"},
		{1.25, 2, "1.25"},
		{0.15, 1, "0.2"},
		{1.005, 2, "1.01"},
		{2.675, 2, "2.68"},
		{0, 2, "0.00"},
		{0, 1, "0.0"},
		{9.96, 1, "10.0"},
		{99.995, 2, "100.00"},
		{3.333333333333333, 1, "3.3"},
		{12.5, 0, "13"},
		{0.01, 2, "0.01"},
		{-1.25, 1, "-1.3"},
		{1e21, 1, "1000000000000000000000.0"},
		{math.NaN(), 2, "NaN"},
		{math.Inf(1), 2, "Infinity"},
	}
	for _, c := range cases {
		if got := JavaFixed(c.f, c.decimals); got != c.want {
			t.Errorf("JavaFixed(%v, %d) = %q, want %q", c.f, c.decimals, got, c.want)
		}
	}
}
