package cubic

import (
	"sync"
	"time"
)

// Timer is the narrow surface Runtime needs from a scheduled callback,
// satisfied by *time.Timer.
type Timer interface {
	Stop() bool
}

// AfterFunc schedules fn to run once after d elapses, returning a handle
// that can cancel it. Tests inject a deterministic stand-in; production
// wiring passes time.AfterFunc.
type AfterFunc func(d time.Duration, fn func()) Timer

// Runtime is one live cubic's timers: the recurring action tick (started
// immediately for the Life Cubic, or lazily via Action() the first time the
// owner enters combat stance for every other kind, matching
// Cubic.doAction()'s idempotent restart guard) and the one-shot disappear
// timer that fires when the granted lifetime elapses. Level is the level of
// the SUMMON skill that granted this cubic — fixed at construction, not
// re-read on a later refresh, matching CubicList.addOrRefreshCubic only
// resetting the disappear timer for an already-active cubic.
type Runtime struct {
	id ID
	// Level is the level of the SUMMON skill that granted this cubic.
	// ActivationChance is that same granting skill's own percent gate on
	// each non-Life action tick (Cubic._activationChance) — both fixed at
	// construction, distinct from any later-fired skill's own data.
	Level            int
	ActivationChance int
	interval         time.Duration
	fire             func()
	disappear        func()
	afterFunc        AfterFunc

	mu      sync.Mutex
	running bool
	// generation increments on every Action()/StopAction() call, so a tick
	// scheduled by a now-superseded Action() call can tell it is stale and
	// must not reschedule itself. Without this, a StopAction() immediately
	// followed by an Action() while a tick's fire() is still running (a
	// realistic race: the timer callback and an attack-stance-driven
	// Action()/StopAction() call run on different goroutines) would leave
	// two independent timer chains running, doubling the fire rate.
	generation     int
	actionTimer    Timer
	disappearTimer Timer
}

// NewRuntime builds a cubic runtime. fire runs one action tick; disappear
// runs once when the granted lifetime elapses. Neither timer starts until
// Action() and RefreshDisappear() are called.
func NewRuntime(id ID, level, activationChance int, interval time.Duration, fire, disappear func(), afterFunc AfterFunc) *Runtime {
	if afterFunc == nil {
		afterFunc = func(d time.Duration, fn func()) Timer { return time.AfterFunc(d, fn) }
	}
	return &Runtime{id: id, Level: level, ActivationChance: activationChance, interval: interval, fire: fire, disappear: disappear, afterFunc: afterFunc}
}

// ID satisfies task.AttackStanceCubic.
func (r *Runtime) ID() int { return int(r.id) }

// Action starts the recurring action tick if it isn't already running,
// matching Cubic.doAction()'s no-op-if-already-active guard.
func (r *Runtime) Action() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return
	}
	r.running = true
	r.generation++
	gen := r.generation
	r.actionTimer = r.afterFunc(r.interval, func() { r.tick(gen) })
}

func (r *Runtime) tick(gen int) {
	r.mu.Lock()
	if !r.running || gen != r.generation {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	// fire() panicking must not leave running stuck true: that would make
	// Action()'s no-op-if-already-active guard silently block every future
	// stance re-entry, stalling this cubic for the rest of its grant even
	// though the panic itself gets contained and logged by the caller's
	// recovered afterFunc. Reset state like StopAction() before re-panicking
	// so the caller's recover still logs the value.
	defer func() {
		if p := recover(); p != nil {
			r.mu.Lock()
			if gen == r.generation {
				r.running = false
			}
			r.mu.Unlock()
			panic(p)
		}
	}()

	r.fire()

	r.mu.Lock()
	if r.running && gen == r.generation {
		r.actionTimer = r.afterFunc(r.interval, func() { r.tick(gen) })
	}
	r.mu.Unlock()
}

// StopAction cancels the recurring action tick only, matching
// Cubic.stopAction() — used when a non-Life cubic finds its owner out of
// combat stance on a fire attempt, so it goes idle until the owner's next
// combat entry reactivates it via Action().
func (r *Runtime) StopAction() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	r.generation++
	if r.actionTimer != nil {
		r.actionTimer.Stop()
	}
}

// RefreshDisappear (re)starts the disappear timer for lifetime, matching
// CubicList.addOrRefreshCubic's refreshDisappearTask.
func (r *Runtime) RefreshDisappear(lifetime time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.disappearTimer != nil {
		r.disappearTimer.Stop()
	}
	r.disappearTimer = r.afterFunc(lifetime, r.disappear)
}

// Stop cancels both timers, matching Cubic.stop().
func (r *Runtime) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	r.generation++
	if r.actionTimer != nil {
		r.actionTimer.Stop()
	}
	if r.disappearTimer != nil {
		r.disappearTimer.Stop()
	}
}
