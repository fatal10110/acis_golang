package engine

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestValidLocation(t *testing.T) {
	t.Run("clear route returns target", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		}))

		ox, oy, oz := worldX(0), worldY(0), 0
		tx, ty, tz := worldX(3), worldY(0), 0
		got := e.ValidLocation(ox, oy, oz, tx, ty, tz)
		want := location.Location{X: tx, Y: ty, Z: tz}
		if got != want {
			t.Fatalf("ValidLocation() = %+v, want %+v", got, want)
		}
	})

	t.Run("blocks at first closed edge returns border point", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			if x == 0 && y == 0 {
				return block.Cell{Height: 0, NSWE: block.West | block.North | block.South}
			}
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		}))

		ox, oy, oz := worldX(0), worldY(0), 0
		tx, ty, tz := worldX(3), worldY(0), 0
		got := e.ValidLocation(ox, oy, oz, tx, ty, tz)
		// The first iteration hits the East edge of cell (0,0) at gridX+15
		// (border offset for eastward walk), with checkY flat on the line.
		want := location.Location{X: ox + 7, Y: oy, Z: oz}
		if got != want {
			t.Fatalf("ValidLocation() = %+v, want %+v", got, want)
		}
	})

	t.Run("cliff step above ignore height returns last border point", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			if x == 3 && y == 0 {
				return block.Cell{Height: 100, NSWE: block.AllDirections}
			}
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		}))

		ox, oy, oz := worldX(0), worldY(0), 0
		tx, ty, tz := worldX(3), worldY(0), 0
		got := e.ValidLocation(ox, oy, oz, tx, ty, tz)
		// Three cells walk east open; the step into cell (3,0) has no layer
		// within CellIgnoreHeight of the origin floor, so the engine stops
		// at the border of (2,0)/(3,0): gridX + 15 from cell 2's origin.
		want := location.Location{X: worldX(2) + 7, Y: oy, Z: oz}
		if got != want {
			t.Fatalf("ValidLocation() = %+v, want %+v", got, want)
		}
	})

	t.Run("out of world target returns origin", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		}))

		ox, oy, oz := worldX(0), worldY(0), 0
		got := e.ValidLocation(ox, oy, oz, WorldXMin-1, oy, oz)
		want := location.Location{X: ox, Y: oy, Z: oz}
		if got != want {
			t.Fatalf("ValidLocation() = %+v, want %+v", got, want)
		}
	})
}
