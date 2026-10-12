package summon

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// A servitor's lifetime counts down once a second; a pet eats every ten
// seconds. Both tasks run at a fixed rate, the first tick one period after
// the start.
const (
	servitorLifePeriod = time.Second
	petFeedPeriod      = 10 * time.Second
)

// lifeTask is a summon's lifetime ticker: a servitor's countdown or a pet's
// feeding. Its ticks run on the summon's queue. mu guards ticker, gen and
// gone: the task starts on spawn (and, for a pet, on revive) and stops on
// death and departure, and death can come from the queue of whoever landed
// the killing hit.
type lifeTask struct {
	mu     sync.Mutex
	ticker *sim.Ticker
	// gen tells a tick of a stopped task, which a queue may still run once,
	// from a tick of the running one.
	gen uint64
	// gone is set once the summon has left the world; the task never starts
	// again after that.
	gone bool
}

// startLifeTask starts a's lifetime task on its queue, unless a is dead,
// gone, has no queue, or the task already runs. A dead servitor's lifetime
// stays stopped, even once it is revived; only a pet starts it again
// (revive).
func (a *Actor) startLifeTask() {
	t := &a.life
	t.mu.Lock()
	defer t.mu.Unlock()
	// Read under mu, so a death that stops the task after marking the
	// summon dead cannot be overtaken by a start that saw it alive.
	if t.gone || t.ticker != nil || a.Dead() {
		return
	}
	q := a.Queue()
	if q == nil {
		return
	}
	period := servitorLifePeriod
	if a.isPet {
		period = petFeedPeriod
	}
	gen := t.gen
	t.ticker = q.Every(period, func() { a.runLifeTick(gen) })
}

// stopLifeTask stops a's lifetime task, if it runs. gone also keeps it from
// ever starting again.
func (a *Actor) stopLifeTask(gone bool) {
	t := &a.life
	t.mu.Lock()
	defer t.mu.Unlock()
	if gone {
		t.gone = true
	}
	t.gen++
	if t.ticker != nil {
		t.ticker.Stop()
		t.ticker = nil
	}
}

// moveLifeTask moves a running lifetime task onto a's queue, which a has
// just moved to: the task stops on the queue a left and starts again, a
// full period away, on the new one. A task that is not running stays
// stopped.
func (a *Actor) moveLifeTask() {
	t := &a.life
	t.mu.Lock()
	running := t.ticker != nil
	t.mu.Unlock()
	if !running {
		return
	}
	a.stopLifeTask(false)
	a.startLifeTask()
}

// runLifeTick is one tick of the lifetime task started as generation gen.
func (a *Actor) runLifeTick(gen uint64) {
	t := &a.life
	t.mu.Lock()
	current := t.gen == gen && !t.gone
	t.mu.Unlock()
	if !current {
		return
	}
	if a.isPet {
		a.TickPet(a.world)
		return
	}
	a.TickServitor(a.world)
}
