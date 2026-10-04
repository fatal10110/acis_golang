package summon

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

func mustServitor(t testing.TB, cfg ServitorConfig) *Actor {
	t.Helper()
	a, err := NewServitor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.SetQueue(idleQueue())
	return a
}

func mustPet(t testing.TB, cfg PetConfig) *Actor {
	t.Helper()
	a, err := NewPet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.SetQueue(idleQueue())
	return a
}

// TestSummonInPeaceZoneReadsItsZones: a summon is in peace while its zones
// hold it there.
func TestSummonInPeaceZoneReadsItsZones(t *testing.T) {
	form, err := zone.NewCuboid(-100, 100, -100, 100, -100, 100)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, form))
	pet := mustPet(t, PetConfig{ObjectID: 1, Zones: zones})
	if pet.InPeaceZone() {
		t.Fatal("InPeaceZone() = true before the summon entered its zones")
	}
	pet.EnterZones()
	if !pet.InPeaceZone() {
		t.Fatal("InPeaceZone() = false for a summon inside a peace zone")
	}
}

// idleQueue is a queue on a virtual clock no test advances: timers armed on
// it never fire.
func idleQueue() *sim.Queue { return sim.NewInline(time.Unix(0, 0)).NewQueue("test") }
