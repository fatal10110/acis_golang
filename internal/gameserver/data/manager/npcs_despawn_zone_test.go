package manager

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// TestDespawnAllTakesADecorationOutOfItsZones pins //unspawnall on a placed
// decoration standing in water: Npc.deleteMe runs decayMe, which leaves the
// zones before the object leaves the world, so the zone has no occupant
// left and stops treating the tree as inside it.
func TestDespawnAllTakesADecorationOutOfItsZones(t *testing.T) {
	f := newDespawnFixture(t, nil)
	form, err := zone.NewCuboid(0, 200, 0, 200, -100, 100)
	if err != nil {
		t.Fatal(err)
	}
	water := zone.NewWater(1, form)
	ix := zone.NewIndex()
	ix.Add(water)

	tree, _ := f.templates.Get(3)
	inst, _ := npc.NewInstance(1000, tree)
	decoration, err := npc.NewDecoration(inst, "")
	if err != nil {
		t.Fatalf("NewDecoration() error: %v", err)
	}
	decoration.Attach(npc.DecorationRuntime{World: f.state, Zones: ix})
	decoration.Spawn(80, 80, 0, 0)
	if got := len(water.Core().Occupants()); got != 1 {
		t.Fatalf("water occupants after placing the tree = %d, want 1", got)
	}

	f.npcs.DespawnAll()
	f.queues.Run()

	if got := len(water.Core().Occupants()); got != 0 {
		t.Fatalf("water occupants after //unspawnall = %d, want 0", got)
	}
	if decoration.InsideZone(zone.FlagWater) {
		t.Fatal("deleted decoration still reports itself in water")
	}
	if got := len(f.npcObjects()); got != 0 {
		t.Fatalf("npcs left in the world: %d", got)
	}
}
