package network

import "testing"

func TestParseConditionIntLiteralBases(t *testing.T) {
	cases := []struct {
		raw    string
		want   int
		wantOK bool
	}{
		{"10", 10, true},
		{"0x1F", 31, true},
		{"010", 8, true},
		{"", 0, false},
		{"abc", 0, false},
		{"0x100000000", 0, false},
	}
	for _, c := range cases {
		got, ok := parseConditionInt(c.raw)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("parseConditionInt(%q) = (%d, %v), want (%d, %v)", c.raw, got, ok, c.want, c.wantOK)
		}
	}
}
