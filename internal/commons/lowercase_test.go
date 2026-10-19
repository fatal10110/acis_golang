package commons

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestLowerRuneMatchesOracle compares lowerRune with the reference's
// Character.toLowerCase(int) on JDK 21 (Unicode 15.0) on every code point:
// testdata/lower_case.golden, captured by testdata/oracle/LowerCaseProbe.java,
// lists each code point the mapping changes and its lower case.
func TestLowerRuneMatchesOracle(t *testing.T) {
	want := make([]rune, 0x110000)
	for r := range want {
		want[r] = rune(r)
	}
	rows := 0
	for _, f := range goldenFields(t, "testdata/lower_case.golden", " ") {
		from, err1 := strconv.ParseUint(f[0], 16, 32)
		to, err2 := strconv.ParseUint(f[1], 16, 32)
		if err1 != nil || err2 != nil || from >= uint64(len(want)) {
			t.Fatalf("malformed row %v", f)
		}
		want[from] = rune(to)
		rows++
	}
	if rows == 0 {
		t.Fatal("empty oracle")
	}
	fails := 0
	for r := range rune(len(want)) {
		if got := lowerRune(r); got != want[r] {
			if fails++; fails <= 20 {
				t.Errorf("lowerRune(%U) = %U, want %U", r, got, want[r])
			}
		}
	}
	for _, r := range []rune{-1, 0x110000, 0x7FFFFFFF} {
		if got := lowerRune(r); got != r {
			t.Errorf("lowerRune(%#x) = %#x, want it unchanged", r, got)
		}
	}
}

// TestUTF16LowerKeyKeepsLettersAfterUnicode15 checks that letters encoded
// after Unicode 15.0 keep their case in a key, so a search for the lower
// case a newer table gives them does not match them.
func TestUTF16LowerKeyKeepsLettersAfterUnicode15(t *testing.T) {
	for _, s := range []string{"\uA7CB", "\uA7DC", "\u1C89", "\U00010D50", "\U00016EA0"} {
		if got := UTF16Units(UTF16LowerKey(s)); !slices.Equal(got, UTF16Units(s)) {
			t.Errorf("UTF16LowerKey(%+q) units = %04X, want them unchanged", s, got)
		}
	}
	if strings.Contains(UTF16LowerKey("a\uA7CBb"), UTF16LowerKey("\u0264")) {
		t.Error("a search for U+0264 matches U+A7CB")
	}
}
