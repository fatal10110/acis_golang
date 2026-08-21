package task

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type regenActor struct {
	world.Presence

	id    int32
	ticks int
}

func (a *regenActor) ObjectID() int32 { return a.id }

func (a *regenActor) TickRegen() { a.ticks++ }

func TestNPCRegenTicksRegisteredActors(t *testing.T) {
	state := world.New()
	actor := &regenActor{id: 1}
	state.Spawn(actor, 0, 0, 0, 0)

	r := NewNPCRegen(state)
	r.Tick()

	if actor.ticks != 1 {
		t.Fatalf("actor.ticks = %d, want 1", actor.ticks)
	}
}

func TestNPCRegenTickNilStateIsNoop(t *testing.T) {
	r := NewNPCRegen(nil)
	r.Tick()
}
