package commons

import (
	"slices"
	"testing"
)

// Strings below spell an unpaired surrogate in its three-byte form:
// "\xED\xA0\xBD" is the unit D83D, "\xED\xB8\x80" the unit DE00.

func TestUTF16LenCountsUnits(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"й中", 2},
		{"\U0001F600", 2},
		{"\xED\xA0\xBD", 1},
		{"a\xED\xB8\x80b", 3},
		{"\xED\xA0\xBD\xED\xB8\x80", 2},
		{"\xED\xA0", 2}, // invalid bytes count one unit each
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
		{"x\xED\xA0\xBDy", []uint16{'x', 0xD83D, 'y'}},
		{"\xED\xB8\x80\xED\xA0\xBD", []uint16{0xDE00, 0xD83D}},
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
	// Two halves spelled apart join into the supplementary character.
	if got := UTF16String(UTF16Units("\xED\xA0\xBD\xED\xB8\x80")); got != "\U0001F600" {
		t.Errorf("encoded pair = %q, want U+1F600", got)
	}
}

func TestUTF16RunesKeepsUnpairedSurrogates(t *testing.T) {
	got := UTF16Runes("a\xED\xA0\xBD\U0001F600\xED\xB8\x80")
	if want := []rune{'a', 0xD83D, 0x1F600, 0xDE00}; !slices.Equal(got, want) {
		t.Fatalf("UTF16Runes = %X, want %X", got, want)
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
		{"a\xED\xA0\xBDb", 2, "a\xED\xA0\xBD"},
		{"a\xED\xA0\xBDb", 1, "a"},
		// A cut through a pair keeps its leading surrogate on its own.
		{"a\U0001F600", 2, "a\xED\xA0\xBD"},
		{"a\U0001F600", 3, "a\U0001F600"},
		{"a\U0001F600", 1, "a"},
	}
	for _, tc := range cases {
		if got := UTF16Cut(tc.in, tc.n); got != tc.want {
			t.Errorf("UTF16Cut(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}
