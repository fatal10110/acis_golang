package world

import (
	"slices"
	"testing"
)

// SetPosition moves the object's coordinates but not its region: nobody
// forgets or discovers it, its known list stays the old region's
// neighborhood, and the next Move puts it in the region of its position,
// exchanging the callbacks between the two neighborhoods then.
func TestSetPositionKeepsTheRegionUntilTheNextMove(t *testing.T) {
	s := New()
	log := &teleportLog{}
	for id, col := range map[int32]int{1: 49, 3: 53} {
		x, y := teleportRegionAt(col)
		s.Spawn(&teleportObserver{id: id, log: log}, x, y, 0, 0)
	}
	subject := &regionTestObject{id: 99}
	x, y := teleportRegionAt(50)
	s.Spawn(subject, x, y, 0, 0)
	log.take()
	old := subject.presence().region.Load()

	nx, ny := teleportRegionAt(53)
	s.SetPosition(subject, nx, ny, 7)
	if got := log.take(); len(got) != 0 {
		t.Fatalf("callbacks after SetPosition = %q, want none", got)
	}
	if px, py, pz := subject.Position(); px != nx || py != ny || pz != 7 {
		t.Fatalf("Position() = (%d,%d,%d), want (%d,%d,7)", px, py, pz, nx, ny)
	}
	if subject.presence().region.Load() != old {
		t.Fatal("SetPosition moved the subject out of its region")
	}
	if got, want := objectIDs(s.AppendKnown(nil, subject)), []int32{1}; !sameIDs(got, want) {
		t.Fatalf("known after SetPosition = %v, want the old neighborhood %v", got, want)
	}

	if err := s.Move(subject, nx+10, ny, 7); err != nil {
		t.Fatalf("Move() error: %v", err)
	}
	if got, want := log.take(), []string{"1 forget 99", "3 discover 99"}; !slices.Equal(got, want) {
		t.Fatalf("callbacks after the next Move = %q, want %q", got, want)
	}
	if next, _ := s.RegionAt(nx, ny); subject.presence().region.Load() != next {
		t.Fatal("the next Move left the subject out of the region of its position")
	}
}

// SetPosition on an object off the grid only changes its position.
func TestSetPositionOffTheGrid(t *testing.T) {
	s := New()
	subject := &regionTestObject{id: 99}
	x, y := teleportRegionAt(50)
	s.Spawn(subject, x, y, 0, 0)
	s.Leave(subject)

	nx, ny := teleportRegionAt(53)
	s.SetPosition(subject, nx, ny, 3)
	if px, py, pz := subject.Position(); px != nx || py != ny || pz != 3 {
		t.Fatalf("Position() = (%d,%d,%d), want (%d,%d,3)", px, py, pz, nx, ny)
	}
	if subject.presence().region.Load() != nil {
		t.Fatal("SetPosition put an object off the grid back on it")
	}
}
