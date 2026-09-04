package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBlockedArrivalBroadcastsSameCellMoveToLocation pins CreatureAI's
// arrived-blocked correction: observers get MoveToLocation to the actor's
// current cell, not StopMove.
func TestBlockedArrivalBroadcastsSameCellMoveToLocation(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	geo := &gameservertest.GateGeo{}
	hostile := srv.SpawnMovingHostileNPCAtGeo(t, "SiegeGuard", home, home, geo)
	drainUntilQuiet(t, c)

	stranded := location.Location{X: hostileX, Y: hostileY + 50, Z: hostileZ}
	hostile.SetRunning(false)
	hostile.SetXYZ(stranded.X, stranded.Y, stranded.Z)
	hostile.AI().SetWander()
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside SiegeGuard drift range")
	}
	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), true)
	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "start walk")

	geo.Block()
	if _, moving := hostile.Move().UpdatePosition(move.PositionUpdateInterval); moving {
		t.Fatal("UpdatePosition() moving = true after path closed, want blocked stop")
	}

	frame := mustRead(t, c, "blocked-arrival correction")
	if frame[0] == serverpackets.OpcodeStopMove {
		t.Fatalf("blocked arrival opcode = StopMove (%#x), want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	objectID, dest, origin := moveToLocationCoords(t, frame)
	if objectID != hostile.ObjectID() {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, hostile.ObjectID())
	}
	x, y, z := hostile.Position()
	at := location.Location{X: x, Y: y, Z: z}
	if dest != at || origin != at {
		t.Fatalf("MoveToLocation dest/origin = %+v/%+v, want current cell %+v", dest, origin, at)
	}
}

func moveToLocationCoords(t *testing.T, frame []byte) (objectID int32, dest, origin location.Location) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMoveToLocation, "MoveToLocation")
	r := wireReader(frame[1:])
	objectID = r.ReadInt32()
	dest = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	origin = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	return objectID, dest, origin
}
