package world

import (
	"slices"
	"testing"
)

// TestAppendAroundListsThePointsBlock pins AppendAround: the objects of the
// 3x3 block around the region holding the point, the excepted one left out,
// whatever region that object stands in itself; nothing off the grid.
func TestAppendAroundListsThePointsBlock(t *testing.T) {
	s := New()
	x, y := regionCenter(40, 40)
	mover := &regionTestObject{id: 1}
	s.Spawn(mover, x+3*regionSize, y, 0, 0)
	near := &regionTestObject{id: 2}
	s.Spawn(near, x+regionSize, y-regionSize, 0, 0)
	far := &regionTestObject{id: 3}
	s.Spawn(far, x+2*regionSize, y, 0, 0)
	here := &regionTestObject{id: 4}
	s.Spawn(here, x, y, 0, 0)

	var ids []int32
	for _, o := range s.AppendAround(nil, x, y, mover.ObjectID()) {
		ids = append(ids, o.ObjectID())
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []int32{2, 4}) {
		t.Fatalf("around the point = %v, want [2 4]", ids)
	}

	if got := s.AppendAround(nil, x, y, here.ObjectID()); len(got) != 1 || got[0].ObjectID() != 2 {
		t.Fatalf("around the point except 4 = %v, want only 2", got)
	}
	if got := s.AppendAround(nil, MaxX+1, y, 0); len(got) != 0 {
		t.Fatalf("around a point off the grid = %v, want nothing", got)
	}
}
