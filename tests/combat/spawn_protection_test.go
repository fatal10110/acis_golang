package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// enterAfterRestart drives a dead player back into play through a restart
// point and the Appearing packet that completes the teleport, leaving spawn
// protection active on a quiet stream.
func enterAfterRestart(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, mustRead(t, c, "SSQInfo"), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, mustRead(t, c, "CharSelected"), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	// With a protection window configured, EnterWorld activates it and its
	// status snapshot (UserInfo) leads the reply burst.
	assertFrameOpcode(t, mustRead(t, c, "protection UserInfo"), serverpackets.OpcodeUserInfo, "protection UserInfo")
	readEnterWorldBurst(t, c)
	drainUntilQuiet(t, c)
	objID := srv.SoleObjectID(t)
	srv.MarkPlayerDead(t, objID)
	c.Send(encodeRequestRestartPoint(0))
	assertRestartTeleport(t, c, objID, townRestartPoint)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	assertFrameOpcode(t, mustRead(t, c, "post-teleport UserInfo"), serverpackets.OpcodeUserInfo, "post-teleport UserInfo")
	drainUntilQuiet(t, c)
}

const townRestartX = 200
const townRestartY = 400

// readProtectionMessage scans frames until a string-parameter SystemMessage
// arrives and returns its text.
func readProtectionMessage(t *testing.T, c *scriptedClient) string {
	t.Helper()
	for i := 0; i < 50; i++ {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			t.Fatal("spawn-protection system message never arrived")
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		if id := r.ReadInt32(); id != int32(serverpackets.SystemMessageS1) {
			continue
		}
		if params := r.ReadInt32(); params != 1 {
			continue
		}
		if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
			continue
		}
		return r.ReadString()
	}
	t.Fatal("no string SystemMessage within 50 frames")
	return ""
}

// TestSpawnProtectionEndsWhenPlayerActs pins the action-clear leg: after the
// teleport-completing Appearing activates protection, the next movement
// answers that acting lifted it.
func TestSpawnProtectionEndsWhenPlayerActs(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRestartPoints(restartTable()),
		gameservertest.WithSpawnProtection(5*time.Second),
	)
	enterAfterRestart(t, srv)

	srv.Client.Send(encodeMoveBackwardToLocation(townRestartX+10, townRestartY+10, -300))
	if got := readProtectionMessage(t, srv.Client); got != "As you acted, you are no longer under spawn protection." {
		t.Fatalf("protection message = %q", got)
	}
	drainUntilQuiet(t, srv.Client)
}

// TestSpawnProtectionExpires pins the timer leg: protection left untouched
// ends on its own with the ended announcement.
func TestSpawnProtectionExpires(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRestartPoints(restartTable()),
		gameservertest.WithSpawnProtection(1200*time.Millisecond),
	)
	enterAfterRestart(t, srv)

	if got := readProtectionMessage(t, srv.Client); got != "The spawn protection has ended." {
		t.Fatalf("protection message = %q", got)
	}
	drainUntilQuiet(t, srv.Client)
}
