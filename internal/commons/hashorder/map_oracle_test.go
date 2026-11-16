package hashorder

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// TestSortMapMatchesReferenceProbe replays testdata/map_order.golden, the
// MapProbe lines of testdata/oracle/run.sh: plain hash tables keyed by id
// ("int") or by text ("str"), filled in the order given, then listed in
// their iteration order. The lines cross every doubling up to 1024 buckets,
// hold ids with high halves and signs, text keys hashing alike (one bucket,
// insertion order), non-ASCII and surrogate-pair keys.
func TestSortMapMatchesReferenceProbe(t *testing.T) {
	data, err := os.ReadFile("testdata/map_order.golden")
	if err != nil {
		t.Fatal(err)
	}
	ints, texts := 0, 0
	for i, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		kind, rest, _ := strings.Cut(line, " ")
		in, out, ok := strings.Cut(rest, " => ")
		if !ok {
			t.Fatalf("malformed golden line %d %q", i+1, line)
		}
		switch kind {
		case "int":
			got := parseIDs(t, in)
			SortMap(got, func(id int32) int32 { return id })
			if want := parseIDs(t, out); !slices.Equal(got, want) {
				t.Errorf("line %d: order of %d ids = %v, want %v", i+1, len(got), got, want)
			}
			ints++
		case "str":
			got := strings.Split(in, ",")
			SortMap(got, TextHash)
			if want := strings.Split(out, ","); !slices.Equal(got, want) {
				t.Errorf("line %d: order of %d keys = %q, want %q", i+1, len(got), got, want)
			}
			texts++
		default:
			t.Fatalf("malformed golden line %d %q", i+1, line)
		}
	}
	if ints == 0 || texts == 0 {
		t.Fatal("golden is missing a section")
	}
}
