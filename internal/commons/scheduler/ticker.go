// Package scheduler provides the fixed-rate ticker every periodic subsystem
// (aggro/attack-stance expiry, PvP flag decay, item respawn, ...) is built on
// top of.
package scheduler

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Ticker calls a callback on a fixed period, on its own goroutine, until
// Stop is called.
type Ticker struct {
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// Start launches a goroutine that calls fn every period, starting after the
// first period elapses. A panic inside fn is recovered and logged so one bad
// tick never stops later ticks or crashes the process. If log is nil,
// The zero logger disables logging. Callers must call Stop to release the
// goroutine.
func Start(period time.Duration, fn func(), log zerolog.Logger) *Ticker {

	t := &Ticker{stop: make(chan struct{}), done: make(chan struct{})}
	go t.run(period, fn, log)
	return t
}

func (t *Ticker) run(period time.Duration, fn func(), log zerolog.Logger) {
	defer close(t.done)

	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-t.stop:
			return
		case <-ticker.C:
			tick(fn, log)
		}
	}
}

func tick(fn func(), log zerolog.Logger) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("scheduler: recovered panic in ticked callback")
		}
	}()
	fn()
}

// Stop halts future ticks and blocks until any in-flight tick finishes. Safe
// to call more than once.
func (t *Ticker) Stop() {
	t.stopOnce.Do(func() {
		close(t.stop)
	})
	<-t.done
}
