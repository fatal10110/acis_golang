package network

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
)

// TestMemoOrderMatchesReferenceTable replays the key orders the reference
// hash table gives, captured from it (testdata/memo_order.txt: keys in
// insertion order | the table's order | the order of a copy of it). The
// rows cover two keys sharing a slot, a copy presized past the source's
// table, a resize at the twelfth key, the longest datapack <ai> block, and
// slots long enough to grow a small table and to turn into tree bins.
func TestMemoOrderMatchesReferenceTable(t *testing.T) {
	data, err := os.ReadFile("testdata/memo_order.txt")
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.Split(line, "|")
		keys := strings.Split(parts[0], ",")
		if got := strings.Join(memoOrder(keys), ","); got != parts[1] {
			t.Errorf("row %d memoOrder(%d keys) =\n %s\nwant\n %s", i, len(keys), got, parts[1])
		}
		if got := strings.Join(memoCopyOrder(keys), ","); got != parts[2] {
			t.Errorf("row %d memoCopyOrder(%d keys) =\n %s\nwant\n %s", i, len(keys), got, parts[2])
		}
	}
}

// TestMemoOrderKeepsFirstPlaceOfRepeatedKey pins a repeated key: it keeps
// the place it took first.
func TestMemoOrderKeepsFirstPlaceOfRepeatedKey(t *testing.T) {
	keys := []string{"MoveAroundSocial", "MoveAroundSocial1", "MoveAroundSocial2", "SetHateOccupation", "SetHateOccupationRatio"}
	want := memoOrder(keys)
	if got := memoOrder(append(slices.Clone(keys), "MoveAroundSocial", "SetHateOccupation")); !slices.Equal(got, want) {
		t.Fatalf("memoOrder with repeats = %v, want %v", got, want)
	}
}

// TestBuyListOrderFollowsHashSlots pins the order of an NPC's buylists, as
// the reference lists them for 35276 and 35557 among the 687 datapack
// buylists: by hash slot of a 1024-slot table, not by id.
func TestBuyListOrderFollowsHashSlots(t *testing.T) {
	for _, c := range []struct {
		ids, want []int
	}{
		{ids: []int{352760, 352761, 352762, 352763, 352764, 352765}, want: []int{352765, 352764, 352761, 352760, 352763, 352762}},
		{ids: []int{355570, 355571, 355572, 355573, 355574, 355575}, want: []int{355573, 355572, 355575, 355574, 355571, 355570}},
		{ids: []int{5720, 5721, 5722, 5723, 5724}, want: []int{5720, 5721, 5722, 5723, 5724}},
	} {
		lists := make([]buylist.List, len(c.ids))
		for i, id := range c.ids {
			lists[i] = buylist.List{ID: id}
		}
		var got []int
		for _, l := range buyListOrder(lists, 687) {
			got = append(got, l.ID)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("buyListOrder(%v) = %v, want %v", c.ids, got, c.want)
		}
	}
}
