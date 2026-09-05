package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestReturnHomeBroadcastsWalkThenMove pins Attackable.returnHome's
// forceWalkStance before the home move: observers receive ChangeMoveType
// (walk) immediately followed by MoveToLocation.
func TestReturnHomeBroadcastsWalkThenMove(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	drainUntilQuiet(t, c)

	hostile.SetXYZ(hostileX, hostileY+500, hostileZ)
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside drift range")
	}

	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")
}

// TestSiegeGuardReturnHomeBroadcastsRunThenMove pins SiegeGuard.returnHome:
// forceRunStance immediately, then a MOVE_TO home desire that the next
// TickThink promotes and walks. Arrival at spawn idles (SiegeGuard does
// not idle-wander).
func TestSiegeGuardReturnHomeBroadcastsRunThenMove(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "SiegeGuard", home, home)
	drainUntilQuiet(t, c)

	hostile.SetRunning(false)
	hostile.SetXYZ(hostileX, hostileY+50, hostileZ)
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside SiegeGuard drift range")
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() after ReturnHome = %v, want idle until TickThink", got)
	}

	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), true)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after TickThink = %v, want move_to", got)
	}
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")

	// Production MoveToLocation snaps destination Z through geo.Height, so
	// arrival is not an exact Home identity. Drive the onEvtArrived path.
	hostile.SetXYZ(home.X, home.Y, home.Z-4)
	hostile.AI().Arrived()
	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() after arrival error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() after arrival = %v, want idle", got)
	}
	tickThinkIdle(t, hostile)
	// Periodic idle abort matches thinkIdle: leftover movement StopMove
	// and walk stance. SiegeGuard return-home had switched to run, so
	// observers then see ChangeMoveType(walk) and nothing else.
	assertFrameOpcode(t, mustRead(t, c, "idle StopMove"), serverpackets.OpcodeStopMove, "StopMove")
	assertChangeMoveType(t, mustRead(t, c, "idle walk stance"), hostile.ObjectID(), false)
	if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("unexpected frame after idle Think: opcode %#x", frame[0])
	}
}

func assertChangeMoveType(t *testing.T, frame []byte, objectID int32, running bool) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeChangeMoveType, "ChangeMoveType")
	r := wireReader(frame[1:])
	if got := r.ReadInt32(); got != objectID {
		t.Fatalf("ChangeMoveType object id = %d, want %d", got, objectID)
	}
	wantRun := int32(0)
	if running {
		wantRun = 1
	}
	if got := r.ReadInt32(); got != wantRun {
		t.Fatalf("ChangeMoveType running = %d, want %d", got, wantRun)
	}
	if got := r.ReadInt32(); got != 0 {
		t.Fatalf("ChangeMoveType swimming = %d, want 0", got)
	}
}
