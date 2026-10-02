package pathfind

import (
	"math"
	"testing"
)

func TestCellKeyDistinguishesNeighbours(t *testing.T) {
	seen := make(map[uint64][3]int)
	for _, gx := range []int{0, 1, 2047, 2048, 22527} {
		for _, gy := range []int{0, 1, 2047, 32767} {
			for _, z := range []int{math.MinInt16, -16, -1, 0, 1, 16, math.MaxInt16} {
				key := cellKey(gx, gy, z)
				if prev, ok := seen[key]; ok {
					t.Fatalf("cellKey(%d, %d, %d) = cellKey%v = %#x", gx, gy, z, prev, key)
				}
				seen[key] = [3]int{gx, gy, z}
			}
		}
	}
}

func TestNodeSetGrowsAndForgetsPreviousSearch(t *testing.T) {
	var s nodeSet
	s.reset()
	const cells = 3 << minSetBits // forces two doublings
	for i := range cells {
		s.slot(cellKey(i, i/7, i%13)).fwd = int32(i + 1)
	}
	for i := range cells {
		slot := s.lookup(cellKey(i, i/7, i%13))
		if slot == nil || slot.fwd != int32(i+1) || slot.back != 0 {
			t.Fatalf("cell %d after growth = %+v, want fwd %d", i, slot, i+1)
		}
	}
	if s.lookup(cellKey(cells, 0, 0)) != nil {
		t.Fatal("lookup() found a cell never added")
	}

	s.reset()
	for i := range cells {
		if slot := s.lookup(cellKey(i, i/7, i%13)); slot != nil {
			t.Fatalf("cell %d survived reset: %+v", i, slot)
		}
	}
	if slot := s.slot(cellKey(5, 0, 5)); slot.fwd != 0 || slot.back != 0 {
		t.Fatalf("slot() after reset = %+v, want an empty slot", slot)
	}
}

func TestNodeSetGenerationWrapClearsSlots(t *testing.T) {
	var s nodeSet
	s.reset()
	s.slot(cellKey(1, 2, 3)).back = 9
	s.gen = math.MaxUint32
	s.slots[s.home(cellKey(1, 2, 3))].gen = 1 // a slot left by the generation reset is about to reuse
	s.reset()
	if s.gen != 1 {
		t.Fatalf("generation after wrap = %d, want 1", s.gen)
	}
	if slot := s.lookup(cellKey(1, 2, 3)); slot != nil {
		t.Fatalf("slot from before the wrap = %+v, want none", slot)
	}
}
