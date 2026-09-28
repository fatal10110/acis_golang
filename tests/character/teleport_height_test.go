package character

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// seaFloorGeo is open terrain whose ground lies at seaFloorZ everywhere. A
// clip, when set, is where every straight-line walk stops: its offset from
// the walk's origin.
type seaFloorGeo struct {
	gameservertest.Geo
	clip *location.Location
}

const seaFloorZ = -900

func (seaFloorGeo) Height(int, int, int) int16 { return seaFloorZ }

func (g seaFloorGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	if g.clip == nil {
		return location.Location{X: tx, Y: ty, Z: tz}
	}
	return location.Location{X: ox + g.clip.X, Y: oy + g.clip.Y, Z: oz}
}

// teleportDestination returns where the player's own TeleportToLocation
// sends it.
func teleportDestination(t *testing.T, frames [][]byte, objID int32) location.Location {
	t.Helper()
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeTeleportToLocation || int32(binary.LittleEndian.Uint32(f[1:5])) != objID {
			continue
		}
		return location.Location{
			X: int(int32(binary.LittleEndian.Uint32(f[5:9]))),
			Y: int(int32(binary.LittleEndian.Uint32(f[9:13]))),
			Z: int(int32(binary.LittleEndian.Uint32(f[13:17]))),
		}
	}
	t.Fatal("no TeleportToLocation for the player")
	return location.Location{}
}

// TestTeleportHeightSnapsOnlyOnDryGroundAndNotFlying pins the height rule of
// Creature.teleportTo (Creature.java:409-411): the destination Z snaps to
// geodata ground height only when the creature is not flying and the
// destination is not inside a water zone. A teleport into water keeps the
// requested height (the player swims there instead of landing on the sea
// floor), and so does a flying player's teleport onto dry land.
func TestTeleportHeightSnapsOnlyOnDryGroundAndNotFlying(t *testing.T) {
	form, err := zone.NewCuboid(-1_000, 1_000, -1_000, 1_000, -1_000, 150)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, form))
	srv, character, objID := bootInZones(t, zones, gameservertest.WithGeo(seaFloorGeo{}))
	x, y, z := srv.PlayerPosition(t, objID)

	dry := location.Location{X: x + 5_000, Y: y, Z: z}
	character.TeleportTo(dry.X, dry.Y, dry.Z, 0)
	if got, want := teleportDestination(t, readUntilQuiet(srv.Client), objID), (location.Location{X: dry.X, Y: dry.Y, Z: seaFloorZ}); got != want {
		t.Fatalf("teleport onto dry land went to %+v, want the ground height %+v", got, want)
	}
	appear(t, srv.Client)
	readUntilQuiet(srv.Client)

	water := location.Location{X: 100, Y: 200, Z: -120}
	character.TeleportTo(water.X, water.Y, water.Z, 0)
	if got := teleportDestination(t, readUntilQuiet(srv.Client), objID); got != water {
		t.Fatalf("teleport into water went to %+v, want the requested height %+v", got, water)
	}
	if px, py, pz := srv.PlayerPosition(t, objID); (location.Location{X: px, Y: py, Z: pz}) != water {
		t.Fatalf("world position after a teleport into water = %d,%d,%d, want %+v", px, py, pz, water)
	}
	appear(t, srv.Client)
	readUntilQuiet(srv.Client)

	character.SetFlying(true)
	dry.Y += 300
	character.TeleportTo(dry.X, dry.Y, dry.Z, 0)
	if got := teleportDestination(t, readUntilQuiet(srv.Client), objID); got != dry {
		t.Fatalf("flying teleport went to %+v, want the requested height %+v", got, dry)
	}
}

// TestTeleportScatterLandsOnLastReachablePoint pins the scatter of
// Creature.teleportTo (Creature.java:399-407): a scattered destination whose
// straight line from the requested point is blocked lands on the last point
// GeoEngine.getValidLocation reaches, not back on the requested point.
func TestTeleportScatterLandsOnLastReachablePoint(t *testing.T) {
	clip := location.Location{X: 7, Y: -5}
	geo := blockedGeo{seaFloorGeo{clip: &clip}}
	srv, character, objID := bootInZones(t, zone.NewIndex(), gameservertest.WithGeo(geo))
	x, y, z := srv.PlayerPosition(t, objID)

	target := location.Location{X: x + 3_000, Y: y, Z: z}
	character.TeleportTo(target.X, target.Y, target.Z, 20)
	want := location.Location{X: target.X + clip.X, Y: target.Y + clip.Y, Z: seaFloorZ}
	if got := teleportDestination(t, readUntilQuiet(srv.Client), objID); got != want {
		t.Fatalf("blocked scatter went to %+v, want the last reachable point %+v", got, want)
	}
}

// blockedGeo closes every straight-line walk, so only the clipped
// ValidLocation can move a scatter.
type blockedGeo struct{ seaFloorGeo }

func (blockedGeo) CanMove(int, int, int, int, int, int) bool { return false }
