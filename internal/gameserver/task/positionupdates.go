package task

import (
	"sync"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
)

// PositionUpdateTick is the fixed movement correction interval.
const PositionUpdateTick = move.PositionUpdateInterval

// PositionUpdates runs correction ticks for actors with movement in flight.
//
// mu guards entries and scratch. Tick only ever runs on the scheduler
// ticker's single goroutine, one call at a time, so no separate lock is
// needed to serialize it.
type PositionUpdates struct {
	mu sync.Mutex

	entries map[int32]move.PositionUpdater
	scratch []move.PositionUpdater
}

// NewPositionUpdates returns an empty movement-correction registry.
func NewPositionUpdates() *PositionUpdates {
	return &PositionUpdates{entries: make(map[int32]move.PositionUpdater)}
}

// Start launches the fixed movement-correction task.
func (p *PositionUpdates) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(PositionUpdateTick, p.Tick, log)
}

// Add registers actor for movement-correction ticks.
func (p *PositionUpdates) Add(actor move.PositionUpdater) {
	if actor == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries[actor.ObjectID()] = actor
}

// Remove unregisters actor from movement-correction ticks.
func (p *PositionUpdates) Remove(actor move.PositionUpdater) {
	if actor == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, actor.ObjectID())
}

// Contains reports whether actor is currently registered.
func (p *PositionUpdates) Contains(actor move.PositionUpdater) bool {
	if actor == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.entries[actor.ObjectID()]
	return ok
}

// Tick advances every registered in-flight movement once.
func (p *PositionUpdates) Tick() {
	p.mu.Lock()
	p.scratch = p.scratch[:0]
	for _, actor := range p.entries {
		p.scratch = append(p.scratch, actor)
	}
	actors := p.scratch
	p.mu.Unlock()

	for _, actor := range actors {
		if !actor.PositionUpdate() {
			p.Remove(actor)
		}
	}
}
