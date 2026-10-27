package clan

import (
	"bufio"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

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

// TestRegistryOrderMatchesReferenceProbe replays testdata/order.golden, the
// output of testdata/oracle/run.sh: the reference's own war set and sub-pledge
// registry (hash-ordered concurrent collections), filled the way a boot
// restores them (ascending ids), then listed in their iteration order.
// "war ids => order" covers sets from one clan to a hundred, which grow the
// table through 32, 64, 128 and 256 buckets; "sub types => order" every
// subset of the seven sub-unit types.
func TestRegistryOrderMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/order.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	wars, subs := 0, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for line := 1; sc.Scan(); line++ {
		kind, rest, _ := strings.Cut(sc.Text(), " ")
		in, out, ok := strings.Cut(rest, " => ")
		if !ok {
			t.Fatalf("malformed golden line %d %q", line, sc.Text())
		}
		ids, want := parseIDs(t, in), parseIDs(t, out)
		switch kind {
		case "war":
			set := map[int32]struct{}{}
			for _, id := range ids {
				set[id] = struct{}{}
			}
			if got := hashSetOrder(set); !slices.Equal(got, want) {
				t.Errorf("line %d: war order of %d clans = %v, want %v", line, len(ids), got, want)
			}
			wars++
		case "sub":
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
		default:
			t.Fatalf("malformed golden line %d %q", line, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if wars == 0 || subs == 0 {
		t.Fatal("golden is missing a section")
	}
}
