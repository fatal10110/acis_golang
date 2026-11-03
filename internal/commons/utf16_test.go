package commons

import (
	"slices"
	"testing"
)

func TestUTF16LenCountsUnits(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"й中", 2},
		{"\U0001F600", 2},
		{"a\U0001F600b", 4},
		{"�", 1},
		{"\xED\xA0\xBD", 3}, // invalid bytes count one unit each
		{"\xFF", 1},
	}
	for _, tc := range cases {
		if got := UTF16Len(tc.in); got != tc.want {
			t.Errorf("UTF16Len(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestUTF16UnitsAndStringRoundTrip(t *testing.T) {
	cases := []struct {
		in    string
		units []uint16
	}{
		{"", []uint16{}},
		{"aй", []uint16{'a', 0x0439}},
		{"\U0001F600", []uint16{0xD83D, 0xDE00}},
		{"x\U0001F600y", []uint16{'x', 0xD83D, 0xDE00, 'y'}},
	}
	for _, tc := range cases {
		units := UTF16Units(tc.in)
		if !slices.Equal(units, tc.units) {
			t.Errorf("UTF16Units(%q) = %04X, want %04X", tc.in, units, tc.units)
		}
		if got := UTF16String(units); got != tc.in {
			t.Errorf("UTF16String(%04X) = %q, want %q", units, got, tc.in)
		}
	}
	// Invalid bytes become U+FFFD, as the writer sends them.
	if got := UTF16Units("\xFF"); !slices.Equal(got, []uint16{0xFFFD}) {
		t.Errorf("UTF16Units(\\xFF) = %04X, want [FFFD]", got)
	}
	// An unpaired surrogate unit reads as U+FFFD.
	if got := UTF16String([]uint16{'a', 0xD83D, 'b', 0xDE00}); got != "a�b�" {
		t.Errorf("UTF16String of unpaired units = %q, want a\\uFFFDb\\uFFFD", got)
	}
}

func TestUTF16CutCutsAtUnit(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abc", 2, "ab"},
		{"abc", 0, ""},
		// A cut through a pair leaves its leading unit, as U+FFFD.
		{"a\U0001F600", 2, "a�"},
		{"a\U0001F600", 3, "a\U0001F600"},
		{"a\U0001F600", 1, "a"},
	}
	for _, tc := range cases {
		got := UTF16Cut(tc.in, tc.n)
		if got != tc.want {
			t.Errorf("UTF16Cut(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
		if units := UTF16Units(tc.in); got != UTF16String(units[:min(tc.n, len(units))]) {
			t.Errorf("UTF16Cut(%q, %d) differs from cutting its units", tc.in, tc.n)
		}
	}
}
