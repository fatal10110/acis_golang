package engine

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// flyEngine loads one region whose first block has its floor at 0 on cell
// columns 0-3 and a 200-high floor (a wall to a low flier) on columns 4-7.
// Every other block of the region is null geodata, as is the rest of the
// world.
func flyEngine(t *testing.T) *Engine {
	t.Helper()
	e := New()
	region, err := block.NewRegionFromBlocks([]block.Block{complexBlock(func(x, _ int) block.Cell {
		if x >= 4 {
			return block.Cell{Height: 200, NSWE: block.AllDirections}
		}
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	})})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}
	return e
}

// Expected values are traced by hand through GeoEngine.canFly and
// GeoEngine.getValidFlyLocation (GeoEngine.java:1386-1666) with an oheight
// of 32, the corridor PlayerMove.calculatePath asks for: the line's height at
// each crossed cell border is oz + (int)(mz * distance from the origin), the
// highest layer under that plus 32 must not rise above it, and the lowest
// layer over it must not sink under it plus 32. A stop lands on the border of
// the cell that failed, at the height of the last border cleared (oz before
// the first).
func TestFlyQueries(t *testing.T) {
	e := flyEngine(t)
	x0, y0 := WorldXMin, WorldYMin
	from := location.Location{X: x0 + 8, Y: y0 + 8, Z: 100}
	tests := []struct {
		name   string
		from   location.Location
		to     location.Location
		canFly bool
		stop   location.Location
	}{
		{
			name:   "level line over the low floor",
			from:   from,
			to:     location.Location{X: x0 + 56, Y: y0 + 8, Z: 100},
			canFly: true,
			stop:   location.Location{X: x0 + 56, Y: y0 + 8, Z: 100},
		},
		{
			// Column 4's floor (200) lies above the line (100): the highest
			// layer under 132 is missing, so the flier stops on its border.
			name: "level line into the high floor",
			from: from,
			to:   location.Location{X: x0 + 104, Y: y0 + 8, Z: 100},
			stop: location.Location{X: x0 + 63, Y: y0 + 8, Z: 100},
		},
		{
			name:   "level line over the high floor",
			from:   location.Location{X: x0 + 8, Y: y0 + 8, Z: 250},
			to:     location.Location{X: x0 + 120, Y: y0 + 8, Z: 250},
			canFly: true,
			stop:   location.Location{X: x0 + 120, Y: y0 + 8, Z: 250},
		},
		{
			// mz = -80/48: borders at 7, 23 and 39 units sit at 40-11=29,
			// 40-38=2 and 40-65=-25; the floor (0) is above the last.
			name: "descent into the floor",
			from: location.Location{X: x0 + 8, Y: y0 + 8, Z: 40},
			to:   location.Location{X: x0 + 56, Y: y0 + 8, Z: -40},
			stop: location.Location{X: x0 + 47, Y: y0 + 8, Z: 2},
		},
		{
			// Null geodata answers every layer query with a 0-high layer, so
			// a line above 0 always meets a "ceiling" below its top.
			name: "null geodata",
			from: location.Location{X: x0 + 8, Y: y0 + 8 + 128, Z: 100},
			to:   location.Location{X: x0 + 56, Y: y0 + 8 + 128, Z: 100},
			stop: location.Location{X: x0 + 15, Y: y0 + 8 + 128, Z: 100},
		},
		{
			// Westward out of the geodata grid: the stop is the grid border at
			// the origin cell's floor height.
			name: "out of the grid",
			from: from,
			to:   location.Location{X: x0 - 92, Y: y0 + 8, Z: 100},
			stop: location.Location{X: x0, Y: y0 + 8, Z: 0},
		},
		{
			name:   "same cell",
			from:   from,
			to:     location.Location{X: x0 + 12, Y: y0 + 3, Z: -500},
			canFly: true,
			stop:   location.Location{X: x0 + 12, Y: y0 + 3, Z: -500},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.CanFly(tt.from.X, tt.from.Y, tt.from.Z, 32, tt.to.X, tt.to.Y, tt.to.Z); got != tt.canFly {
				t.Errorf("CanFly() = %v, want %v", got, tt.canFly)
			}
			if got := e.ValidFlyLocation(tt.from.X, tt.from.Y, tt.from.Z, 32, tt.to.X, tt.to.Y, tt.to.Z); got != tt.stop {
				t.Errorf("ValidFlyLocation() = %+v, want %+v", got, tt.stop)
			}
		})
	}
}
