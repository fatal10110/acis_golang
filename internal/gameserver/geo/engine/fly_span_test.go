package engine

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// roofedEngine is one multilayer block: every cell has a floor at 0, and the
// cells from column 4 on a roof at 400 over it.
func roofedEngine(t *testing.T) *Engine {
	t.Helper()
	var cells [block.CellCount][]block.Cell
	for x := range block.CellsX {
		for y := range block.CellsY {
			layers := []block.Cell{{Height: 0, NSWE: block.AllDirections}}
			if x >= 4 {
				layers = append(layers, block.Cell{Height: 400, NSWE: block.AllDirections})
			}
			cells[x*block.CellsY+y] = layers
		}
	}
	b, err := block.NewMultilayer(cells)
	if err != nil {
		t.Fatal(err)
	}
	return newTestEngine(t, b)
}

// The fly line also checks the span it climbs or sinks inside every cell,
// not only the borders it crosses: an owner-approved divergence
// from the reference, whose line ends anywhere in its last cell (#4846).
func TestD19W2ReproFlyThroughRoofSameCell(t *testing.T) {
	e := roofedEngine(t)
	ox, oy := worldX(5), worldY(1)
	tx, ty := ox+3, oy+2
	tests := []struct {
		name   string
		oz, tz int
		want   bool
	}{
		{"down through the roof", 500, 100, false},
		{"down through roof and floor", 500, -5000, false},
		{"down under the floor", 100, -5000, false},
		{"up through the roof", 100, 500, false},
		{"up to touch the roof", 100, 368, true},
		{"up into the roof", 100, 369, false},
		{"over the roof", 500, 700, true},
		{"down onto the roof", 500, 400, true},
		{"under the roof", 100, 200, true},
		{"down onto the floor", 200, 0, true},
		{"take off from the floor", 0, 200, true},
		{"sunk into the floor, climbing out", -3, 200, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.CanFly(ox, oy, tt.oz, 32, tx, ty, tt.tz); got != tt.want {
				t.Fatalf("CanFly(%d -> %d) = %v, want %v", tt.oz, tt.tz, got, tt.want)
			}
			want := location.Location{X: tx, Y: ty, Z: tt.tz}
			if !tt.want {
				want = location.Location{X: ox, Y: oy, Z: tt.oz}
			}
			if got := e.ValidFlyLocation(ox, oy, tt.oz, 32, tx, ty, tt.tz); got != want {
				t.Fatalf("ValidFlyLocation(%d -> %d) = %+v, want %+v", tt.oz, tt.tz, got, want)
			}
		})
	}
}

// A line crossing the roof inside any cell it walks, first, middle or last,
// is refused even though every border it crosses is clear (#4846,
// owner-approved divergence).
func TestD19W2ReproFlyThroughRoofAcrossCells(t *testing.T) {
	e := roofedEngine(t)
	y := worldY(1)

	// Above the roof of column 5 down to under the roof of column 6: the
	// line meets the border under the roof, so only the first cell's span
	// shows the crossing. The flight stops at the origin.
	origin := location.Location{X: worldX(5), Y: y, Z: 500}
	if e.CanFly(origin.X, origin.Y, origin.Z, 32, worldX(6), y, 100) {
		t.Fatal("CanFly down through the first cell's roof = true, want false")
	}
	if got := e.ValidFlyLocation(origin.X, origin.Y, origin.Z, 32, worldX(6), y, 100); got != origin {
		t.Fatalf("ValidFlyLocation down through the first cell's roof = %+v, want the origin %+v", got, origin)
	}
	under := location.Location{X: worldX(5), Y: y, Z: 100}
	if e.CanFly(under.X, under.Y, under.Z, 32, worldX(6), y, 700) {
		t.Fatal("CanFly up through the first cell's roof = true, want false")
	}

	// From the open column 0 high over the roofed column 4: the border
	// into column 4 is crossed over its roof, and the line sinks through
	// it inside the cell. The flight stops on that border.
	from := location.Location{X: worldX(0), Y: y, Z: 600}
	tx := worldX(4) + 7
	if e.CanFly(from.X, from.Y, from.Z, 32, tx, y, 350) {
		t.Fatal("CanFly down through the last cell's roof = true, want false")
	}
	// The border point is the last unit of column 3, at the line's height
	// where it crosses into column 4.
	want := location.Location{X: worldX(4) - 9, Y: y, Z: 407}
	if got := e.ValidFlyLocation(from.X, from.Y, from.Z, 32, tx, y, 350); got != want {
		t.Fatalf("ValidFlyLocation down through the last cell's roof = %+v, want the border into column 4 %+v", got, want)
	}

	// A steep line between roofed columns crosses the roof inside column
	// 5, a middle cell, both ways: borders into column 5 over the roof
	// sinking, under it climbing. Each flight stops on the border it
	// entered column 5 by, the last unit of the cell it came from.
	downFrom := location.Location{X: worldX(4), Y: y, Z: 600}
	if e.CanFly(downFrom.X, downFrom.Y, downFrom.Z, 32, worldX(7), y, 100) {
		t.Fatal("CanFly down through a middle cell's roof = true, want false")
	}
	want = location.Location{X: worldX(5) - 9, Y: y, Z: 528}
	if got := e.ValidFlyLocation(downFrom.X, downFrom.Y, downFrom.Z, 32, worldX(7), y, 100); got != want {
		t.Fatalf("ValidFlyLocation down through a middle cell's roof = %+v, want the border into column 5 %+v", got, want)
	}
	upFrom := location.Location{X: worldX(7), Y: y, Z: 100}
	if e.CanFly(upFrom.X, upFrom.Y, upFrom.Z, 32, worldX(4), y, 600) {
		t.Fatal("CanFly up through a middle cell's roof = true, want false")
	}
	want = location.Location{X: worldX(6) - 8, Y: y, Z: 350}
	if got := e.ValidFlyLocation(upFrom.X, upFrom.Y, upFrom.Z, 32, worldX(4), y, 600); got != want {
		t.Fatalf("ValidFlyLocation up through a middle cell's roof = %+v, want the border into column 5 %+v", got, want)
	}

	// Controls: the same lines kept over the roof, or under it from the
	// open side, stay clear.
	if !e.CanFly(from.X, from.Y, from.Z, 32, tx, y, 450) {
		t.Fatal("CanFly over the roof = false, want true")
	}
	if !e.CanFly(worldX(3), y, 100, 32, worldX(6), y, 100) {
		t.Fatal("CanFly level under the roof from the open side = false, want true")
	}
}

// The ground line keeps its floor comparison: the swimmer's guard against a
// target under the floor lives in the move layer (#4846), so CanMove still
// answers true here, in the same cell and across cells.
func TestD19W2ReproSwimBelowFloor(t *testing.T) {
	e := newTestEngine(t, block.NewFlat(0))
	ox, oy := worldX(1), worldY(1)
	if !e.CanMove(ox, oy, 30, ox+3, oy+2, -5000) {
		t.Fatal("same-cell CanMove to -5000 = false, want the ground answer true")
	}
	if e.Height(ox+3, oy+2, -5000) != 0 {
		t.Fatal("floor under the swimmer's target is not at 0")
	}
}
