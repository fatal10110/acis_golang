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
// table early or turn into trees, and tree buckets that lose ids and turn
// back into chains or stay trees; "sub types => order" every subset of the
// seven sub-unit types.
func TestRegistryOrderMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/order.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	wars, subs, ops := 0, 0, 0
	var trees treeCounts
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
	if trees.lines == 0 {
		t.Fatal("golden never crowds a bucket into a tree")
	}
	if trees.chained == 0 || trees.kept == 0 || trees.addedAfterKept == 0 {
		t.Fatalf("golden misses a removal from a tree bucket: %+v", trees)
	}
}

// treeCounts tallies what the "ops" lines do to tree buckets: lines ending
// with one, removals that turn one back into a chain or leave it a tree, and
// ids added to a tree bucket that has lost an id before.
type treeCounts struct {
	lines, chained, kept, addedAfterKept int
}

// opsBase is the id the "ops" golden lines count their offsets from.
const opsBase = 0x10000000

// replayOps plays one "ops" golden line, "+offset" declaring a war and
// "-offset" ending one, against a warSet and compares its listing.
func replayOps(t *testing.T, line int, ops, order string, trees *treeCounts) {
	t.Helper()
	var set warSet
	shrunk := map[int32]bool{} // ids of tree buckets that have lost an id
	for _, op := range strings.Split(ops, ",") {
		n, err := strconv.ParseInt(op[1:], 10, 32)
		if err != nil || (op[0] != '+' && op[0] != '-') {
			t.Fatalf("malformed op %q on golden line %d", op, line)
		}
		id := int32(opsBase + n)
		if op[0] == '+' {
			if set.table != nil && !set.has(id) {
				if bin := set.table[set.index(id)]; bin.root != nil && slices.ContainsFunc(bin.ids, func(m int32) bool { return shrunk[m] }) {
					trees.addedAfterKept++
				}
			}
			set.add(id)
			continue
		}
		wasTree := set.has(id) && set.table[set.index(id)].root != nil
		set.remove(id)
		if !wasTree {
			continue
		}
		if bin := set.table[set.index(id)]; bin.root != nil {
			trees.kept++
			for _, m := range bin.ids {
				shrunk[m] = true
			}
		} else {
			trees.chained++
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
	if slices.ContainsFunc(set.table, func(b warBin) bool { return b.root != nil }) {
		trees.lines++
	}
}
