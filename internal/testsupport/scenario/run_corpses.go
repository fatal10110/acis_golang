package scenario

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// corpseStep is how long the clock of the corpses moves between two decay
// sweeps in a kill step: the reference probe's tick.
const corpseStep = 500 * time.Millisecond

// corpses decays the dead NPCs whose line names a corpse time. Its clock is
// the scenario's own, moved by the steps that let time pass, and the decay
// sweep, which the server runs each second, runs as it moves.
type corpses struct {
	ns    atomic.Int64
	task  *task.Decay
	state atomic.Pointer[world.State]
	// respawnHook is the server's hook of a decaying NPC its spawn
	// replaces (nil for none), set once it booted.
	respawnHook atomic.Pointer[func(int32) func()]
}

func newCorpses(t *testing.T, start time.Time) *corpses {
	t.Helper()
	c := &corpses{}
	c.ns.Store(start.UnixNano())
	d, err := task.NewDecay(c, c.now)
	if err != nil {
		t.Fatalf("new decay: %v", err)
	}
	c.task = d
	return c
}

func (c *corpses) now() time.Time { return time.Unix(0, c.ns.Load()) }

// Decay removes a corpse from the world, as the server's decay does.
func (c *corpses) Decay(actor task.DecayActor) {
	state := c.state.Load()
	if state == nil {
		return
	}
	if obj, ok := state.Object(actor.ObjectID()); ok {
		if corpse, ok := obj.(interface {
			Decay(*world.State, func()) bool
		}); ok {
			var hook func()
			if f := c.respawnHook.Load(); f != nil {
				hook = (*f)(actor.ObjectID())
			}
			corpse.Decay(state, hook)
		}
	}
}

// hasCorpses reports whether an npc line or an npcmaker line's spawn names
// a corpse time.
func (sc *Scenario) hasCorpses() bool {
	for _, n := range sc.NPCs {
		if n.Corpse != 0 {
			return true
		}
	}
	return slices.ContainsFunc(sc.NpcMakers, func(m NpcMaker) bool { return m.Spawn != nil && m.Spawn.Corpse != 0 })
}

// passCorpseTime moves the corpses' clock by d and sweeps the decay task
// once, letting the NPCs it removes finish leaving.
func (r *run) passCorpseTime(t *testing.T, d time.Duration) {
	t.Helper()
	if r.corpses == nil {
		return
	}
	r.corpses.ns.Add(int64(d))
	if err := r.corpses.task.Tick(); err != nil {
		t.Fatalf("%s: decay tick: %v", r.sc.Name, err)
	}
	r.srv.Settle(t)
}

// passKillTime lets the time pass that a kill step waits for. A scenario
// with corpses passes it in the probe's ticks, sweeping the decay task after
// each, so a corpse leaves at the tick the reference's does.
func (r *run) passKillTime(t *testing.T) {
	t.Helper()
	if r.corpses == nil {
		r.srv.Advance(t, killWait)
		return
	}
	for range killWait / corpseStep {
		r.srv.Advance(t, corpseStep)
		r.passCorpseTime(t, corpseStep)
		r.passRespawns(t, corpseStep)
	}
}
