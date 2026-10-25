package character

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// sitWalkTarget is far enough from spawnOrigin that a walk toward it is
// still under way when the sit request arrives: the virtual clock does not
// move on its own.
var sitWalkTarget = location.Location{X: 810, Y: 20, Z: 30}

// TestSitRequestStopsMoveFirst pins PlayerAI.thinkSit (PlayerAI.java:465-487):
// an accepted sit runs CreatureMove.stop (CreatureMove.java:452-463) before
// sitDown, so a following player is answered StopMove ahead of
// ChangeWaitType, on its own client and on every observer's, and is no
// longer moving or following afterwards. A player standing still has
// nothing to stop. A sit during a walk to a point waits for the arrival
// instead (TestSitRequestDuringWalkRunsOnArrival).
func TestSitRequestStopsMoveFirst(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(t *testing.T, srv *gameservertest.Server, c, observer *testsupport.ScriptedClient, observerID int32)
		want  []byte
	}{
		{
			name: "following",
			setup: func(t *testing.T, _ *gameservertest.Server, c, observer *testsupport.ScriptedClient, observerID int32) {
				click := encodeAction(observerID, int32(spawnOrigin.X), int32(spawnOrigin.Y), int32(spawnOrigin.Z), false)
				c.Send(click)
				mustReadOpcode(t, c, serverpackets.OpcodeMyTargetSelected, "observer selected")
				c.Send(click)
				syncSitClient(t, c)
				drainQuiet(t, observer)
			},
			want: []byte{serverpackets.OpcodeStopMove, serverpackets.OpcodeChangeWaitType},
		},
		{
			name: "standing still",
			want: []byte{serverpackets.OpcodeChangeWaitType},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, observer, objID, observerID := bootSitObserverPair(t)
			if tc.setup != nil {
				tc.setup(t, srv, c, observer, observerID)
			}
			if tc.name == "following" && !srv.PlayerMove(t, objID).Following() {
				t.Fatal("second click on the observer started no follow")
			}

			own := testsupport.SyncBarrierFrames(t, c, func() {
				c.Send(encodeRequestChangeWaitType(false))
				c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
			}, serverpackets.OpcodeItemList)
			if got := sitResponseOpcodes(own); !slices.Equal(got, tc.want) {
				t.Fatalf("sitter frames = %#x, want %#x", got, tc.want)
			}
			seen := readObserverUntil(t, observer, tc.want[len(tc.want)-1])
			if got := sitResponseOpcodes(seen); !slices.Equal(got, tc.want) {
				t.Fatalf("observer frames = %#x, want %#x", got, tc.want)
			}
			for _, frame := range seen {
				if frame[0] == serverpackets.OpcodeStopMove {
					if id := wire.NewReader(frame[1:]).ReadInt32(); id != objID {
						t.Fatalf("observer StopMove object = %d, want sitter %d", id, objID)
					}
				}
			}
			mover := srv.PlayerMove(t, objID)
			if mover.Moving() || mover.Following() {
				t.Fatalf("after the sit Moving=%v Following=%v, want both false", mover.Moving(), mover.Following())
			}
		})
	}
}

// bootSitObserverPair enters the sitter and a second player at the shared
// spawn, so the second one observes every broadcast of the first.
func bootSitObserverPair(t *testing.T) (srv *gameservertest.Server, c, observer *testsupport.ScriptedClient, objID, observerID int32) {
	t.Helper()
	srv = gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c = srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	objID = srv.SoleObjectID(t)

	observerID = srv.SeedCharacterFor(t, "player2", "Second", 1, 0).ID
	observer = srv.DialClient(t, "player2", 1)
	enterWorld(t, observer)
	drainQuiet(t, observer)
	drainQuiet(t, c)
	return srv, c, observer, objID, observerID
}

// startSitWalk starts the sitter's walk toward sitWalkTarget and drains both
// clients past its MoveToLocation.
func startSitWalk(t *testing.T, c, observer *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeMoveBackwardToLocation(sitWalkTarget, spawnOrigin, 1))
	expectGroundClickAck(t, c)
	mustReadOpcode(t, c, serverpackets.OpcodeMoveToLocation, "sitter walk")
	mustReadOpcode(t, observer, serverpackets.OpcodeMoveToLocation, "observer sees the walk")
	drainQuiet(t, c)
	drainQuiet(t, observer)
}

// syncSitClient waits until the server has handled everything c sent.
func syncSitClient(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList)
}

// readObserverUntil reads observer frames up to and including the first
// frame with opcode last.
func readObserverUntil(t *testing.T, observer *testsupport.ScriptedClient, last byte) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 20 {
		frame := observer.ReadWithTimeout(5 * time.Second)
		if frame == nil {
			t.Fatalf("observer frames %#x: no %#x within 5s", sitResponseOpcodes(frames), last)
		}
		frames = append(frames, frame)
		if frame[0] == last {
			return frames
		}
	}
	t.Fatalf("observer got no %#x within 20 frames", last)
	return nil
}

// sitResponseOpcodes keeps the opcodes a sit request can answer with.
func sitResponseOpcodes(frames [][]byte) []byte {
	var got []byte
	for _, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodeStopMove, serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeChairSit, serverpackets.OpcodeActionFailed:
			got = append(got, frame[0])
		}
	}
	return got
}
