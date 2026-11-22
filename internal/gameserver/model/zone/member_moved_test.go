package zone

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestMemberMovedAndRemove pins the position update that revalidates at
// once (Moved): within the holding region against that region's zones,
// across regions against both, restarting the step count either way; and
// Remove, after which no Enter or position report puts the creature back.
// Every report says whether the creature was on the grid.
func TestMemberMovedAndRemove(t *testing.T) {
	ix := NewIndex()
	ix.Add(NewPeace(1, testCuboid(t, 0, 1000, 0, 1000)))
	far := world.MinX + 10
	tw := NewTown(2, testCuboid(t, far, far+1000, 0, 1000))
	tw.Peaceful = false
	ix.Add(tw)
	s := &memberStub{id: 1, class: ClassNPC, at: location.Location{X: 2000, Y: 500}}
	peace := func() bool { return s.ZoneFlags().Has(FlagPeace) }
	town := func() bool { return s.ZoneFlags().Has(FlagTown) }

	if s.Moved(ix, s) || s.Step(ix, s) || s.Settle(ix, s) || s.Place(ix, s) {
		t.Fatal("before Enter: a position report said the creature is on the grid")
	}
	if !s.Enter(ix, s) {
		t.Fatal("Enter refused a creature never removed")
	}
	s.at = location.Location{X: 500, Y: 500}
	if !s.Moved(ix, s) || !peace() {
		t.Fatal("moved into the zone in the same region: not entered at once")
	}
	for i := 1; i <= 4; i++ {
		s.at = location.Location{X: 2000 + i, Y: 500}
		s.Step(ix, s)
	}
	s.at = location.Location{X: 500, Y: 500}
	s.Moved(ix, s)
	s.at = location.Location{X: 2000, Y: 500}
	s.Step(ix, s)
	if !peace() {
		t.Fatal("Moved did not restart the step count: the next step revalidated")
	}

	s.at = location.Location{X: far + 500, Y: 500}
	s.Moved(ix, s)
	if peace() || !town() {
		t.Fatalf("moved across regions into the town: peace %v, town %v", peace(), town())
	}

	s.Remove(ix, s)
	if town() {
		t.Fatal("removed: still in the town")
	}
	if s.Enter(ix, s) || town() {
		t.Fatal("Enter after Remove put the creature back on the grid")
	}
	if s.Moved(ix, s) || town() {
		t.Fatal("a position report after Remove entered a zone")
	}
	if s.Reload(ix, s) || town() {
		t.Fatal("a reload after Remove entered a zone")
	}
}
