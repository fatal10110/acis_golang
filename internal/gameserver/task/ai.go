package task

import (
	"errors"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ErrReentrantTick is returned by AI, Decay, and AttackStance's Tick when a
// call arrives while another Tick on the same registry is still running.
// Each of those types documents a single-goroutine, one-call-at-a-time
// contract; a caller that bypasses their Start wiring and invokes Tick from
// more than one goroutine gets this error instead of silently corrupting the
// in-flight tick's shared scratch buffer.
var ErrReentrantTick = errors.New("task: Tick called concurrently; Tick is single-goroutine only")

// AITick is the fixed hostile-NPC AI interval.
const AITick = time.Second

// AIActor is the narrow actor brain surface the AI task runs.
type AIActor interface {
	world.Tracked
	Tick()
	Think() error
}

// AI runs active actor brains once per tick.
//
// Add and Remove are safe to call concurrently with Tick. Tick only ever
// runs on the scheduler ticker's single goroutine, one call at a time;
// ticking enforces that contract by logging and returning ErrReentrantTick
// on reentrant or concurrent Tick calls instead of running, since Start's
// caller is the only reliable place that can act on the returned error.
type AI struct {
	state *world.State
	log   zerolog.Logger

	*activeRegistry[int32, AIActor]
}

// NewAI returns an empty active-AI registry. A nil state treats every actor
// as active. When state is non-nil, registered actors must already be
// spawned into it; off-grid actors are not ticked.
func NewAI(state *world.State) *AI {
	return &AI{state: state, activeRegistry: newActiveRegistry[int32, AIActor]()}
}

// Start launches the fixed one-second AI task.
func (a *AI) Start(log zerolog.Logger) *scheduler.Ticker {
	a.log = log
	return scheduler.Start(AITick, func() { a.Tick() }, log)
}

// Add registers actor for recurring AI ticks.
func (a *AI) Add(actor AIActor) {
	if actor == nil {
		return
	}
	a.add(actor.ObjectID(), actor)
}

// Remove unregisters actor from recurring AI ticks.
func (a *AI) Remove(actor AIActor) {
	if actor == nil {
		return
	}
	a.remove(actor.ObjectID())
}

// Tick runs one AI cycle for every registered actor in an active region,
// and for inactive-region actors that explicitly opt out of sleeping. It
// logs and returns ErrReentrantTick without doing anything else if another Tick call is
// already in flight.
func (a *AI) Tick() error {
	if !a.ticking.CompareAndSwap(false, true) {
		a.log.Error().Err(ErrReentrantTick).Msg("task: AI.Tick")
		return ErrReentrantTick
	}
	defer a.ticking.Store(false)
	defer a.release()

	for _, actor := range a.snapshot() {
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
		if err := actor.Think(); err != nil {
			a.log.Warn().Err(err).Int32("actor_id", actor.ObjectID()).Msg("ai: think")
		}
	}
	return nil
}
