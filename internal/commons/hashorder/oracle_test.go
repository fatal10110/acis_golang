package hashorder

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

// TestOrderMatchesReferenceProbe replays testdata/order.golden, the output
// of testdata/oracle/run.sh: the reference's own hash-ordered concurrent set
// (the one a clan's war set is), filled the way a boot restores it (ascending ids), then
// listed in its iteration order. "war ids => order" covers sets from one id
// to a hundred, which grow the table through 32, 64, 128 and 256 buckets;
// "ops +id,-id => order" plays ids added and removed after the boot, with
// crowded buckets that grow the table early or turn into trees, and tree
// buckets that lose ids and turn back into chains or stay trees.
func TestOrderMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/order.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	wars, ops := 0, 0
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
			var set Set
			for _, id := range ids {
				set.Add(id)
			}
			if got := set.Order(); !slices.Equal(got, want) {
				t.Errorf("line %d: order of %d ids = %v, want %v", line, len(ids), got, want)
			}
			wars++
		default:
			t.Fatalf("malformed golden line %d %q", line, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if wars == 0 || ops == 0 {
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

// replayOps plays one "ops" golden line, "+offset" adding an id and
// "-offset" removing one, against a Set and compares its listing.
func replayOps(t *testing.T, line int, ops, order string, trees *treeCounts) {
	t.Helper()
	var set Set
	shrunk := map[int32]bool{} // ids of tree buckets that have lost an id
	for _, op := range strings.Split(ops, ",") {
		n, err := strconv.ParseInt(op[1:], 10, 32)
		if err != nil || (op[0] != '+' && op[0] != '-') {
			t.Fatalf("malformed op %q on golden line %d", op, line)
		}
		id := int32(opsBase + n)
		if op[0] == '+' {
			if set.table != nil && !set.Has(id) {
				if bin := set.table[set.index(id)]; bin.root != nil && slices.ContainsFunc(bin.ids, func(m int32) bool { return shrunk[m] }) {
					trees.addedAfterKept++
				}
			}
			set.Add(id)
			continue
		}
		wasTree := set.Has(id) && set.table[set.index(id)].root != nil
		set.Remove(id)
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
	if got := set.Order(); !slices.Equal(got, want) {
		t.Errorf("line %d: order after %s = %v, want %v", line, ops, got, want)
	}
	if slices.ContainsFunc(set.table, func(b bucket) bool { return b.root != nil }) {
		trees.lines++
	}
}
