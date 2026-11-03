package armorset

import (
	"slices"
	"testing"
)

// TestInTableOrder lists the sets as a hash table keyed by chest id lists
// them: thirteen chests grow the table to 32 slots, so 48 and 16 share slot
// 16 behind every lower slot, chests sharing a slot keep their load order,
// and 65536 spreads its high half into slot 1. A repeated chest keeps its
// first place and takes the later set.
func TestInTableOrder(t *testing.T) {
	chests := []int32{48, 16, 33, 1, 65, 65536, 3, 4, 5, 6, 7, 8, 9}
	var sets []Set
	for _, c := range chests {
		sets = append(sets, Set{Name: "first", Chest: c})
	}
	sets = append(sets, Set{Name: "second", Chest: 33})
	tbl := NewTable(sets)

	var got []int32
	for _, s := range tbl.InTableOrder() {
		got = append(got, s.Chest)
		if want := map[bool]string{true: "second", false: "first"}[s.Chest == 33]; s.Name != want {
			t.Fatalf("set %d name = %q, want %q", s.Chest, s.Name, want)
		}
	}
	want := []int32{33, 1, 65, 65536, 3, 4, 5, 6, 7, 8, 9, 48, 16}
	if !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if tbl.Len() != len(chests) {
		t.Fatalf("Len = %d, want %d", tbl.Len(), len(chests))
	}
}
