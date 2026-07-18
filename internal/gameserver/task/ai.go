package task

import (
	"sync"
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
// All methods are safe for concurrent use; mu guards actors.
type AI struct {
	state *world.State

	mu       sync.RWMutex
	actors   map[int32]AIActor
	inactive map[int32]struct{}
}

// NewAI returns an empty active-AI registry. A nil state treats every actor
// as active. When state is non-nil, registered actors must already be
// spawned into it; off-grid actors are not ticked.
func NewAI(state *world.State) *AI {
	return &AI{
		state:    state,
		actors:   make(map[int32]AIActor),
		inactive: make(map[int32]struct{}),
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
	delete(a.inactive, actor.ObjectID())
}

// Remove unregisters actor from recurring AI ticks.
func (a *AI) Remove(actor AIActor) {
	if actor == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.actors, actor.ObjectID())
	delete(a.inactive, actor.ObjectID())
}

// Tick runs one AI cycle for every registered actor in an active region,
// and for inactive-region actors that explicitly opt out of sleeping.
func (a *AI) Tick() {
	a.mu.RLock()
	actors := make([]AIActor, 0, len(a.actors))
	for _, actor := range a.actors {
		actors = append(actors, actor)
	}
	a.mu.RUnlock()

	for _, actor := range actors {
		placed, active := regionActivity(a.state, actor)
		switch {
		case !placed:
			a.clearInactive(actor)
			continue
		case active:
			a.clearInactive(actor)
		default:
			if a.markInactive(actor) {
				notifyInactiveRegion(actor)
			}
			if sleepsWhenRegionInactive(actor) {
				continue
			}
		}
		actor.Tick()
		actor.Think()
	}
}

func (a *AI) markInactive(actor AIActor) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := actor.ObjectID()
	if _, ok := a.actors[id]; !ok {
		return false
	}
	if _, ok := a.inactive[id]; ok {
		return false
	}
	a.inactive[id] = struct{}{}
	return true
}

func (a *AI) clearInactive(actor AIActor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.inactive, actor.ObjectID())
}
