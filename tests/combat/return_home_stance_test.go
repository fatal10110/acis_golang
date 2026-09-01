package combat

import (
	"testing"

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
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", 20001, home, home)
	drainUntilQuiet(t, c)

	hostile.SetXYZ(hostileX, hostileY+500, hostileZ)
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside drift range")
	}

	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")
}

// TestSiegeGuardReturnHomeBroadcastsRunThenMove pins SiegeGuard.returnHome's
// forceRunStance before the home move when the guard was walking.
func TestSiegeGuardReturnHomeBroadcastsRunThenMove(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "SiegeGuard", 100, home, home)
	drainUntilQuiet(t, c)

	hostile.SetRunning(false)
	hostile.SetXYZ(hostileX, hostileY+50, hostileZ)
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside SiegeGuard drift range")
	}

	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), true)
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")
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
