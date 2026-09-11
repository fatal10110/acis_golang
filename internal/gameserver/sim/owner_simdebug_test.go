//go:build simdebug

package sim

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestAssertOwnerPanicsWhileAnotherGoroutineDrains(t *testing.T) {
	p := startPool(t, 2, zerolog.Nop())
	busy, other := p.NewQueue("busy"), p.NewQueue("other")
	entered, release := make(chan struct{}), make(chan struct{})
	busy.Post(func() {
		close(entered)
		<-release
	})
	<-entered
	defer close(release)

	if !panics(func() { AssertOwner(busy) }) {
		t.Fatal("AssertOwner did not panic from the test goroutine")
	}
	fromOther := make(chan bool)
	other.Post(func() { fromOther <- panics(func() { AssertOwner(busy) }) })
	if !<-fromOther {
		t.Fatal("AssertOwner did not panic from another queue's worker")
	}
}
