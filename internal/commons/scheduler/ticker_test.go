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
