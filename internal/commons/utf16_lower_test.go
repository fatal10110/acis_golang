package commons

import (
	"slices"
	"strings"
	"testing"
)

func TestUTF16LowerKeyLowersByCodePointAndKeepsUnits(t *testing.T) {
	cases := []struct {
		in    string
		units []uint16
	}{
		{"", []uint16{}},
		{"AbÇ", []uint16{'a', 'b', 0x00E7}},
		{"ДЖ", []uint16{0x0434, 0x0436}},
		{"A\xED\xA0\xBDB", []uint16{'a', 0xD83D, 'b'}},
		{"\xED\xB8\x80", []uint16{0xDE00}},
		{"\U0001F600", []uint16{0xD83D, 0xDE00}},
		{"\xED\xA0\xBD\xED\xB8\x80", []uint16{0xD83D, 0xDE00}},
		{"\U00010400", []uint16{0xD801, 0xDC28}},               // a supplementary letter lowers as one code point
		{"\xED\xA0\x81\xED\xB0\x80", []uint16{0xD801, 0xDC28}}, // so do its encoded halves
		{"İ", []uint16{'i', 0x0307}},
		{"\xFF", []uint16{0xFFFD}},
	}
	for _, tc := range cases {
		if got := UTF16Units(UTF16LowerKey(tc.in)); !slices.Equal(got, tc.units) {
			t.Errorf("UTF16LowerKey(%q) units = %04X, want %04X", tc.in, got, tc.units)
		}
	}
}

func TestUTF16LowerKeyMatchesUnitSequences(t *testing.T) {
	cases := []struct {
		text, search string
		want         bool
	}{
		{"a\xED\xA0\xBDb", "\xED\xA0\xBD", true},
		{"a\xED\xA0\xBDb", "\xED\xA0\xBE", false},
		{"a\xED\xA0\xBDb", "���", false},
		{"x\U0001F600y", "\xED\xA0\xBD", true},
		{"x\U0001F600y", "\xED\xB8\x80Y", true},
		{"x\U0001F600y", "\U0001F600", true},
		{"x\xED\xA0\xBD\xED\xB8\x80y", "X\U0001F600", true},
		{"\U0001F600", "\U0001F601", false},
	}
	for _, tc := range cases {
		if got := strings.Contains(UTF16LowerKey(tc.text), UTF16LowerKey(tc.search)); got != tc.want {
			t.Errorf("key of %q contains key of %q = %v, want %v", tc.text, tc.search, got, tc.want)
		}
	}
}
