package engine_test

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
)

// TestFlyThroughRoofOnShippedRegion flies over every cell of the shipped
// region 22_22 whose top layer stands 200 to 600 over the next one: from 300
// over the top layer down to 40 over the lower one, inside the cell and into
// the cell east of it. The reference allows all of them inside the cell and
// nearly all into the next one; here a line that crosses a layer while it
// sinks inside a cell is refused, an owner-approved divergence (#4846).
// Inside the cell every line
// crosses the top layer. Into the next cell a line with a small gap still
// crosses the border over the top layer, and is refused only when the east
// cell has a layer it would sink through. It needs the datapack geodata,
// which the repository and CI do not have.
func TestFlyThroughRoofOnShippedRegion(t *testing.T) {
	tile := [2]int{22, 22}
	region := readRegion(t, tile)
	if region == nil {
		t.Fatal("datapack ships no geodata for region 22_22")
	}
	e := engine.New()
	if err := e.SetRegion(tile[0], tile[1], region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
	geoX0 := (tile[0] - engine.TileXMin) * regionGeoCells
	geoY0 := (tile[1] - engine.TileYMin) * regionGeoCells
	world := func(geo, worldMin int) int { return geo<<4 + worldMin + 8 }

	sampled, sameCell, nextCell := 0, 0, 0
	for gx := range regionGeoCells {
		for gy := range regionGeoCells {
			bx, by := gx/block.CellsX, gy/block.CellsY
			cx, cy := gx%block.CellsX, gy%block.CellsY
			if !region.HasGeodata(bx, by) {
				continue
			}
			topLayer, top := region.BelowCell(bx, by, cx, cy, math.MaxInt32)
			if topLayer == -1 {
				continue
			}
			lowerLayer, lower := region.BelowCell(bx, by, cx, cy, int32(top.Height))
			if lowerLayer == -1 {
				continue
			}
			if gap := int(top.Height) - int(lower.Height); gap < 200 || gap > 600 {
				continue
			}
			sampled++
			ox, oy := world(geoX0+gx, engine.WorldXMin), world(geoY0+gy, engine.WorldYMin)
			oz, tz := int(top.Height)+300, int(lower.Height)+40
			if !e.CanFly(ox, oy, oz, 32, ox+3, oy+2, tz) {
				sameCell++
			}
			if !e.CanFly(ox, oy, oz, 32, ox+16, oy, tz) {
				nextCell++
			}
		}
	}
	t.Logf("cells sampled: %d; refused in the same cell: %d; refused into the next cell: %d", sampled, sameCell, nextCell)
	if sampled != 51771 || sameCell != sampled || nextCell != 46059 {
		t.Errorf("sampled %d, refused %d in the same cell and %d into the next cell; want 51771, all and 46059", sampled, sameCell, nextCell)
	}
}
