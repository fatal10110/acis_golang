package scheduler

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestTickerTicksAndRecoversPanics(t *testing.T) {
	ticks := make(chan struct{}, 10)
	var calls int32

	tk := Start(5*time.Millisecond, func() {
		n := atomic.AddInt32(&calls, 1)
		ticks <- struct{}{}
		if n == 2 {
			panic("boom")
		}
	}, zerolog.Nop())
	defer tk.Stop()

	for i := 0; i < 4; i++ {
		select {
		case <-ticks:
		case <-time.After(time.Second):
			t.Fatalf("tick %d did not fire in time", i+1)
		}
	}

	if got := atomic.LoadInt32(&calls); got < 4 {
		t.Fatalf("expected at least 4 calls, got %d", got)
	}
}

func TestTickerStopStopsFutureTicks(t *testing.T) {
	var calls int32
	tk := Start(5*time.Millisecond, func() {
		atomic.AddInt32(&calls, 1)
	}, zerolog.Nop())

	time.Sleep(20 * time.Millisecond)
	tk.Stop()
	tk.Stop() // Stop must be safe to call more than once.
	after := atomic.LoadInt32(&calls)

	time.Sleep(30 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != after {
		t.Fatalf("ticks continued after Stop: before=%d after=%d", after, got)
	}
}
