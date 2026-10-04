package task

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// phasedRegenActor is a regenerating actor on its own virtual clock whose
// shortfall the test sets.
type phasedRegenActor struct {
	world.Presence
	q     *sim.Queue
	short bool
	ticks []time.Time
	regen creature.Regen
}

func (*phasedRegenActor) ObjectID() int32          { return 1 }
func (*phasedRegenActor) Kind() actor.Kind         { return actor.KindNPC }
func (a *phasedRegenActor) Queue() *sim.Queue      { return a.q }
func (a *phasedRegenActor) Regen() *creature.Regen { return &a.regen }
func (a *phasedRegenActor) SettleRegen()           { a.regen.Settle(a.q, func() bool { return a.short }) }

func (a *phasedRegenActor) TickRegen() { a.ticks = append(a.ticks, a.q.Now()); a.SettleRegen() }

func (a *phasedRegenActor) sinceStart(i int) string { return a.ticks[i].Sub(time.Unix(0, 0)).String() }

// TestNPCRegenRunsDuePhaseOnce pins the sweep side of the per-actor phase:
// nothing runs before the deadline one period after the drop, and two sweeps
// that both find the tick due before the actor's queue runs it still apply
// it once, as the reference's single scheduled task would.
func TestNPCRegenRunsDuePhaseOnce(t *testing.T) {
	t.Parallel()
	loop := sim.NewInline(time.Unix(0, 0))
	a := &phasedRegenActor{q: loop.NewQueue("regen")}
	state := world.New()
	state.AddObject(a)
	regen := NewNPCRegen(state)

	loop.Advance(1200 * time.Millisecond)
	a.short = true
	a.SettleRegen() // the hit

	loop.Advance(2999 * time.Millisecond)
	regen.Tick() // also the backstop pass: must not move the phase
	loop.Run()
	if len(a.ticks) != 0 {
		t.Fatalf("ticked at %s, before the 4.2s deadline", a.sinceStart(0))
	}

	loop.Advance(time.Millisecond)
	regen.Tick()
	regen.Tick()
	loop.Run()
	if len(a.ticks) != 1 || a.sinceStart(0) != "4.2s" {
		t.Fatalf("ticks = %v, want one at 4.2s", a.ticks)
	}

	loop.Advance(3 * time.Second)
	regen.Tick()
	loop.Run()
	if len(a.ticks) != 2 || a.sinceStart(1) != "7.2s" {
		t.Fatalf("ticks = %v, want the second at 7.2s", a.ticks)
	}
}

// TestNPCRegenBackstopArmsShortActor covers a resource left short by a
// change that settled nothing: the backstop pass arms its phase, and the
// first tick follows one period later.
func TestNPCRegenBackstopArmsShortActor(t *testing.T) {
	t.Parallel()
	loop := sim.NewInline(time.Unix(0, 0))
	a := &phasedRegenActor{q: loop.NewQueue("regen"), short: true}
	state := world.New()
	state.AddObject(a)
	regen := NewNPCRegen(state)

	regen.Tick()
	loop.Run()
	if !a.regen.Active() {
		t.Fatal("backstop pass left a short actor idle")
	}
	loop.Advance(3 * time.Second)
	regen.Tick()
	loop.Run()
	if len(a.ticks) != 1 || a.sinceStart(0) != "3s" {
		t.Fatalf("ticks = %v, want one at 3s", a.ticks)
	}
}
