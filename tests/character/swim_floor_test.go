package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestSwimmerCannotTargetUnderTheFloor swims a player over a flat geodata
// floor at 0. A click under the floor is not walked to: the swim stops
// where the line meets the floor. A click in the water over the floor is.
// Refusing the target under the floor is an owner-approved divergence from
// the reference, which only caps a swim at the water surface (#4846).
func TestSwimmerCannotTargetUnderTheFloor(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(-1_000, 1_000, -1_000, 1_000, -1_000, 150)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, form))

	floor := block.NewRegionBuilder()
	for i := range block.RegionBlocksX * block.RegionBlocksY {
		floor.SetFlat(i, 0)
	}
	geo := engine.New()
	tileX := engine.TileXMin + (0-engine.WorldXMin)/engine.TileSize
	tileY := engine.TileYMin + (0-engine.WorldYMin)/engine.TileSize
	if err := geo.SetRegion(tileX, tileY, floor.Build()); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}

	srv, _, objID := bootInZones(t, zones, gameservertest.WithGeo(move.NewGeo(geo, nil)))
	c := srv.Client

	swim := func(target location.Location) location.Location {
		t.Helper()
		c.Send(encodeMoveBackwardToLocation(target, target, 1))
		expectGroundClickAck(t, c)
		frame := c.Read()
		if frame[0] != serverpackets.OpcodeMoveToLocation {
			t.Fatalf("swim opcode = %#x, want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
		}
		_, dest, _ := gameservertest.ReadMoveToLocationCoords(t, frame)
		waitForWorldPosition(t, srv, objID, dest)
		return dest
	}

	under := location.Location{X: 300, Y: 200, Z: -500}
	if dest := swim(under); dest == under || dest.Z < 0 {
		t.Fatalf("swim toward %+v under the floor ends at %+v, want a point at or over the floor at 0", under, dest)
	}
	over := location.Location{X: 300, Y: 200, Z: 100}
	if dest := swim(over); dest != over {
		t.Fatalf("swim toward %+v in the water ends at %+v, want the target", over, dest)
	}
}
