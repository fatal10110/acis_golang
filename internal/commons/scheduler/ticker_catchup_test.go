package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
)

// The tests here run in a synctest bubble: time is a fake clock that moves
// only when every goroutine in the bubble is blocked, so a stall of several
// periods is exact and costs no real time.

const catchUpPeriod = 20 * time.Millisecond

// A first run that stalls five periods leaves four runs owed. Fixed-rate
// scheduling runs them back to back once the stall ends, so 15.5 periods in
// fn has been called 15 times, once per elapsed period; dropping the missed
// runs would leave it at 11.
func TestStartRunsMissedPeriodsAfterAStall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		var runs atomic.Int32
		var mu sync.Mutex
		var at []time.Duration
		ticker := Start(catchUpPeriod, func() {
			mu.Lock()
			at = append(at, time.Since(start))
			mu.Unlock()
			if runs.Add(1) == 1 {
				time.Sleep(5 * catchUpPeriod)
			}
		}, zerolog.Nop())
		defer ticker.StopAndWait()

		time.Sleep(15*catchUpPeriod + catchUpPeriod/2)
		synctest.Wait()
		if got := runs.Load(); got != 15 {
			t.Fatalf("runs after 15.5 periods = %d, want 15", got)
		}
		mu.Lock()
		defer mu.Unlock()
		// The four owed runs land together when the stall ends, then the
		// schedule resumes on its original grid.
		want := []time.Duration{1, 6, 6, 6, 6, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
		for i, w := range want {
			if at[i] != w*catchUpPeriod {
				t.Fatalf("run %d at %v, want %v (runs at %v)", i+1, at[i], w*catchUpPeriod, at)
			}
		}
	})
}

// StartContext keeps the same fixed rate as Start.
func TestStartContextRunsMissedPeriodsAfterAStall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runs atomic.Int32
		ticker := StartContext(catchUpPeriod, func(context.Context) {
			if runs.Add(1) == 1 {
				time.Sleep(3 * catchUpPeriod)
			}
		}, zerolog.Nop())
		defer ticker.StopAndWait()

		time.Sleep(6*catchUpPeriod + catchUpPeriod/2)
		synctest.Wait()
		if got := runs.Load(); got != 6 {
			t.Fatalf("runs after 6.5 periods = %d, want 6", got)
		}
	})
}

// Runs never overlap: an owed run starts only once the one before it has
// returned, on the ticker's single goroutine.
func TestStartCatchUpRunsNeverOverlap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var running, overlaps, runs atomic.Int32
		ticker := Start(catchUpPeriod, func() {
			if running.Add(1) != 1 {
				overlaps.Add(1)
			}
			// Every run overruns by half a period, so the ticker is always
			// behind and every run after the first is a catch-up run.
			time.Sleep(catchUpPeriod + catchUpPeriod/2)
			running.Add(-1)
			runs.Add(1)
		}, zerolog.Nop())
		time.Sleep(20 * catchUpPeriod)
		ticker.StopAndWait()
		if got := overlaps.Load(); got != 0 {
			t.Fatalf("%d runs overlapped another", got)
		}
		if runs.Load() == 0 {
			t.Fatal("no run completed")
		}
	})
}

// A panicking run is recovered and still counts as the period's run: the
// owed runs behind it follow and the ticker keeps going.
func TestStartCatchUpSurvivesAPanickingRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runs atomic.Int32
		ticker := Start(catchUpPeriod, func() {
			if runs.Add(1) == 1 {
				time.Sleep(4 * catchUpPeriod)
				panic("stalled tick")
			}
		}, zerolog.Nop())
		defer ticker.StopAndWait()

		time.Sleep(8*catchUpPeriod + catchUpPeriod/2)
		synctest.Wait()
		if got := runs.Load(); got != 8 {
			t.Fatalf("runs after 8.5 periods = %d, want 8", got)
		}
	})
}

// A Stop while the ticker is behind ends the catch-up: StopAndWait waits
// for the run in flight only, not for the owed runs behind it.
func TestStopAndWaitSkipsOwedRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runs atomic.Int32
		ticker := Start(catchUpPeriod, func() {
			if runs.Add(1) == 1 {
				time.Sleep(10 * catchUpPeriod)
			}
		}, zerolog.Nop())

		time.Sleep(3 * catchUpPeriod)
		ticker.StopAndWait()
		if got := runs.Load(); got != 1 {
			t.Fatalf("runs = %d after stopping mid-stall, want 1 (the run in flight)", got)
		}
	})
}
