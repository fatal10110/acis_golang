package scheduler

import (
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
