package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

func TestLivePlayerMarkDetaching(t *testing.T) {
	live := queuedTestLive(t, 1)
	sim.RunOwned(live.Queue(), live.markDetaching)
	if !live.detached() {
		t.Fatal("delivery remains enabled after detachment")
	}
}

// TestOnLiveRunsAsOwnerOnAClosedQueue: once a player's queue refuses posts
// (detached, or the pool stopping), onLive still runs the work as that
// queue's owner, so detach's owner-only writes do not panic.
func TestOnLiveRunsAsOwnerOnAClosedQueue(t *testing.T) {
	live := queuedTestLive(t, 1)
	live.Queue().Close()
	if !onLive(live, live.markDetaching) {
		t.Fatal("onLive reported a failed run on a closed queue")
	}
	if !live.detached() {
		t.Fatal("detach work did not run on a closed queue")
	}
}
