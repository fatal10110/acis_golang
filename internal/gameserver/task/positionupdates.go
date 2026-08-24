package task

import (
	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// PositionUpdateTick is the fixed movement correction interval.
const PositionUpdateTick = move.PositionUpdateInterval

// PositionUpdates runs correction ticks for actors with movement in flight.
//
// Add, Remove, and Contains are safe to call concurrently with Tick. Tick
// only ever runs on the scheduler ticker's single goroutine, one call at a
// time; ticking enforces that contract by logging and returning without
// doing anything else on a reentrant or concurrent Tick call, since Start's
// caller is the only reliable place that owns the scheduler goroutine.
type PositionUpdates struct {
	log zerolog.Logger

	*activeRegistry[int32, move.PositionUpdater]
}

// NewPositionUpdates returns an empty movement-correction registry.
func NewPositionUpdates(_ *world.State) *PositionUpdates {
	return &PositionUpdates{activeRegistry: newActiveRegistry[int32, move.PositionUpdater]()}
}

// Start launches the fixed movement-correction task.
func (p *PositionUpdates) Start(log zerolog.Logger) *scheduler.Ticker {
	p.log = log
	return scheduler.Start(PositionUpdateTick, p.Tick, log)
}

// Add registers actor for movement-correction ticks.
func (p *PositionUpdates) Add(actor move.PositionUpdater) {
	if actor == nil {
		return
	}
	p.add(actor.ObjectID(), actor)
}

// Remove unregisters actor from movement-correction ticks.
func (p *PositionUpdates) Remove(actor move.PositionUpdater) {
	if actor == nil {
		return
	}
	p.remove(actor.ObjectID())
}

// Contains reports whether actor is currently registered.
func (p *PositionUpdates) Contains(actor move.PositionUpdater) bool {
	if actor == nil {
		return false
	}
	return p.contains(actor.ObjectID())
}

// Tick advances every registered in-flight movement once. A PositionUpdate
// return of false means the actor's own bookkeeping already deregistered
// it (or decided it needs no further ticks) — Tick does not remove it
// again, since by the time PositionUpdate returns, a concurrent goroutine
// may have already re-added the same actor for a new move, and a
// second, redundant removal here would strip that fresh registration
// out from under it. It logs and returns without doing anything else if
// another Tick call is already in flight.
func (p *PositionUpdates) Tick() {
	if !p.beginTick(p.log, "task: PositionUpdates.Tick") {
		return
	}
	defer p.endTick()
	defer p.releaseSnapshot()

	for _, actor := range p.snapshot() {
		actor.PositionUpdate()
	}
}
