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
// first period elapses. A panic inside fn is recovered and logged so one bad
// tick never stops later ticks or crashes the process. A zero-value logger
// disables logging. Callers must call Stop to release the goroutine.
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

	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-t.stop:
			return
		case <-ticker.C:
			tick(ctx, fn, log, stats)
		}
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
