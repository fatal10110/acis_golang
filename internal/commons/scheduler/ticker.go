// Package scheduler provides the fixed-rate ticker every periodic subsystem
// (aggro/attack-stance expiry, PvP flag decay, item respawn, ...) is built on
// top of.
package scheduler

import (
	"context"
	"expvar"
	"fmt"
	"path"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

var tickerVars = expvar.NewMap("tickers")

// Ticker calls a callback on a fixed period, on its own goroutine, until
// Stop is called.
type Ticker struct {
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	// cancel ends the context StartContext hands every tick. Start's ticks
	// take no context, so for them it ends nothing anyone watches.
	cancel context.CancelFunc
}

// Start launches a goroutine that calls fn every period, starting after the
// first period elapses. Runs are due at fixed instants, start + k*period, and
// never overlap. A run that overruns, or a goroutine stalled past one or more
// due instants, does not drop the missed runs: every overdue run then follows
// the previous one back to back until the ticker catches up with the clock,
// so fn is called once per elapsed period and a callback that counts its
// calls stays locked to real time. A Stop between two overdue runs ends the
// catch-up. A panic inside fn is recovered and logged so one bad tick never
// stops later ticks or crashes the process. A zero-value logger disables
// logging. Callers must call Stop to release the goroutine.
func Start(period time.Duration, fn func(), log zerolog.Logger) *Ticker {
	return start(period, func(context.Context) { fn() }, statsFor(fn), log)
}

// StartContext is Start for a tick that can be cut short: every call of fn
// gets a context that Stop cancels, so a tick in flight when the ticker stops
// can end early instead of making StopAndWait wait out however long fn would
// otherwise take. fn decides what ending early means; the ticker only cancels.
func StartContext(period time.Duration, fn func(context.Context), log zerolog.Logger) *Ticker {
	return start(period, fn, statsFor(fn), log)
}

func start(period time.Duration, fn func(context.Context), stats *tickStat, log zerolog.Logger) *Ticker {
	ctx, cancel := context.WithCancel(context.Background())
	t := &Ticker{stop: make(chan struct{}), done: make(chan struct{}), cancel: cancel}
	go t.run(ctx, period, fn, log, stats)
	return t
}

func (t *Ticker) run(ctx context.Context, period time.Duration, fn func(context.Context), log zerolog.Logger, stats *tickStat) {
	defer close(t.done)

	// due is the instant the next run is owed. It advances by exactly one
	// period per run, never from the time a run ends, so an overrun leaves it
	// in the past and the owed runs follow at once.
	due := time.Now().Add(period)
	timer := time.NewTimer(period)
	defer timer.Stop()
	for {
		if wait := time.Until(due); wait > 0 {
			timer.Reset(wait)
			select {
			case <-t.stop:
				return
			case <-timer.C:
			}
		} else {
			select {
			case <-t.stop:
				return
			default:
			}
		}
		tick(ctx, fn, log, stats)
		due = due.Add(period)
	}
}

func tick(ctx context.Context, fn func(context.Context), log zerolog.Logger, stats *tickStat) {
	start := time.Now()
	defer func() {
		stats.observe(time.Since(start))
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("scheduler: recovered panic in ticked callback")
		}
	}()
	fn(ctx)
}

func tickerName(fn any) string {
	return path.Base(runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name())
}

// statsMu serializes the check-then-set in statsFor. expvar.Map's own Get
// and Set are each safe, but without this lock two concurrent Start calls
// for the same ticker name both allocate and both Set: the loser's ticker
// then writes into a *tickStat nothing publishes, and the winning Set
// rebinds the key to a zeroed struct, resetting the max high-water mark.
var statsMu sync.Mutex

func statsFor(fn any) *tickStat {
	name := tickerName(fn)
	statsMu.Lock()
	defer statsMu.Unlock()
	if v := tickerVars.Get(name); v != nil {
		if s, ok := v.(*tickStat); ok {
			return s
		}
	}
	s := &tickStat{}
	tickerVars.Set(name, s)
	return s
}

type tickStat struct {
	mu   sync.Mutex
	last int64
	max  int64
}

func (s *tickStat) observe(d time.Duration) {
	ns := d.Nanoseconds()
	s.mu.Lock()
	s.last = ns
	if ns > s.max {
		s.max = ns
	}
	s.mu.Unlock()
}

func (s *tickStat) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf(`{"last":%d,"max":%d}`, s.last, s.max)
}

// Stop requests that future ticks halt and cancels the context a tick started
// by StartContext is running under. Safe to call more than once.
func (t *Ticker) Stop() {
	t.stopOnce.Do(func() {
		close(t.stop)
		t.cancel()
	})
}

// StopAndWait requests that future ticks halt and waits for any current tick.
// A StartContext tick sees its context canceled first, so the wait lasts only
// as long as that tick takes to notice.
func (t *Ticker) StopAndWait() {
	t.Stop()
	<-t.done
}
