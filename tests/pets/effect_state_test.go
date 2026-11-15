package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// effectInUse reports e.InUse() read on a's queue. An effect's in-use flag is
// owned by the queue of the actor it sits on, and an exit the owner's
// departure runs there reaches the test only through the client socket,
// which the race detector does not order, so a read from the test goroutine
// is unordered against it.
func effectInUse(t *testing.T, a *summon.Actor, e *effect.Effect) bool {
	t.Helper()
	got := make(chan bool, 1)
	if !a.Queue().Post(func() { got <- e.InUse() }) {
		t.Fatal("read an effect's state: the summon's queue is closed")
	}
	return <-got
}
