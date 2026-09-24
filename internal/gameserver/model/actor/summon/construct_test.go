package summon

import (
	"testing"
	"time"

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

type peaceZoneQueryStub bool

func (q peaceZoneQueryStub) EffectRangeInPeaceZone(_, _, _, _, _, _ int) bool { return bool(q) }

func TestSummonInPeaceZoneQueriesCurrentZone(t *testing.T) {
	pet := mustPet(t, PetConfig{ObjectID: 1, Zones: peaceZoneQueryStub(true)})
	if !pet.InPeaceZone() {
		t.Fatal("InPeaceZone() = false for a summon inside a peace zone")
	}
}

// idleQueue is a queue on a virtual clock no test advances: timers armed on
// it never fire.
func idleQueue() *sim.Queue { return sim.NewInline(time.Unix(0, 0)).NewQueue("test") }
