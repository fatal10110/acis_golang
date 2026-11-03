package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The emote requests below pin the SOCIAL reuse gate every RequestSocialAction
// spends before any other check: SocialTime per client, restarted by each
// request that passes the gate whatever happens to it next, and silent when
// it refuses.

const socialFloodDelay = 2 * time.Second

// bootSocialFlood boots the emoting player with SocialTime set and a second
// player in view of it.
func bootSocialFlood(t *testing.T) (srv *gameservertest.Server, c, observer *scriptedClient, objID, observerID int32) {
	t.Helper()
	srv, c, objID = bootSocialIdle(t, nil, gameservertest.WithSocialDelay(socialFloodDelay))
	if !srv.DrivesClock() {
		t.Skip("placing requests on either side of the reuse window needs the driven clock")
	}
	observerID = srv.SeedCharacterFor(t, "observer", "Observer", 5, 0).ID
	observer = srv.DialClient(t, "observer", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, c)
	return srv, c, observer, objID, observerID
}

// advanceClientTo lets the driven clock reach at, as the client reads it.
func advanceClientTo(t *testing.T, srv *gameservertest.Server, c *scriptedClient, at time.Time) {
	t.Helper()
	if d := at.Sub(c.Now()); d > 0 {
		srv.Advance(t, d)
	}
}

func TestEmoteFloodDroppedInsideSocialTime(t *testing.T) {
	t.Parallel()
	srv, c, observer, objID, _ := bootSocialFlood(t)

	start := c.Now()
	c.Send(encodeRequestSocialAction(2))
	assertEmoteShown(t, c, objID, 2, "first emote")
	assertEmoteShown(t, observer, objID, 2, "observer of the first emote")

	// Just short of SocialTime the request is still refused, with nothing
	// to the requester (assertEmoteDropped) or anyone watching.
	advanceClientTo(t, srv, c, start.Add(socialFloodDelay-10*time.Millisecond))
	assertEmoteDropped(t, c, "emote inside SocialTime")
	assertNoEmote(t, observer, "observer of a gated emote")

	// The refused request did not push the window out: the next request at
	// SocialTime after the accepted one goes through.
	advanceClientTo(t, srv, c, start.Add(socialFloodDelay))
	c.Send(encodeRequestSocialAction(3))
	assertEmoteShown(t, c, objID, 3, "emote once SocialTime has passed")
	assertEmoteShown(t, observer, objID, 3, "observer of the emote after SocialTime")
}

func TestEmoteRejectedPastTheGateStillStartsSocialTime(t *testing.T) {
	t.Parallel()
	srv, c, observer, objID, _ := bootSocialFlood(t)

	// Action id 1 is out of range: dropped silently, after the gate has
	// already been spent.
	start := c.Now()
	assertEmoteDroppedID(t, c, 1, "out-of-range emote")
	assertEmoteDropped(t, c, "valid emote inside the out-of-range one's SocialTime")
	assertNoEmote(t, observer, "observer of dropped emotes")

	advanceClientTo(t, srv, c, start.Add(socialFloodDelay))
	c.Send(encodeRequestSocialAction(4))
	assertEmoteShown(t, c, objID, 4, "emote once SocialTime has passed")
}

func TestEmoteSocialTimeIsPerClient(t *testing.T) {
	t.Parallel()
	_, c, observer, objID, observerID := bootSocialFlood(t)

	c.Send(encodeRequestSocialAction(2))
	assertEmoteShown(t, c, objID, 2, "first player's emote")
	assertEmoteShown(t, observer, objID, 2, "second player watching the first")

	observer.Send(encodeRequestSocialAction(5))
	assertEmoteShown(t, observer, observerID, 5, "second player's own emote inside the first's SocialTime")
	assertEmoteShown(t, c, observerID, 5, "first player watching the second")
}

// assertEmoteDroppedID is assertEmoteDropped for action id actionID.
func assertEmoteDroppedID(t *testing.T, c *scriptedClient, actionID int32, what string) {
	t.Helper()
	c.Send(encodeRequestSocialAction(actionID))
	assertNoEmote(t, c, what)
}

// assertNoEmote reads c until quiet and checks no SocialAction, ActionFailed
// or system message reached it.
func assertNoEmote(t *testing.T, c *scriptedClient, what string) {
	t.Helper()
	for _, frame := range readQuiet(c) {
		switch frame[0] {
		case serverpackets.OpcodeSocialAction, serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage:
			t.Fatalf("%s got opcode %#x, want nothing", what, frame[0])
		}
	}
}
