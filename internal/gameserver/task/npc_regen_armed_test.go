package task

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// sweptRegenActor is a regenerating actor whose shortfall the test sets,
// counting how often a sweep looks at its phase.
type sweptRegenActor struct {
	world.Presence
	id     int32
	q      *sim.Queue
	short  atomic.Bool
	looks  atomic.Int32
	ticks  atomic.Int32
	regen  creature.Regen
	shortF func() bool
}

func newSweptRegenActor(id int32, q *sim.Queue) *sweptRegenActor {
	a := &sweptRegenActor{id: id, q: q}
	a.shortF = a.short.Load
	return a
}

func (a *sweptRegenActor) ObjectID() int32 { return a.id }
func (*sweptRegenActor) Kind() actor.Kind  { return actor.KindNPC }
func (a *sweptRegenActor) Queue() *sim.Queue {
	return a.q
}

func (a *sweptRegenActor) Regen() *creature.Regen {
	a.looks.Add(1)
	return &a.regen
}
func (a *sweptRegenActor) SettleRegen() { a.regen.Settle(a.q, a.shortF) }
func (a *sweptRegenActor) TickRegen()   { a.ticks.Add(1); a.SettleRegen() }

// TestNPCRegenSweepWalksOnlyArmedPhases pins the sweep's cost to the armed
// phases: once a backstop pass has bound every actor, the sweeps between
// two backstop passes look at the one short actor's phase and at no idle
// one, and the short actor still ticks one period after its drop.
func TestNPCRegenSweepWalksOnlyArmedPhases(t *testing.T) {
	t.Parallel()
	loop := sim.NewInline(time.Unix(0, 0))
	q := loop.NewQueue("regen")
	state := world.New()
	idle := make([]*sweptRegenActor, 100)
	for i := range idle {
		idle[i] = newSweptRegenActor(int32(i)+1, q)
		state.AddObject(idle[i])
	}
	wounded := newSweptRegenActor(1000, q)
	state.AddObject(wounded)
	regen := NewNPCRegen(state)

	regen.Tick() // backstop pass: binds every phase
	loop.Run()
	for _, a := range idle {
		a.looks.Store(0)
	}

	loop.Advance(time.Second)
	wounded.short.Store(true)
	wounded.SettleRegen() // the hit, between two backstop passes
	for range regenBackstopEvery - 1 {
		loop.Advance(RegenSweep)
		regen.Tick()
		loop.Run()
	}
	for _, a := range idle {
		if n := a.looks.Load(); n != 0 {
			t.Fatalf("idle actor %d looked at %d times between backstop passes, want 0", a.id, n)
		}
	}
	if n := wounded.looks.Load(); n == 0 {
		t.Fatal("armed actor never looked at")
	}
	if n := wounded.ticks.Load(); n != 0 {
		t.Fatalf("armed actor ticked %d times by 3.9s, want none before 4s", n)
	}
	loop.Advance(RegenSweep)
	regen.Tick() // 4s: the backstop pass and the tick one period after the hit
	loop.Run()
	if n := wounded.ticks.Load(); n != 1 {
		t.Fatalf("armed actor ticked %d times at 4s, want 1", n)
	}
}

// TestNPCRegenSkipsUntrackedDuePhase covers an armed actor the world stopped
// tracking: its due tick is not run and it leaves the armed set with its
// phase still armed, and once tracked again the next backstop pass binds it
// and runs the overdue tick at once.
func TestNPCRegenSkipsUntrackedDuePhase(t *testing.T) {
	t.Parallel()
	loop := sim.NewInline(time.Unix(0, 0))
	a := newSweptRegenActor(1, loop.NewQueue("regen"))
	state := world.New()
	state.AddObject(a)
	regen := NewNPCRegen(state)
	regen.Tick() // backstop pass: binds the phase
	loop.Run()

	a.short.Store(true)
	a.SettleRegen()
	state.RemoveObject(a.id)
	loop.Advance(NPCRegenTick)
	regen.Tick()
	loop.Run()
	if n := a.ticks.Load(); n != 0 {
		t.Fatalf("untracked actor ticked %d times, want none", n)
	}
	if !a.regen.Active() {
		t.Fatal("skipping the untracked actor disarmed its phase")
	}
	if n := len(regen.armed.appendTo(nil)); n != 0 {
		t.Fatalf("armed set holds %d actors after the untracked one was skipped, want 0", n)
	}

	state.AddObject(a)
	regen.sweeps = 0 // the next sweep is a backstop pass
	regen.Tick()
	loop.Run()
	if n := a.ticks.Load(); n != 1 {
		t.Fatalf("tracked again: ticked %d times at the backstop pass, want 1", n)
	}
}

// TestNPCRegenArmedSetRace races settles that arm and disarm, removals and
// re-additions of the tracked objects, and sweeps, then checks the armed
// set agrees with the phases: with everyone full it is empty, and with
// everyone short it holds every actor.
func TestNPCRegenArmedSetRace(t *testing.T) {
	t.Parallel()
	loop := sim.NewInline(time.Unix(0, 0))
	q := loop.NewQueue("regen")
	state := world.New()
	actors := make([]*sweptRegenActor, 32)
	for i := range actors {
		actors[i] = newSweptRegenActor(int32(i)+1, q)
		state.AddObject(actors[i])
	}
	regen := NewNPCRegen(state)

	var wg sync.WaitGroup
	for _, a := range actors {
		wg.Go(func() {
			for i := range 200 {
				a.short.Store(i%3 != 0)
				a.SettleRegen()
				if i%50 == 0 {
					state.RemoveObject(a.id)
					state.AddObject(a)
				}
			}
		})
	}
	for range 300 {
		loop.Advance(RegenSweep)
		regen.Tick()
	}
	wg.Wait()
	loop.Run()

	for _, a := range actors {
		a.short.Store(false)
		a.SettleRegen()
	}
	if n := len(regen.armed.appendTo(nil)); n != 0 {
		t.Fatalf("armed set holds %d actors with every phase idle, want 0", n)
	}
	regen.sweeps = 0
	regen.Tick() // a backstop pass binds any phase a skip unbound
	loop.Run()
	for _, a := range actors {
		a.short.Store(true)
		a.SettleRegen()
	}
	if n := len(regen.armed.appendTo(nil)); n != len(actors) {
		t.Fatalf("armed set holds %d actors with every phase armed, want %d", n, len(actors))
	}
}
