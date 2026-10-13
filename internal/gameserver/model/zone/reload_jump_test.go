package zone

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestReloadAfterAJumpKeepsTheRegionHoldingTheCreature pins a zone reload
// after an instant teleport into another region: the reload revalidates
// the creature against the reloaded zones of the region still holding it,
// so it leaves the zone it jumped out of and enters none at its landing
// point; its next step moves it to the region of its position and enters
// them.
func TestReloadAfterAJumpKeepsTheRegionHoldingTheCreature(t *testing.T) {
	edge := world.MinX + 100*regionEdge
	layout := func() (*Index, *Arena, *Peace) {
		ix := NewIndex()
		arena := NewArena(1, testCuboid(t, edge-1000, edge-1, 0, 1000))
		peace := NewPeace(2, testCuboid(t, edge+3000, edge+4000, 0, 1000))
		ix.Add(arena)
		ix.Add(peace)
		return ix, arena, peace
	}
	ix, _, _ := layout()
	s := &memberStub{id: 1, class: ClassNPC, at: location.Location{X: edge - 500, Y: 500}}
	s.Enter(ix, s)
	s.at = location.Location{X: edge + 3500, Y: 500}

	next, arena, peace := layout()
	old := ix.Swap(next)
	s.Reload(ix, s)
	old.Clear()
	flags := s.ZoneFlags()
	if flags.Has(FlagPeace) || peace.Inside(s) {
		t.Fatal("the reload entered the zone at the landing point, in a region not holding the creature")
	}
	if flags.Has(FlagNoSummonFriend) || arena.Inside(s) {
		t.Fatal("the reload kept the creature in the zone it jumped out of")
	}

	s.at.X++
	s.Step(ix, s)
	if !flags.Has(FlagPeace) || !peace.Inside(s) {
		t.Fatal("the first step after the reload did not enter the landing point's zone")
	}
}
