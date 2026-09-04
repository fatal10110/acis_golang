package combat

import (
	"testing"

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
// forceRunStance immediately, then a MOVE_TO home desire that the next Think
// promotes and walks. Arrival at spawn idles (SiegeGuard does not idle-wander).
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
	hostile.AI().SetWander()
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside SiegeGuard drift range")
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() after ReturnHome = %v, want wander until Think", got)
	}

	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), true)
	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after Think = %v, want move_to", got)
	}
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")

	hostile.SetXYZ(home.X, home.Y, home.Z)
	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() after arrival error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() after arrival = %v, want idle", got)
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
