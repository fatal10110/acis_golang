package task

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// AITick is the fixed hostile-NPC AI interval.
const AITick = time.Second

// AIActor is the narrow actor brain surface the AI task runs.
type AIActor interface {
	world.Tracked
	Tick()
	Think()
}

// AI runs active actor brains once per tick.
//
// Add and Remove are safe to call concurrently with Tick. mu guards actors and
// the scratch refill. Tick only ever runs on the scheduler ticker's single
// goroutine, one call at a time; ticking enforces that contract by panicking
// on reentrant or concurrent Tick calls.
type AI struct {
	state *world.State

	mu      sync.Mutex
	actors  map[int32]AIActor
	scratch []AIActor

	ticking atomic.Bool
}

// NewAI returns an empty active-AI registry. A nil state treats every actor
// as active. When state is non-nil, registered actors must already be
// spawned into it; off-grid actors are not ticked.
func NewAI(state *world.State) *AI {
	return &AI{
		state:  state,
		actors: make(map[int32]AIActor),
	}
}

// Start launches the fixed one-second AI task.
func (a *AI) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(AITick, a.Tick, log)
}

// Add registers actor for recurring AI ticks.
func (a *AI) Add(actor AIActor) {
	if actor == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actors[actor.ObjectID()] = actor
}

// Remove unregisters actor from recurring AI ticks.
func (a *AI) Remove(actor AIActor) {
	if actor == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.actors, actor.ObjectID())
}

// Tick runs one AI cycle for every registered actor in an active region,
// and for inactive-region actors that explicitly opt out of sleeping.
func (a *AI) Tick() {
	if !a.ticking.CompareAndSwap(false, true) {
		panic("task: AI.Tick called concurrently; Tick is single-goroutine only")
	}
	defer a.ticking.Store(false)

	a.mu.Lock()
	a.scratch = a.scratch[:0]
	for _, actor := range a.actors {
		a.scratch = append(a.scratch, actor)
	}
	actors := a.scratch
	a.mu.Unlock()

	defer clear(a.scratch)

	for _, actor := range actors {
		placed, active := regionActivity(a.state, actor)
		switch {
		case !placed:
			continue
		case active:
		default:
			if sleepsWhenRegionInactive(actor) {
				continue
			}
		}
		actor.Tick()
		actor.Think()
	}
}
