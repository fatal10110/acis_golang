package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMovementUpdatesWorldState drives a walk over the real wire protocol
// and requires the resulting position to become observable in world state,
// not just in the client's own packet stream.
func TestMovementUpdatesWorldState(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)

	target := location.Location{X: 80, Y: 70, Z: 30}
	spawn := location.Location{X: 10, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(target, spawn, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	waitForWorldPosition(t, srv.State, objID, target)
}

func TestSwimmingMovementCapsPositionAtWaterSurface(t *testing.T) {
	form, err := zone.NewCuboid(-1_000, 1_000, -1_000, 1_000, -1_000, 150)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, form))
	if _, ok := zone.FindAt[*zone.Water](zones, 10, 20, 30); !ok {
		t.Fatal("water zone missing at player spawn")
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	)
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)
	target := location.Location{X: 300, Y: 200, Z: 200}
	c.Send(encodeMoveBackwardToLocation(target, target, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	waitForWorldPosition(t, srv.State, objID, location.Location{X: target.X, Y: target.Y, Z: 150})
}

// TestMoveBackwardToLocationRejectsBeyond9900Units pins
// MoveBackwardToLocation.java:109-114: a target farther than 9900 units
// from the packet's own origin is rejected with ActionFailed instead of
// starting a walk, regardless of how far the server-authoritative position
// actually is from either coordinate.
func TestMoveBackwardToLocationRejectsBeyond9900Units(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)

	origin := location.Location{X: 0, Y: 0, Z: 0}
	target := location.Location{X: 10000, Y: 0, Z: 0} // 10000 > 9900 cap
	c.Send(encodeMoveBackwardToLocation(target, origin, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}

// TestBlockedWalkBroadcastsSameCellMoveToLocation pins the player MOVE_TO
// blocked-arrival branch: observers get MoveToLocation to the cell the
// walk actually stopped on, not StopMove.
func TestBlockedWalkBroadcastsSameCellMoveToLocation(t *testing.T) {
	geo := &gameservertest.GateGeo{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(geo),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read()
	c.Read()
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)

	spawn := location.Location{X: 10, Y: 20, Z: 30}
	target := location.Location{X: 80, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(target, spawn, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}

	advanced := srv.TickPlayerBlocked(t, objID, geo)
	frame := c.Read()
	if frame[0] == serverpackets.OpcodeStopMove {
		t.Fatalf("blocked arrival opcode = StopMove (%#x), want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	if frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("blocked arrival opcode = %#x, want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	objectID, dest, origin := gameservertest.ReadMoveToLocationCoords(t, frame)
	if objectID != objID {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, objID)
	}
	if dest != advanced || origin != advanced {
		t.Fatalf("MoveToLocation dest/origin = %+v/%+v, want advanced cell %+v", dest, origin, advanced)
	}
}
