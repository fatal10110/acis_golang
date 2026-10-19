package maker

import "testing"

// TestParseInt32 pins the memo integer parse to a decimal 32-bit value: a
// value outside int32 or not a number is refused.
func TestParseInt32(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int32
		ok   bool
	}{
		{"8", 8, true},
		{"-5", -5, true},
		{"2147483647", 2147483647, true},
		{"-2147483648", -2147483648, true},
		{"2147483648", 0, false},
		{"-2147483649", 0, false},
		{"", 0, false},
		{"8a", 0, false},
	} {
		got, ok := parseInt32(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseInt32(%q) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
