package commons

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestUTF16LowerKeyFinalSigmaCases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ΑΣ", "ας"},
		{"Σ", "σ"},     // nothing cased before it
		{"ΑΣΑ", "ασα"}, // a cased letter follows in the word
		{"ΟΔΟΣ ΟΔΟΣ", "οδος οδος"},     // each word ends with one
		{"ΑΣ.", "ας."},                 // a trailing period is not in the word
		{"Α.Σ", "α.ς"},                 // a single mid-word mark joins letters
		{"Α..Σ", "α..σ"},               // two do not
		{"Α_Σ", "α_ς"},                 // connector punctuation is a mid-word mark
		{"ΑΣ1", "ας1"},                 // a trailing number joins the word, uncased
		{"ΑΣ1Β", "ασ1β"},               // and a word after it too
		{"a.1Σ", "a.1σ"},               // ".1Σ" is its own number-led segment
		{"ΑΣ'Β", "ασ'β"},               // the apostrophe joins letters
		{"ΑΣ\u00ADΒ", "ασ\u00ADβ"},     // so does the soft hyphen
		{"\u0345Σ", "\u0345σ"},         // a leading combining mark stands alone
		{"Α\u0301Σ", "α\u0301ς"},       // a combining mark stays with its letter
		{"ΑΣ\u05D0", "ας\u05D0"},       // an uncased letter after it
		{"Α\u05D0Σ", "α\u05D0ς"},       // and before it, after a cased one
		{"\u05D0Σ", "\u05D0σ"},         // only uncased letters before it
		{"中Σ", "中σ"},                   // an ideograph is not in the word
		{"ΑΣ\uFFFFΒ", "ας\uFFFFβ"},     // U+FFFF ends the scan
		{"\U00010400Σ", "\U00010428ς"}, // a supplementary letter before it
		// With a character before the pair, the boundary test before the
		// sigma backs up from the pair's low half and settles on the sigma,
		// so the sigma starts a word.
		{"x\U00010400Σ", "x\U00010428σ"},
	}
	for _, tc := range cases {
		if got := UTF16Units(UTF16LowerKey(tc.in)); !slices.Equal(got, UTF16Units(tc.want)) {
			t.Errorf("UTF16LowerKey(%+q) units = %04X, want those of %+q", tc.in, got, tc.want)
		}
	}
}

// TestUTF16LowerKeyFinalSigma replays testdata/final_sigma.golden, each row
// an input and its lower case as UTF-16 units in hex, captured from the
// reference server's String.toLowerCase() on JDK 21 (en_US) by
// testdata/oracle/FinalSigmaProbe.java. Inputs mix capital sigmas with
// cased and uncased letters, digits, mid-word and mid-number marks, number
// signs, combining and format marks, spaces, kana, ideographs, Thai,
// dandas, surrogate pairs, lone surrogates, and U+FFFF.
func TestUTF16LowerKeyFinalSigma(t *testing.T) {
	rows, fails := 0, 0
	for _, f := range goldenFields(t, "testdata/final_sigma.golden", "\t") {
		rows++
		if got := UTF16Units(UTF16LowerKey(UTF16String(parseUnits(t, f[0])))); !slices.Equal(got, parseUnits(t, f[1])) {
			if fails++; fails <= 20 {
				t.Errorf("UTF16LowerKey(%s) units = %04X, want %s", f[0], got, f[1])
			}
		}
	}
	if rows == 0 || fails > 0 {
		t.Fatalf("%d of %d rows differ", fails, rows)
	}
}

// TestIsCasedMatchesOracle compares isCased with the reference's casing
// test on every code point: testdata/cased.golden lists the ranges it
// accepts.
func TestIsCasedMatchesOracle(t *testing.T) {
	want := make([]bool, 0x110000)
	for _, f := range goldenFields(t, "testdata/cased.golden", "-") {
		first, err1 := strconv.ParseUint(f[0], 16, 32)
		last, err2 := strconv.ParseUint(f[1], 16, 32)
		if err1 != nil || err2 != nil {
			t.Fatalf("malformed range %v", f)
		}
		for r := first; r <= last; r++ {
			want[r] = true
		}
	}
	fails := 0
	for r := range rune(len(want)) {
		if isCased(r) != want[r] {
			if fails++; fails <= 20 {
				t.Errorf("isCased(%U) = %v, want %v", r, !want[r], want[r])
			}
		}
	}
}

// TestUTF16LowerKeyLongSigmaRun lowers a run of 16,000 capital sigmas, the
// first with nothing cased before it and the last with nothing after it.
// Testing each sigma's boundaries afresh would rescan the run per sigma;
// shared scan results keep the whole key linear.
func TestUTF16LowerKeyLongSigmaRun(t *testing.T) {
	const n = 16000
	want := strings.Repeat("σ", n-1) + "ς"
	if got := UTF16LowerKey(strings.Repeat("Σ", n)); got != want {
		t.Fatalf("UTF16LowerKey of %d sigmas differs from %d σ then ς", n, n-1)
	}
}

func goldenFields(t *testing.T, path, sep string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows [][]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		a, b, ok := strings.Cut(sc.Text(), sep)
		if !ok {
			t.Fatalf("%s: malformed row %q", path, sc.Text())
		}
		rows = append(rows, []string{a, b})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return rows
}

func parseUnits(t *testing.T, hex string) []uint16 {
	t.Helper()
	units := []uint16{}
	for _, f := range strings.Fields(hex) {
		u, err := strconv.ParseUint(f, 16, 16)
		if err != nil {
			t.Fatal(fmt.Errorf("unit %q: %w", f, err))
		}
		units = append(units, uint16(u))
	}
	return units
}
