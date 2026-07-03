package commons

import "testing"

// cases are known-good vectors for the algorithm, not values re-derived
// from the formula under test.
func TestLegacyStringHash(t *testing.T) {
	cases := []struct {
		in   string
		want int32
	}{
		{"", 0},
		{"a", 97},
		{"hello", 99162322},
		{"StatSet", -232503986},
		{"multisell/1.xml", 1568074198},
		{"Hello, World!", 1498789909},
		{"The quick brown fox jumps over the lazy dog", -609428141},
		{"123456", 1450575459},
		{"日本語", 25921943},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", -1203646688},
	}

	for _, c := range cases {
		if got := LegacyStringHash(c.in); got != c.want {
			t.Errorf("LegacyStringHash(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
