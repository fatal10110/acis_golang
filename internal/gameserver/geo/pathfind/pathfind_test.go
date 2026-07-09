package pathfind

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFind(t *testing.T) {
	t.Run("reachable path returns corner points", func(t *testing.T) {
		finder := New(newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		})), DefaultOptions())

		path, cost, ok := finder.Find(at(0, 0, 0), at(3, 0, 0))
		if !ok {
			t.Fatal("Find() = no path, want path")
		}
		if len(path) == 0 {
			t.Fatal("Find() returned empty path, want target point")
		}
		if got := path[len(path)-1]; got != at(3, 0, 0) {
			t.Fatalf("Find() last = %#v, want %#v", got, at(3, 0, 0))
		}
		if cost <= 0 {
			t.Fatalf("Find() cost = %d, want a positive cost for a 3-cell path", cost)
		}
	})

	t.Run("blocked path returns no path", func(t *testing.T) {
		finder := New(newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			switch {
			case x == 1:
				return block.Cell{Height: 0, NSWE: block.NoDirections}
			default:
				return block.Cell{Height: 0, NSWE: block.AllDirections}
			}
		})), DefaultOptions())

		path, _, ok := finder.Find(at(0, 0, 0), at(2, 0, 0))
		if ok {
			t.Fatalf("Find() = %#v, want no path", path)
		}
		if len(path) != 0 {
			t.Fatalf("Find() len = %d, want 0", len(path))
		}
	})

	t.Run("weighted path avoids obstacle cells", func(t *testing.T) {
		finder := New(newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			switch {
			case x == 1 && y == 0:
				return block.Cell{Height: 0, NSWE: block.East | block.West | block.South}
			case y == 1:
				return block.Cell{Height: 0, NSWE: block.AllDirections}
			default:
				return block.Cell{Height: 0, NSWE: block.AllDirections}
			}
		})), Options{
			MoveWeight:      10,
			MoveWeightDiag:  14,
			ObstacleWeight:  100,
			HeuristicWeight: 12,
			MaxIterations:   100,
		})

		path, _, ok := finder.Find(at(0, 0, 0), at(2, 0, 0))
		if !ok {
			t.Fatal("Find() = no path, want path")
		}
		if len(path) == 0 {
			t.Fatal("Find() returned empty path, want detour")
		}
		if got := path[len(path)-1]; got != at(2, 0, 0) {
			t.Fatalf("Find() last = %#v, want %#v", got, at(2, 0, 0))
		}
		for _, step := range path {
			if step == at(1, 0, 0) {
				t.Fatalf("Find() = %#v, want path that avoids weighted obstacle", path)
			}
		}
	})

	t.Run("iteration cap returns no path", func(t *testing.T) {
		finder := New(newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		})), Options{
			MoveWeight:      10,
			MoveWeightDiag:  14,
			ObstacleWeight:  30,
			HeuristicWeight: 12,
			MaxIterations:   1,
		})

		path, _, ok := finder.Find(at(0, 0, 0), at(7, 7, 0))
		if ok {
			t.Fatalf("Find() = %#v, want no path", path)
		}
	})
}

func TestOptionsFromProperties(t *testing.T) {
	props, err := config.ParseString(`
MoveWeight = 11
MoveWeightDiag = 15
ObstacleWeight = 31
HeuristicWeight = 13
MaxIterations = 1234
`)
	if err != nil {
		t.Fatalf("ParseString(): %v", err)
	}

	got, err := OptionsFromProperties(props)
	if err != nil {
		t.Fatalf("OptionsFromProperties(): %v", err)
	}

	want := Options{
		MoveWeight:      11,
		MoveWeightDiag:  15,
		ObstacleWeight:  31,
		HeuristicWeight: 13,
		MaxIterations:   1234,
	}
	if got != want {
		t.Fatalf("OptionsFromProperties() = %#v, want %#v", got, want)
	}
}

func TestOptionsFromPropertiesDefaultsAndErrors(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		props, err := config.ParseString("")
		if err != nil {
			t.Fatalf("ParseString(): %v", err)
		}

		got, err := OptionsFromProperties(props)
		if err != nil {
			t.Fatalf("OptionsFromProperties(): %v", err)
		}
		if got != DefaultOptions() {
			t.Fatalf("OptionsFromProperties() = %#v, want %#v", got, DefaultOptions())
		}
	})

	t.Run("invalid integer", func(t *testing.T) {
		props, err := config.ParseString("MoveWeight = nope\n")
		if err != nil {
			t.Fatalf("ParseString(): %v", err)
		}

		if _, err := OptionsFromProperties(props); err == nil {
			t.Fatal("OptionsFromProperties() error = nil, want error")
		}
	})
}

func newTestEngine(t *testing.T, first block.Block) *engine.Engine {
	t.Helper()

	e := engine.New()
	region, err := block.NewRegionFromBlocks([]block.Block{first})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks(): %v", err)
	}
	if err := e.SetRegion(engine.TileXMin, engine.TileYMin, region); err != nil {
		t.Fatalf("SetRegion(): %v", err)
	}
	return e
}

func complexBlock(cell func(x, y int) block.Cell) block.Block {
	var cells [block.CellCount]block.Cell
	for x := range block.CellsX {
		for y := range block.CellsY {
			cells[x*block.CellsY+y] = cell(x, y)
		}
	}
	return block.NewComplex(cells)
}

func at(geoX, geoY int, z int) location.Location {
	return location.Location{
		X: engine.WorldX(geoX),
		Y: engine.WorldY(geoY),
		Z: z,
	}
}
