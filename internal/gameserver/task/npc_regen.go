package task

import (
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// NPCRegenTick is the fixed HP/MP regeneration period, 3s.
const NPCRegenTick = creature.RegenPeriod

// RegenSweep is how often NPCRegen polls the regeneration phases: a tick
// runs at most this long after its deadline.
const RegenSweep = 100 * time.Millisecond

// regenBackstopEvery is how many sweeps pass between two backstop passes:
// one per regeneration period.
const regenBackstopEvery = int(NPCRegenTick / RegenSweep)

type npcRegenActor interface {
	world.Tracked
	Queued
	// Regen returns the actor's regeneration phase.
	Regen() *creature.Regen
	// TickRegen applies one regeneration step and settles the phase.
	TickRegen()
	// SettleRegen arms or disarms the phase to match the actor's vitals.
	SettleRegen()
	// RegenShort reports whether the actor has something to regenerate: in
	// the world, alive and below a maximum. It is safe from any goroutine
	// holding none of the actor's locks.
	RegenShort() bool
}

// NPCRegen runs the regeneration ticks of every spawned actor that
// regenerates: attackable NPCs, players and summons. Each actor keeps its own
// fixed-rate phase (creature.Regen), armed the moment one of its resources
// drops, so its first tick comes one period after the drop and later drops do
// not move it. Every sweep posts the ticks that came due to their actors'
// queues, walking only the armed phases: each phase is bound to the sweep's
// armed set, which it joins when it arms and leaves when it disarms.
//
// Once per period a backstop pass visits every tracked object. It binds each
// actor's phase to the armed set, so an actor joins the sweep within one
// period of entering the world, never later than its first tick can come
// due, and a phase that dropped out of the set is back in it by the next
// pass. It also settles each idle actor left short by a change that settled
// nothing, such as an NPC's maximum raised under a full resource, which arms
// its phase. An armed phase needs no settle (its next tick settles it), and
// neither does an idle actor with nothing to regenerate, so a pass posts
// nothing to either.
//
// scratch and due are per-tick scan buffers reused across calls instead of
// reallocated. tickGuard enforces the single-goroutine, one-call-at-a-time
// contract that reuse depends on: two concurrent Tick calls appending into
// the same backing array would silently drop or duplicate scanned actors
// rather than panic or race-detect, so Tick fails loudly instead of
// running when the guard is already held.
type NPCRegen struct {
	state   *world.State
	armed   armedRegens
	scratch []world.Tracked
	due     []npcRegenActor
	log     zerolog.Logger
	// sweeps counts Tick calls, to place the backstop passes; only the
	// guarded Tick touches it.
	sweeps int

	tickGuard
}

// NewNPCRegen returns a regeneration sweep over state's spawned actors.
func NewNPCRegen(state *world.State) *NPCRegen {
	return &NPCRegen{state: state, armed: armedRegens{actors: make(map[npcRegenActor]struct{})}}
}

// Start launches the regeneration sweep.
func (r *NPCRegen) Start(log zerolog.Logger) *scheduler.Ticker {
	r.log = log
	return scheduler.Start(RegenSweep, r.Tick, log)
}

// Tick posts every armed actor's due regeneration tick to its queue, where
// the tick is claimed and applied. On the first call and every
// regenBackstopEvery calls after it, it first runs the backstop pass. It
// logs and returns without doing anything else if another Tick call is
// already in flight.
//
// A due actor no longer tracked by the world is not ticked: its phase leaves
// the armed set, and a backstop pass binds it again should it come back.
func (r *NPCRegen) Tick() {
	if r == nil || r.state == nil {
		return
	}
	if !r.beginTick(r.log, "task: NPCRegen.Tick") {
		return
	}
	defer r.endTick()

	if r.sweeps%regenBackstopEvery == 0 {
		r.backstop()
	}
	r.sweeps++

	r.due = r.armed.appendTo(r.due[:0])
	for _, actor := range r.due {
		regen, q := actor.Regen(), actor.Queue()
		if !regen.Due(q.Now()) {
			continue
		}
		if obj, ok := r.state.Object(actor.ObjectID()); !ok || obj != world.Tracked(actor) {
			regen.Unbind(&r.armed)
			continue
		}
		// A later sweep may post again before this runs; only one claims.
		if !q.Post(func() {
			if regen.Claim(q.Now()) {
				actor.TickRegen()
			}
		}) {
			regen.Unbind(&r.armed)
		}
	}
	clear(r.due)
}

// backstop binds every tracked actor's phase to the armed set and posts a
// settle to each idle actor left short.
func (r *NPCRegen) backstop() {
	r.scratch = r.state.AppendObjects(r.scratch[:0])
	for _, obj := range r.scratch {
		actor, ok := obj.(npcRegenActor)
		if !ok {
			continue
		}
		regen := actor.Regen()
		regen.Bind(&r.armed, actor)
		// The settle runs on the actor's queue and decides from the vitals
		// there; a change racing this read settles on its own.
		if !regen.Active() && actor.RegenShort() {
			actor.Queue().Post(actor.SettleRegen)
		}
	}
	// Drop references past this pass's length so a shrinking population
	// doesn't keep despawned objects reachable through unused capacity.
	clear(r.scratch[:cap(r.scratch)])
}

// armedRegens is the set of armed phases bound to a sweep
// (creature.RegenSet), keyed by their actors. A phase calls Arm and Disarm
// under its own lock, so the lock order is phase, then mu: nothing holding mu
// calls into a phase.
type armedRegens struct {
	mu     sync.Mutex
	actors map[npcRegenActor]struct{}
}

// Arm adds owner, the actor its phase was bound with.
func (s *armedRegens) Arm(owner any) {
	actor, ok := owner.(npcRegenActor)
	if !ok {
		return
	}
	s.mu.Lock()
	s.actors[actor] = struct{}{}
	s.mu.Unlock()
}

// Disarm removes owner.
func (s *armedRegens) Disarm(owner any) {
	actor, ok := owner.(npcRegenActor)
	if !ok {
		return
	}
	s.mu.Lock()
	delete(s.actors, actor)
	s.mu.Unlock()
}

// appendTo appends every armed actor to dst and returns the extended slice.
func (s *armedRegens) appendTo(dst []npcRegenActor) []npcRegenActor {
	s.mu.Lock()
	defer s.mu.Unlock()
	for actor := range s.actors {
		dst = append(dst, actor)
	}
	return dst
}
