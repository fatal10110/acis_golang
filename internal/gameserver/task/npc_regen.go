package task

import (
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// NPCRegenTick is the fixed HP/MP regeneration period (Formulas.
// getRegeneratePeriod's HP_REGENERATE_PERIOD, 3s).
const NPCRegenTick = creature.RegenPeriod

// RegenSweep is how often NPCRegen polls the regeneration phases: a tick
// runs at most this long after its deadline.
const RegenSweep = 100 * time.Millisecond

// regenBackstopEvery is how many sweeps pass between two backstop passes:
// one per regeneration period.
const regenBackstopEvery = int(NPCRegenTick / RegenSweep)

type npcRegenActor interface {
	Queued
	// Regen returns the actor's regeneration phase.
	Regen() *creature.Regen
	// TickRegen applies one regeneration step and settles the phase.
	TickRegen()
	// SettleRegen arms or disarms the phase to match the actor's vitals.
	SettleRegen()
}

// NPCRegen runs the regeneration ticks of every spawned actor that
// regenerates: attackable NPCs, players and summons. Each actor keeps its own
// fixed-rate phase (creature.Regen), armed the moment one of its resources
// drops, so its first tick comes one period after the drop and later drops do
// not move it. Every sweep posts the ticks that came due to their actors'
// queues.
//
// Once per period a sweep also settles every spawned actor, which arms the
// phase of any actor left short by a change that settled nothing: a value
// restored before the actor entered the world, or a maximum raised under a
// full resource.
//
// scratch is a per-tick scan buffer reused across calls instead of
// reallocated. tickGuard enforces the single-goroutine, one-call-at-a-time
// contract that reuse depends on: two concurrent Tick calls appending into
// the same backing array would silently drop or duplicate scanned actors
// rather than panic or race-detect, so Tick fails loudly instead of
// running when the guard is already held.
type NPCRegen struct {
	state   *world.State
	scratch []world.Tracked
	log     zerolog.Logger
	// sweeps counts Tick calls, to place the backstop passes; only the
	// guarded Tick touches it.
	sweeps int

	tickGuard
}

// NewNPCRegen returns a regeneration sweep over state's spawned actors.
func NewNPCRegen(state *world.State) *NPCRegen {
	return &NPCRegen{state: state}
}

// Start launches the regeneration sweep.
func (r *NPCRegen) Start(log zerolog.Logger) *scheduler.Ticker {
	r.log = log
	return scheduler.Start(RegenSweep, r.Tick, log)
}

// Tick posts every spawned actor's due regeneration tick to its queue,
// where the tick is claimed and applied. On the first call and every
// regenBackstopEvery calls after it, it also posts each actor's settle. It
// logs and returns without doing anything else if another Tick call is
// already in flight.
func (r *NPCRegen) Tick() {
	if r == nil || r.state == nil {
		return
	}
	if !r.beginTick(r.log, "task: NPCRegen.Tick") {
		return
	}
	defer r.endTick()

	backstop := r.sweeps%regenBackstopEvery == 0
	r.sweeps++
	r.scratch = r.state.AppendObjects(r.scratch[:0])
	for _, obj := range r.scratch {
		actor, ok := obj.(npcRegenActor)
		if !ok {
			continue
		}
		q := actor.Queue()
		if regen := actor.Regen(); regen.Active() && regen.Due(q.Now()) {
			// A later sweep may post again before this runs; only one claims.
			q.Post(func() {
				if regen.Claim(q.Now()) {
					actor.TickRegen()
				}
			})
		}
		if backstop {
			q.Post(actor.SettleRegen)
		}
	}
	// Drop references past this tick's length so a shrinking population
	// doesn't keep despawned objects reachable through unused capacity.
	clear(r.scratch[:cap(r.scratch)])
}
