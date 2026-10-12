package zone

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// swimmingStub is a member that records its swim state changes.
type swimmingStub struct {
	memberStub
	swims []bool
}

func (s *swimmingStub) SwimStateChanged(swimming bool) { s.swims = append(s.swims, swimming) }

// TestReloadMovesMembersWithoutExitRules pins a zone reload's move of one
// creature: Swap hands every holder the new zones; Reload drops the
// creature from the old zones it stood in without their exit rules, clears
// what they raised and enters the new zones around it; a creature that only
// the old zones kept in water is told it left the water; one mid-teleport
// enters nothing until it lands; Clear empties the old zones of whatever
// the reload did not reach.
func TestReloadMovesMembersWithoutExitRules(t *testing.T) {
	ix := NewIndex()
	field := NewSiege(1, testCuboid(t, 0, 1000, 0, 1000))
	field.SetActive(true)
	var exits int
	field.Unsummon = func(Actor) { exits++ }
	water := NewWater(2, testCuboid(t, 0, 1000, 0, 1000))
	ix.Add(field)
	ix.Add(water)

	summon := &swimmingStub{memberStub: memberStub{id: 1, class: ClassSummon, at: location.Location{X: 500, Y: 500}}}
	summon.Enter(ix, summon)
	jumper := &memberStub{id: 2, class: ClassNPC, at: location.Location{X: 500, Y: 500}}
	jumper.Enter(ix, jumper)
	stray := &memberStub{id: 3, class: ClassNPC, at: location.Location{X: 500, Y: 500}}
	stray.Enter(ix, stray)
	if !summon.ZoneFlags().Has(FlagSiege) || !summon.ZoneFlags().Has(FlagWater) {
		t.Fatal("the summon did not enter the battlefield and the water")
	}

	next := NewIndex()
	peace := NewPeace(3, testCuboid(t, 0, 1000, 0, 1000))
	next.Add(peace)
	old := ix.Swap(next)
	if got := ix.All(); len(got) != 1 || got[0] != peace {
		t.Fatalf("after Swap the index holds %v, want the reloaded peace zone", got)
	}

	summon.Reload(ix, summon)
	if exits != 0 {
		t.Fatal("the reload ran the battlefield's exit rule")
	}
	flags := summon.ZoneFlags()
	if flags.Has(FlagSiege) || flags.Has(FlagPvP) || flags.Has(FlagWater) || !flags.Has(FlagPeace) {
		t.Fatal("after the reload the summon keeps the old zones' flags or misses the peace")
	}
	if len(summon.swims) != 2 || summon.swims[1] {
		t.Fatalf("swim changes = %v, want entering then leaving the water", summon.swims)
	}
	if field.Inside(summon) || water.Inside(summon) || !peace.Inside(summon) {
		t.Fatal("the summon was not moved from the old zones to the new one")
	}

	jumper.teleporting = true
	jumper.Reload(ix, jumper)
	if jumper.ZoneFlags().Has(FlagPeace) || peace.Inside(jumper) || jumper.ZoneFlags().Has(FlagSiege) {
		t.Fatal("a creature mid-teleport entered the reloaded zones or kept the old flags")
	}

	old.Clear()
	if field.Inside(stray) || water.Inside(stray) || exits != 0 {
		t.Fatal("Clear left the stray in the old zones or ran their exit rules")
	}
}
