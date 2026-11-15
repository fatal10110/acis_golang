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

// TestWalkToLowerFloorStopsAtClosedCastleDoor walks a player on the shipped
// geodata from inside Gludio castle's station toward a point beyond its
// closed door (gludio_castle_station), where the cell past the door has a
// lower floor too. With no route search, the walk ends on the last point a
// straight walk reaches: the door, not the lower floor behind it. It needs
// the datapack geodata, which the repository and CI do not have, so it runs
// locally (it skips otherwise).
func TestWalkToLowerFloorStopsAtClosedCastleDoor(t *testing.T) {
	t.Parallel()
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
	start := location.Location{X: -19032, Y: 108072, Z: -2032}
	character.TeleportTo(start.X, start.Y, start.Z, 0)
	if got := teleportDestination(t, readUntilQuiet(srv.Client), objID); got != start {
		t.Fatalf("teleport went to %+v, want %+v", got, start)
	}
	appear(t, srv.Client)
	readUntilQuiet(srv.Client)

	target := location.Location{X: -18952, Y: 108072, Z: -2032}
	srv.Client.Send(encodeMoveBackwardToLocation(target, start, 1))
	expectGroundClickAck(t, srv.Client)
	frame := srv.Client.Read()
	if frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	_, dest, origin := gameservertest.ReadMoveToLocationCoords(t, frame)
	if want := (location.Location{X: -19009, Y: 108072, Z: -2032}); dest != want {
		t.Fatalf("walk toward %+v ends at %+v, want the door at %+v", target, dest, want)
	}
	if origin != start {
		t.Fatalf("walk origin = %+v, want %+v", origin, start)
	}
}
