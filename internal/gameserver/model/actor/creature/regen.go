package creature

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// RegenPeriod is the HP/MP/CP regeneration period of every creature but a
// door (Formulas.getRegeneratePeriod: HP_REGENERATE_PERIOD, 3s).
const RegenPeriod = 3 * time.Second

// Regen is one creature's regeneration phase (CreatureStatus._regTask): the
// deadline of its next fixed-rate tick. Settle arms it when a resource first
// falls short of its maximum, its first tick one full period later; later
// changes while it is armed leave the phase alone. It is disarmed once
// nothing is short or the creature is dead, so the next drop starts a fresh
// phase. The regeneration sweep runs the ticks that come due (Claim).
//
// The zero value is idle. All methods are safe for concurrent use.
type Regen struct {
	mu sync.Mutex
	// due is the next tick's deadline in Unix nanoseconds, zero while
	// idle. It is written under mu and read without it by Due.
	due atomic.Int64
}

// Settle arms or disarms the phase to match short, which reports whether the
// creature is in the world and alive with a resource below its maximum. An
// arming call starts the phase at the time of q, the creature's queue; a
// creature with no queue yet stays idle.
//
// Call it after every change to the creature's current resources, outside
// the locks short takes. short is read under the phase's own lock, so of two
// concurrent calls the later one decides from the later values: a tick that
// fills the creature and a hit that lands just after it cannot leave a
// wounded creature idle.
func (r *Regen) Settle(q *sim.Queue, short func() bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if q == nil || !short() {
		r.due.Store(0)
		return
	}
	if r.due.Load() == 0 {
		r.due.Store(q.Now().Add(RegenPeriod).UnixNano())
	}
}

// Due reports whether the phase is armed with a tick due at now. It takes no
// lock, so a sweep can poll every creature cheaply; Claim decides.
func (r *Regen) Due(now time.Time) bool {
	due := r.due.Load()
	return due != 0 && due <= now.UnixNano()
}

// Claim takes the tick due at now, if any, and reports whether it did. The
// phase moves to the next deadline on its fixed-rate grid; deadlines a
// stalled sweep missed entirely are skipped rather than run back to back.
func (r *Regen) Claim(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	due, at := r.due.Load(), now.UnixNano()
	if due == 0 || due > at {
		return false
	}
	period := int64(RegenPeriod)
	due += period
	if due <= at {
		due += ((at-due)/period + 1) * period
	}
	r.due.Store(due)
	return true
}

// Active reports whether the phase is armed.
func (r *Regen) Active() bool {
	return r.due.Load() != 0
}
