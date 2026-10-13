package npc

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

const (
	// folkStaleAggroAge is how old the last damage of an aggro list entry
	// is, at least, when the out-of-territory sweep stops its hate.
	folkStaleAggroAge = 90 * time.Second
	// folkSweepDelay is the delay from the first arrival outside territory
	// to the first sweep, and folkSweepPeriod the fixed rate of the later
	// ones.
	folkSweepDelay  = 100 * time.Millisecond
	folkSweepPeriod = 10 * time.Second
)

// folkOutOfTerritory is this life's part of a civilian NPC's
// out-of-territory state, guarded by folkCast.brain: timer and seq are the
// stale-aggro sweep's pending firing on this life's queue (nil timer for
// none) and the generation a firing must match to run. Whether the last
// arrival was outside, and the sweep's next deadline on its fixed-rate
// grid, are kept across lives in the NPC's Scratch.
type folkOutOfTerritory struct {
	timer *sim.Timer
	seq   uint64
}

// keepTerritory resumes, before f is published, the out-of-territory state
// f's earlier lives left: when the last of their arrivals was outside, the
// stale-aggro sweep goes on, on its grid, on f's queue, and f's next arrival
// outside opens no out-of-territory point.
func (f *Folk) keepTerritory() {
	f.cast.brain.Lock()
	defer f.cast.brain.Unlock()
	memory := f.Scratch().Territory()
	outside, due := memory.Load()
	if !outside {
		return
	}
	next, d := ai.NextOnGrid(due, f.now(), folkSweepPeriod)
	memory.Store(true, next)
	f.armSweepAt(d)
}

// atHookPoint hands f's behavior hook point p with brain released. The
// caller holds brain and holds it again on return; a pass the point raises,
// or one another goroutine runs meanwhile, may have changed anything brain
// guards.
func (f *Folk) atHookPoint(p ai.HookPoint) {
	f.cast.brain.Unlock()
	defer f.cast.brain.Lock()
	f.AtHookPoint(p)
}

// syncOutOfTerritory settles f's out-of-territory state on an arrival, under
// brain. The first arrival outside its territory opens the out-of-territory
// point, then arms the stale-aggro sweep: 100 ms later, then every 10 s,
// while f is still outside, each aggro list entry whose last damage is 90 s
// old or more loses its hate and f's attack desire on it. An arrival back
// inside disarms the sweep. Nothing else disarms it: a sweep on an NPC that
// hates no one finds nothing to do, and the NPC's queue closing at its decay
// cancels this life's firing; the next life resumes it on its grid
// (keepTerritory).
func (f *Folk) syncOutOfTerritory() {
	memory := f.Scratch().Territory()
	if f.InTerritory() {
		memory.Store(false, time.Time{})
		f.stopSweep()
		return
	}
	if outside, _ := memory.Load(); outside {
		return
	}
	f.atHookPoint(ai.HookOutOfTerritory)
	f.stopSweep()
	f.armSweep(folkSweepDelay)
}

// stopSweep disarms f's stale-aggro sweep, under brain.
func (f *Folk) stopSweep() {
	oot := &f.cast.oot
	oot.seq++
	if oot.timer != nil {
		oot.timer.Stop()
		oot.timer = nil
	}
}

// armSweep schedules f's next stale-aggro sweep d from now on f's queue and
// records f outside with that deadline, under brain.
func (f *Folk) armSweep(d time.Duration) {
	f.Scratch().Territory().Store(true, f.now().Add(d))
	f.armSweepAt(d)
}

// armSweepAt schedules this life's next stale-aggro sweep firing d from now
// on f's queue, under brain.
func (f *Folk) armSweepAt(d time.Duration) {
	if f.queue == nil {
		return
	}
	oot := &f.cast.oot
	seq := oot.seq
	oot.timer = f.queue.After(d, func() { f.sweepStaleAggro(seq) })
}

// sweepStaleAggro is one firing of f's stale-aggro sweep: it re-arms on the
// fixed-rate grid, then, while f stands outside its territory, stops the
// hate of each aggro list entry whose last damage is folkStaleAggroAge old
// or more and drops f's attack desire on it. A firing of a sweep since
// disarmed does nothing.
func (f *Folk) sweepStaleAggro(seq uint64) {
	f.cast.brain.Lock()
	defer f.cast.brain.Unlock()
	oot := &f.cast.oot
	if seq != oot.seq {
		return
	}
	now := f.now()
	_, due := f.Scratch().Territory().Load()
	_, d := ai.NextOnGrid(due.Add(folkSweepPeriod), now, folkSweepPeriod)
	f.armSweep(d)
	if f.InTerritory() {
		return
	}
	for _, t := range f.fight.aggro.Snapshot() {
		if now.Sub(t.Timestamp) < folkStaleAggroAge {
			continue
		}
		f.cast.desires.Remove(ai.IntentionAttack, t.Attacker)
		f.fight.aggro.StopHate(t.Attacker)
	}
}
