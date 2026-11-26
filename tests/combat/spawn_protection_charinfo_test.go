package combat

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// charInfoTeamTail is how far the team byte sits from CharInfo's end:
// everything after it is fixed width (CharInfo.java:151-163), 43 bytes.
const charInfoTeamTail = 44

// charInfoTeam returns the object id and team byte of a CharInfo frame.
func charInfoTeam(frame []byte) (objectID int32, team byte) {
	return int32(binary.LittleEndian.Uint32(frame[17:21])), frame[len(frame)-charInfoTeamTail]
}

// enterProtected selects slot 0 and enters the world with a protection
// window configured, and returns every frame the login left, the
// protection UserInfo that follows the EtcStatusUpdate excluded.
func enterProtected(t *testing.T, c *scriptedClient) [][]byte {
	t.Helper()
	c.Send(clientwire.RequestGameStart(0))
	assertFrameOpcode(t, mustRead(t, c, "SSQInfo"), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, mustRead(t, c, "CharSelected"), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(clientwire.EnterWorld())
	var frames [][]byte
	protection := false
	for {
		frame := mustRead(t, c, "EnterWorld burst")
		if !protection && len(frames) > 0 && frames[len(frames)-1][0] == serverpackets.OpcodeEtcStatusUpdate {
			assertFrameOpcode(t, frame, serverpackets.OpcodeUserInfo, "protection UserInfo")
			protection = true
			continue
		}
		frames = append(frames, frame)
		if frame[0] == serverpackets.OpcodeActionFailed {
			break
		}
	}
	if !protection {
		t.Fatal("EnterWorld burst has no EtcStatusUpdate ahead of the protection UserInfo")
	}
	return append(frames, c.ReadQueued()...)
}

// teamsOf lists, in arrival order, the team byte of every CharInfo of
// objectID among frames.
func teamsOf(frames [][]byte, objectID int32) []byte {
	var teams []byte
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		if id, team := charInfoTeam(frame); id == objectID {
			teams = append(teams, team)
		}
	}
	return teams
}

// awaitCharInfoTeam reads c until a CharInfo of objectID with team arrives.
func awaitCharInfoTeam(t *testing.T, c *scriptedClient, objectID int32, team byte, what string) {
	t.Helper()
	for range 200 {
		frame := c.ReadWithTimeout(5 * time.Second)
		if frame == nil {
			break
		}
		if frame[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		if id, got := charInfoTeam(frame); id == objectID && got == team {
			return
		}
	}
	t.Fatalf("%s: no CharInfo of %d with team %d", what, objectID, team)
}

// protectedPair is two players in the same region under a spawn
// protection window: the subject, on the server's own client, entered
// first, then the observer.
type protectedPair struct {
	srv                   *gameservertest.Server
	observer              *scriptedClient
	subjectID, observerID int32
	// observerSaw is every frame the observer's login left.
	observerSaw [][]byte
}

// bootProtectedPair enters the pair. The subject sees the observer arrive
// in the blue team: the observer's protection is set before it spawns.
func bootProtectedPair(t *testing.T, window time.Duration) protectedPair {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRestartPoints(restartTable()),
		gameservertest.WithSpawnProtection(window),
	)
	p := protectedPair{srv: srv, subjectID: srv.SoleObjectID(t)}
	enterProtected(t, srv.Client)

	p.observerID = srv.SeedCharacterFor(t, "player2", "Watcher", 5, 0).ID
	p.observer = srv.DialClient(t, "player2", 1)
	p.observerSaw = enterProtected(t, p.observer)
	if teams := teamsOf(srv.Client.ReadQueued(), p.observerID); len(teams) != 1 || teams[0] != 1 {
		t.Fatalf("subject's CharInfo teams of the protected observer = %v, want [1]", teams)
	}
	return p
}

// TestSpawnProtectionCharInfoFollowsTeleportAndAct pins the observer side
// of Player.setSpawnProtection: each change ends in broadcastUserInfo, so
// the players around see the blue team marker come back when a teleport
// lands and drop when the protected player acts.
func TestSpawnProtectionCharInfoFollowsTeleportAndAct(t *testing.T) {
	t.Parallel()
	p := bootProtectedPair(t, time.Minute)
	srv, observer, subjectID := p.srv, p.observer, p.subjectID
	c := srv.Client
	if teams := teamsOf(p.observerSaw, subjectID); len(teams) != 1 || teams[0] != 1 {
		t.Fatalf("observer's CharInfo teams of the protected subject = %v, want [1]", teams)
	}

	// The restart request ends the login's protection once the teleport
	// has taken the subject out of everyone's view.
	srv.MarkPlayerDead(t, subjectID)
	c.Send(encodeRequestRestartPoint(0))
	assertRestartTeleport(t, c, subjectID, townRestartPoint)
	if got := readProtectionMessage(t, c); got != "As you acted, you are no longer under spawn protection." {
		t.Fatalf("restart protection message = %q", got)
	}
	observer.ReadQueued()

	c.Send(clientwire.SingleOpcode(clientpackets.OpcodeAppearing))
	assertFrameOpcode(t, mustRead(t, c, "post-teleport UserInfo"), serverpackets.OpcodeUserInfo, "post-teleport UserInfo")
	awaitCharInfoTeam(t, observer, subjectID, 1, "teleport landed")

	c.Send(encodeMoveBackwardToLocation(townRestartX+10, townRestartY+10, -300))
	if got := readProtectionMessage(t, c); got != "As you acted, you are no longer under spawn protection." {
		t.Fatalf("move protection message = %q", got)
	}
	awaitCharInfoTeam(t, observer, subjectID, 0, "acted")
}

// TestSpawnProtectionCharInfoFollowsExpiry pins the timer leg: protection
// running out shows the players around the protected player out of the
// blue team.
func TestSpawnProtectionCharInfoFollowsExpiry(t *testing.T) {
	t.Parallel()
	p := bootProtectedPair(t, 1500*time.Millisecond)
	awaitCharInfoTeam(t, p.srv.Client, p.observerID, 0, "expired")
}
