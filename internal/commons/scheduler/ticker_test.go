package scheduler

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestStopFromTickReturns(t *testing.T) {
	returned := make(chan struct{})
	ready := make(chan struct{})

	var ticker *Ticker
	ticker = Start(time.Millisecond, func() {
		<-ready
		ticker.Stop()
		close(returned)
	}, zerolog.Nop())
	close(ready)

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Stop called by a tick did not return")
	}
}

func TestStopAndWaitWaitsForTick(t *testing.T) {
	entered := make(chan struct{})
	finish := make(chan struct{})
	ticker := Start(time.Millisecond, func() {
		close(entered)
		<-finish
	}, zerolog.Nop())

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("tick did not start")
	}

	waiter, ok := any(ticker).(interface{ StopAndWait() })
	if !ok {
		t.Fatal("Ticker has no StopAndWait")
	}
	stopped := make(chan struct{})
	go func() {
		waiter.StopAndWait()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("StopAndWait returned before the tick finished")
	case <-time.After(10 * time.Millisecond):
	}
	close(finish)

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("StopAndWait did not return after the tick finished")
	}
}

func slowTick() {
	time.Sleep(2 * time.Millisecond)
}

func TestStartRecordsTickDuration(t *testing.T) {
	ticker := Start(time.Millisecond, slowTick, zerolog.Nop())
	defer ticker.StopAndWait()

	name := tickerName(slowTick)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		v := tickerVars.Get(name)
		s, ok := v.(*tickStat)
		if !ok {
			time.Sleep(time.Millisecond)
			continue
		}
		s.mu.Lock()
		last, max := s.last, s.max
		s.mu.Unlock()
		if last >= int64(2*time.Millisecond) && max >= last {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("ticker %q did not record last/max duration >= 2ms", name)
}

// statsFor's check-then-set spans two expvar.Map operations. Each is safe on
// its own, but without statsMu two concurrent Start calls for the same
// ticker name both allocate and both Set: one ticker then writes into a
// *tickStat nothing publishes, and the winning Set rebinds the key to a
// zeroed struct, resetting the max high-water mark. This is a lost update,
// not a data race, so -race alone does not catch it.
func TestStatsForIsSingleFlightPerTickerName(t *testing.T) {
	fn := func() {}
	name := tickerName(fn)

	for iter := 0; iter < 200; iter++ {
		tickerVars.Delete(name)

		const goroutines = 8
		got := make([]*tickStat, goroutines)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				got[i] = statsFor(fn)
			}(i)
		}
		close(start)
		wg.Wait()

		distinct := make(map[*tickStat]struct{}, goroutines)
		for _, s := range got {
			distinct[s] = struct{}{}
		}
		if len(distinct) != 1 {
			t.Fatalf("iteration %d: statsFor returned %d distinct *tickStat for ticker %q, want 1", iter, len(distinct), name)
		}
		if published := tickerVars.Get(name); published != got[0] {
			t.Fatalf("iteration %d: published expvar %p is not the *tickStat handed to callers %p", iter, published, got[0])
		}
	}
}

// An orphaned *tickStat is invisible in /debug/vars: durations observed on
// it never reach the published value. This pins the observable consequence.
func TestStatsForObservationsReachPublishedVar(t *testing.T) {
	fn := func() {}
	name := tickerName(fn)
	tickerVars.Delete(name)

	const goroutines = 8
	stats := make([]*tickStat, goroutines)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			stats[i] = statsFor(fn)
		}(i)
	}
	close(start)
	wg.Wait()

	for _, s := range stats {
		s.observe(42 * time.Millisecond)
	}

	published := tickerVars.Get(name)
	if published == nil {
		t.Fatal("ticker name not published")
	}
	want := fmt.Sprintf(`{"last":%d,"max":%d}`, (42 * time.Millisecond).Nanoseconds(), (42 * time.Millisecond).Nanoseconds())
	if got := published.String(); got != want {
		t.Fatalf("published expvar = %s, want %s (an orphaned tickStat swallowed the observations)", got, want)
	}
}
