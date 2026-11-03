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
// protection active, and returns the frames queued after the post-teleport
// UserInfo.
func enterAfterRestart(t *testing.T, srv *gameservertest.Server) [][]byte {
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
	// The restart request acted under the EnterWorld protection, which ends
	// once its handler has sent the teleport: notice, then UserInfo.
	if got := readProtectionMessage(t, c); got != "As you acted, you are no longer under spawn protection." {
		t.Fatalf("restart protection message = %q", got)
	}
	assertFrameOpcode(t, mustRead(t, c, "acted UserInfo"), serverpackets.OpcodeUserInfo, "acted UserInfo")
	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	assertFrameOpcode(t, mustRead(t, c, "post-teleport UserInfo"), serverpackets.OpcodeUserInfo, "post-teleport UserInfo")
	return c.ReadQueued()
}

const (
	townRestartX = 200
	townRestartY = 400
)

// readProtectionMessage scans frames until a string-parameter SystemMessage
// arrives and returns its text. Each read waits well past the protection
// windows the scenarios set, so a timer firing late on a loaded machine still
// lands.
func readProtectionMessage(t *testing.T, c *scriptedClient) string {
	t.Helper()
	for range 50 {
		frame := c.ReadWithTimeout(5 * time.Second)
		if frame == nil {
			t.Fatal("spawn-protection system message never arrived")
		}
		if text, ok := protectionText(frame); ok {
			return text
		}
	}
	t.Fatal("no string SystemMessage within 50 frames")
	return ""
}

// protectionText returns the text of frame if it is a string-parameter
// SystemMessage, the form the spawn-protection notices take.
func protectionText(frame []byte) (string, bool) {
	if frame[0] != serverpackets.OpcodeSystemMessage {
		return "", false
	}
	r := wireReader(frame[1:])
	if id := r.ReadInt32(); id != int32(serverpackets.SystemMessageS1) {
		return "", false
	}
	if params := r.ReadInt32(); params != 1 {
		return "", false
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		return "", false
	}
	return r.ReadString(), true
}

// TestSpawnProtectionEndsWhenPlayerActs pins the action-clear leg: after the
// teleport-completing Appearing activates protection, the next movement
// answers that acting lifted it.
func TestSpawnProtectionEndsWhenPlayerActs(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRestartPoints(restartTable()),
		gameservertest.WithSpawnProtection(1200*time.Millisecond),
	)
	// On the wall clock a loaded machine can take the whole window to finish
	// the restart, so the expiry may already be among the frames it left.
	got := ""
	for _, frame := range enterAfterRestart(t, srv) {
		if text, ok := protectionText(frame); ok {
			got = text
			break
		}
	}
	if got == "" {
		got = readProtectionMessage(t, srv.Client)
	}
	if got != "The spawn protection has ended." {
		t.Fatalf("protection message = %q", got)
	}
	drainUntilQuiet(t, srv.Client)
}
