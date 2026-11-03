package task

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestNPCRegenBackstopPostsNothingForSettledActors pins the backstop pass's
// cost to the stragglers: an actor with nothing to regenerate and an actor
// whose armed phase the sweep already walks get no task on their queue.
func TestNPCRegenBackstopPostsNothingForSettledActors(t *testing.T) {
	t.Parallel()
	loop := sim.NewInline(time.Unix(0, 0))
	q := loop.NewQueue("regen")
	full, armed := newSweptRegenActor(1, q), newSweptRegenActor(2, q)
	state := world.New()
	state.AddObject(full)
	state.AddObject(armed)
	regen := NewNPCRegen(state)
	regen.Tick() // backstop pass: binds both phases
	loop.Run()

	armed.short.Store(true)
	armed.SettleRegen()
	before := q.Accepted()
	regen.sweeps = 0 // the next sweep is a backstop pass
	regen.Tick()
	if n := q.Accepted() - before; n != 0 {
		t.Fatalf("backstop pass posted %d tasks for a full and an armed actor, want none", n)
	}
	loop.Run()
	if full.regen.Active() || !armed.regen.Active() {
		t.Fatalf("phases active = %v (full), %v (armed), want false, true", full.regen.Active(), armed.regen.Active())
	}
}

// TestNPCRegenBackstopRestoresDroppedArmedPhase covers an armed phase that
// dropped out of the armed set: the next backstop pass binds it again
// without moving it, so it ticks at its original deadline.
func TestNPCRegenBackstopRestoresDroppedArmedPhase(t *testing.T) {
	t.Parallel()
	loop := sim.NewInline(time.Unix(0, 0))
	a := newSweptRegenActor(1, loop.NewQueue("regen"))
	state := world.New()
	state.AddObject(a)
	regen := NewNPCRegen(state)
	regen.Tick() // backstop pass: binds the phase
	loop.Run()

	a.short.Store(true)
	a.SettleRegen() // armed at 0s, due at 3s
	a.regen.Unbind(&regen.armed)
	if n := len(regen.armed.appendTo(nil)); n != 0 {
		t.Fatalf("armed set holds %d actors after the unbind, want 0", n)
	}

	loop.Advance(time.Second)
	regen.sweeps = 0 // the next sweep is a backstop pass
	regen.Tick()
	loop.Run()
	if n := len(regen.armed.appendTo(nil)); n != 1 {
		t.Fatalf("armed set holds %d actors after the backstop pass, want 1", n)
	}

	loop.Advance(NPCRegenTick - time.Second - time.Millisecond)
	regen.Tick()
	loop.Run()
	if n := a.ticks.Load(); n != 0 {
		t.Fatalf("ticked %d times before the 3s deadline, want none", n)
	}
	loop.Advance(time.Millisecond)
	regen.Tick()
	loop.Run()
	if n := a.ticks.Load(); n != 1 {
		t.Fatalf("ticked %d times at the 3s deadline, want 1", n)
	}
}
