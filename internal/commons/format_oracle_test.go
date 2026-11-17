package commons

import (
	"bufio"
	"encoding/hex"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestFormatMatchesReferenceProbe replays testdata/format.golden, the output
// of testdata/oracle/FormatProbe.java: the reference's Double.toString,
// Float.toString and String.format("%.Nf") over fixed and seeded values, and
// the placeholder replacement of NpcHtmlMessage.replace(String, String).
func TestFormatMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/format.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	counts := map[string]int{}
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		fields := strings.Split(sc.Text(), " ")
		kind := fields[0]
		counts[kind]++
		switch {
		case kind == "double" && len(fields) == 3:
			v := math.Float64frombits(parseBits(t, line, fields[1], 64))
			if got := FormatDouble(v); got != fields[2] {
				t.Errorf("line %d: FormatDouble(%#x) = %q, want %q", line, math.Float64bits(v), got, fields[2])
			}
		case kind == "float" && len(fields) == 3:
			v := math.Float32frombits(uint32(parseBits(t, line, fields[1], 32)))
			if got := FormatFloat(v); got != fields[2] {
				t.Errorf("line %d: FormatFloat(%#x) = %q, want %q", line, math.Float32bits(v), got, fields[2])
			}
		case kind == "fixed" && len(fields) == 4:
			v := math.Float64frombits(parseBits(t, line, fields[1], 64))
			d, err := strconv.Atoi(fields[2])
			if err != nil {
				t.Fatalf("line %d: %v", line, err)
			}
			if got := FormatFixed(v, d); got != fields[3] {
				t.Errorf("line %d: FormatFixed(%#x, %d) = %q, want %q", line, math.Float64bits(v), d, got, fields[3])
			}
		case kind == "html" && len(fields) == 3:
			in := unhex(t, line, fields[1])
			got := HTMLValue(in)
			if fields[2] == "EXC" {
				// The reference throws on these values and the page is not sent;
				// HTMLValue keeps the page by reading the backslash as quoting
				// the next character and a trailing one as itself.
				if !strings.Contains(in, `\`) {
					t.Errorf("line %d: %q has no backslash yet the reference rejects it", line, in)
				}
				continue
			}
			if want := unhex(t, line, fields[2]); got != want {
				t.Errorf("line %d: HTMLValue(%q) = %q, want %q", line, in, got, want)
			}
		default:
			t.Fatalf("malformed golden line %d %q", line, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"double", "float", "fixed", "html"} {
		if counts[kind] < 20 {
			t.Errorf("golden has %d %s lines", counts[kind], kind)
		}
	}
}

func parseBits(t *testing.T, line int, s string, size int) uint64 {
	t.Helper()
	v, err := strconv.ParseUint(s, 16, size)
	if err != nil {
		t.Fatalf("line %d: %v", line, err)
	}
	return v
}

func unhex(t *testing.T, line int, s string) string {
	t.Helper()
	if s == "-" {
		return ""
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("line %d: %v", line, err)
	}
	return string(b)
}
