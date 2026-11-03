package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPanicInQueuedHandlerClosesWithServerClose ends a session whose queued
// handler panicked: the client reads ServerClose as its last frame before the
// close, so it shows the disconnect instead of hanging on a dead connection.
func TestPanicInQueuedHandlerClosesWithServerClose(t *testing.T) {
	t.Parallel()
	stance := &panicOnStanceCheck{}
	srv := gameservertest.Boot(t,
		gameservertest.WithAttackStanceTracker(stance),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRealPool(), // sim.Inline does not recover task panics
	)
	c := srv.Client
	enterWorld(t, c)

	stance.armed.Store(true)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))

	for range 100 {
		if c.Read()[0] == serverpackets.OpcodeServerClose {
			c.ExpectClosed()
			return
		}
	}
	t.Fatal("no ServerClose within 100 frames of the panicked restart")
}
