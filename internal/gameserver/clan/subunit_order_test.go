package clan

import (
	"bufio"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestSubunitOrderMatchesReferenceProbe replays testdata/subunit_order.golden,
// written by internal/commons/hashorder/testdata/oracle/run.sh: the
// reference's own sub-pledge registry (a hash-ordered concurrent map) filled
// with every subset of the seven sub-unit types, listed in its iteration
// order.
func TestSubunitOrderMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/subunit_order.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	subs := 0
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		kind, rest, _ := strings.Cut(sc.Text(), " ")
		in, out, ok := strings.Cut(rest, " => ")
		if kind != "sub" || !ok {
			t.Fatalf("malformed golden line %d %q", line, sc.Text())
		}
		ids, want := parseIDs(t, in), parseIDs(t, out)
		cl := newClan(1, "Knights", 1)
		for _, id := range ids {
			cl.subunits[int(id)] = &SubPledge{ID: int(id)}
		}
		var got []int32
		for _, sp := range cl.Subunits() {
			got = append(got, int32(sp.ID))
		}
		if !slices.Equal(got, want) {
			t.Errorf("line %d: sub-unit order of %v = %v, want %v", line, ids, got, want)
		}
		subs++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if subs == 0 {
		t.Fatal("golden is empty")
	}
}

func parseIDs(t *testing.T, s string) []int32 {
	t.Helper()
	var out []int32
	for _, f := range strings.Split(s, ",") {
		n, err := strconv.ParseInt(f, 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, int32(n))
	}
	return out
}
