package task

import (
	"sync/atomic"

	"github.com/rs/zerolog"
)

// tickGuard enforces a single-goroutine, one-call-at-a-time Tick contract,
// shared by serialDeadlineRegistry (Decay, AttackStance) and activeRegistry
// (AI, PositionUpdates). endTick only ever releases the guard; a type that
// also needs to clear a reused scratch buffer on tick completion does so
// with its own separately named step (see activeRegistry.releaseSnapshot),
// so that endTick means the same thing everywhere it's called.
type tickGuard struct {
	ticking atomic.Bool
}

// beginTick claims the guard, or logs msg via log and reports false if
// another Tick is already running.
func (g *tickGuard) beginTick(log zerolog.Logger, msg string) bool {
	if !g.ticking.CompareAndSwap(false, true) {
		log.Error().Err(ErrReentrantTick).Msg(msg)
		return false
	}
	return true
}

// endTick releases the guard claimed by a successful beginTick.
func (g *tickGuard) endTick() {
	g.ticking.Store(false)
}
