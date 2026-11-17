package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMoveToLocationOnRealGeodataBroadcastsRequestedZ pins
// PlayerMove.moveToLocation on shipped geodata: when the straight walk is
// clear (calculatePath returns null) the broadcast destination is the click
// with its head-level Z, not the geodata floor under it. The blocked case,
// where the destination is replaced by the geodata's own valid location, is
// TestWalkToLowerFloorStopsAtClosedCastleDoor. Local only: it needs the
// shipped geodata.
func TestMoveToLocationOnRealGeodataBroadcastsRequestedZ(t *testing.T) {
	t.Parallel()
	srv, objID := bootAtClosedGludioStationDoor(t)
	character, _ := soleOnlineCharacter(t, srv)
	collision := int(character.CollisionHeight())
	start := gludioStationStart

	// Open ground on the station's own floor, short of the door.
	open := location.Location{X: start.X + 5, Y: start.Y, Z: start.Z}
	srv.Client.Send(encodeMoveBackwardToLocation(open, start, 1))
	expectGroundClickAck(t, srv.Client)
	frame := srv.Client.Read()
	if frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("open walk opcode = %#x, want MoveToLocation", frame[0])
	}
	id, dest, _ := gameservertest.ReadMoveToLocationCoords(t, frame)
	if want := (location.Location{X: open.X, Y: open.Y, Z: open.Z + collision}); id != objID || dest != want {
		t.Fatalf("open walk broadcast to %+v, want the clicked point at head height %+v", dest, want)
	}
}
