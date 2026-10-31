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
// table through 32, 64, 128 and 256 buckets; "ops +id,-id => order" plays
// wars declared and ended after the boot, with crowded buckets that grow the
// table early or turn into trees; "sub types => order" every subset of the
// seven sub-unit types.
func TestRegistryOrderMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/order.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	wars, subs, ops, trees := 0, 0, 0, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for line := 1; sc.Scan(); line++ {
		kind, rest, _ := strings.Cut(sc.Text(), " ")
		in, out, ok := strings.Cut(rest, " => ")
		if !ok {
			t.Fatalf("malformed golden line %d %q", line, sc.Text())
		}
		if kind == "ops" {
			replayOps(t, line, in, out, &trees)
			ops++
			continue
		}
		ids, want := parseIDs(t, in), parseIDs(t, out)
		switch kind {
		case "war":
			var set warSet
			for _, id := range ids {
				set.add(id)
			}
			if got := set.order(); !slices.Equal(got, want) {
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
	if wars == 0 || subs == 0 || ops == 0 {
		t.Fatal("golden is missing a section")
	}
	if trees == 0 {
		t.Fatal("golden never crowds a bucket into a tree")
	}
}

// opsBase is the id the "ops" golden lines count their offsets from.
const opsBase = 0x10000000

// replayOps plays one "ops" golden line, "+offset" declaring a war and
// "-offset" ending one, against a warSet and compares its listing.
func replayOps(t *testing.T, line int, ops, order string, trees *int) {
	t.Helper()
	var set warSet
	for _, op := range strings.Split(ops, ",") {
		n, err := strconv.ParseInt(op[1:], 10, 32)
		if err != nil || (op[0] != '+' && op[0] != '-') {
			t.Fatalf("malformed op %q on golden line %d", op, line)
		}
		if id := int32(opsBase + n); op[0] == '+' {
			set.add(id)
		} else {
			set.remove(id)
		}
	}
	var want []int32
	if order != "" {
		for _, id := range parseIDs(t, order) {
			want = append(want, opsBase+id)
		}
	}
	if got := set.order(); !slices.Equal(got, want) {
		t.Errorf("line %d: order after %s = %v, want %v", line, ops, got, want)
	}
	if slices.ContainsFunc(set.table, func(b warBin) bool { return b.tree }) {
		*trees++
	}
}
