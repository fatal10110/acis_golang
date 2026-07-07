package datadiff

import "testing"

func TestFormatFloat(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{100, "100"},
		{1.5, "1.5"},
		{1.1, "1.1"},
		{1.123456, "1.123456"},
		{-1.5, "-1.5"},
		{-0.0, "0"},
		{0.05, "0.05"},
		{132.6, "132.6"},
		// A value whose 7th decimal digit sits exactly on a rounding tie: a
		// fixed-precision format would force a round-half-up-vs-even
		// choice here; the shortest round-trip form has no such ambiguity
		// and reproduces every digit the source literal actually needs.
		{244.2552175, "244.2552175"},
		{2889.881883, "2889.881883"},
	}
	for _, c := range cases {
		if got := FormatFloat(c.in); got != c.want {
			t.Errorf("FormatFloat(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
