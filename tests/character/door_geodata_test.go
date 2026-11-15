package character

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/reader"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// gludioStationStart is a point inside Gludio castle's station, just short
// of its closed door (gludio_castle_station, 19210008).
var gludioStationStart = location.Location{X: -19032, Y: 108072, Z: -2032}

// bootAtClosedGludioStationDoor boots a player on the shipped geodata of
// the region holding Gludio castle's station door, with that door closed,
// and places the player at gludioStationStart. It needs the datapack
// geodata, which the repository and CI do not have, so its callers run
// locally (they skip otherwise).
func bootAtClosedGludioStationDoor(t *testing.T) (*gameservertest.Server, int32) {
	t.Helper()
	const doorID = 19210008
	doors, err := xml.LoadDoors(datapack.Path(t, "data", "xml", "doors.xml"), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadDoors: %v", err)
	}
	tmpl, ok := doors.Get(doorID)
	if !ok {
		t.Fatalf("door %d missing", doorID)
	}
	tileX := engine.TileXMin + (tmpl.Position.X-engine.WorldXMin)/engine.TileSize
	tileY := engine.TileYMin + (tmpl.Position.Y-engine.WorldYMin)/engine.TileSize
	path := filepath.Join(datapack.Geodata(t), fmt.Sprintf("%d_%d_conv.dat", tileX, tileY))
	region, _, err := reader.ReadL2OFF(path)
	if err != nil {
		t.Fatalf("ReadL2OFF(%s): %v", path, err)
	}
	geo := engine.New()
	if err := geo.SetRegion(tileX, tileY, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
	shape, err := dynamic.NewDoorObject(tmpl, geo)
	if err != nil {
		t.Fatalf("NewDoorObject: %v", err)
	}
	geo.AddObject(shape)

	srv, character, objID := bootInZones(t, zone.NewIndex(), gameservertest.WithGeo(move.NewGeo(geo, nil)))
	start := gludioStationStart
	character.TeleportTo(start.X, start.Y, start.Z, 0)
	if got := teleportDestination(t, readUntilQuiet(srv.Client), objID); got != start {
		t.Fatalf("teleport went to %+v, want %+v", got, start)
	}
	appear(t, srv.Client)
	readUntilQuiet(srv.Client)
	return srv, objID
}

// walkFromGludioStation clicks target from gludioStationStart and returns
// where the walk ends.
func walkFromGludioStation(t *testing.T, srv *gameservertest.Server, target location.Location) location.Location {
	t.Helper()
	start := gludioStationStart
	srv.Client.Send(encodeMoveBackwardToLocation(target, start, 1))
	expectGroundClickAck(t, srv.Client)
	frame := srv.Client.Read()
	if frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	_, dest, origin := gameservertest.ReadMoveToLocationCoords(t, frame)
	if origin != start {
		t.Fatalf("walk origin = %+v, want %+v", origin, start)
	}
	return dest
}

// TestWalkToLowerFloorStopsAtClosedCastleDoor walks a player toward a point
// beyond Gludio castle's closed station door on the same floor, where the
// cell past the door has a lower floor too. With no route search, the walk
// ends on the last point a straight walk reaches: the door, not the lower
// floor behind it.
func TestWalkToLowerFloorStopsAtClosedCastleDoor(t *testing.T) {
	t.Parallel()
	srv, _ := bootAtClosedGludioStationDoor(t)
	target := location.Location{X: -18952, Y: 108072, Z: -2032}
	if got, want := walkFromGludioStation(t, srv, target), (location.Location{X: -19009, Y: 108072, Z: -2032}); got != want {
		t.Fatalf("walk toward %+v ends at %+v, want the door at %+v", target, got, want)
	}
}

// TestWalkToLowerFloorTargetBehindClosedCastleDoorIsRefused clicks a point on
// the lower floor behind Gludio castle's closed station door: the walk
// stops at the door on the station's floor and never reaches the target.
func TestWalkToLowerFloorTargetBehindClosedCastleDoorIsRefused(t *testing.T) {
	t.Parallel()
	srv, _ := bootAtClosedGludioStationDoor(t)
	target := location.Location{X: -18952, Y: 108072, Z: -2560}
	if got, want := walkFromGludioStation(t, srv, target), (location.Location{X: -19009, Y: 108072, Z: -2032}); got != want {
		t.Fatalf("walk toward %+v ends at %+v, want the door at %+v on the station floor", target, got, want)
	}
}
