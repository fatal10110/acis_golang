package commons

import (
	"bufio"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestLegacyStringHashMatchesReferenceProbe checks every line of
// testdata/stringhash.golden, the output of
// testdata/oracle/StringHashProbe.java: fixed strings (including
// supplementary-plane runes) and every shipped multisell list name.
func TestLegacyStringHashMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/stringhash.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			t.Fatalf("malformed golden line %q", sc.Text())
		}
		want, err := strconv.ParseInt(fields[0], 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		var s []byte
		if fields[1] != "-" {
			if s, err = hex.DecodeString(fields[1]); err != nil {
				t.Fatal(err)
			}
		}
		if got := LegacyStringHash(string(s)); got != int32(want) {
			t.Errorf("LegacyStringHash(%q) = %d, want %d", s, got, want)
		}
		n++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n < 50 {
		t.Fatalf("golden has %d lines, want the fixed strings plus every multisell name", n)
	}
}
