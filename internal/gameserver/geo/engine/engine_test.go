package engine

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

func TestCanMove(t *testing.T) {
	t.Run("allows clear step", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		}))

		if !e.CanMove(worldX(0), worldY(0), 0, worldX(1), worldY(0), 0) {
			t.Fatal("CanMove() = false, want true")
		}
	})

	t.Run("blocks closed nswe edge", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			if x == 0 && y == 0 {
				return block.Cell{Height: 0, NSWE: block.West | block.North | block.South}
			}
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		}))

		if e.CanMove(worldX(0), worldY(0), 0, worldX(1), worldY(0), 0) {
			t.Fatal("CanMove() = true, want false")
		}
	})

	t.Run("blocks excessive height jump", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			if x == 1 && y == 0 {
				return block.Cell{Height: 64, NSWE: block.AllDirections}
			}
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		}))

		if e.CanMove(worldX(0), worldY(0), 0, worldX(1), worldY(0), 64) {
			t.Fatal("CanMove() = true, want false")
		}
	})
}

func TestCanSee(t *testing.T) {
	t.Run("allows clear line", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		}))

		if !e.CanSee(worldX(0), worldY(0), 0, worldX(3), worldY(0), 0) {
			t.Fatal("CanSee() = false, want true")
		}
	})

	t.Run("blocks wall crossing", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			switch {
			case x == 0 && y == 0:
				return block.Cell{Height: 0, NSWE: block.West | block.North | block.South}
			case x == 1 && y == 0:
				return block.Cell{Height: 40, NSWE: block.AllDirections}
			default:
				return block.Cell{Height: 0, NSWE: block.AllDirections}
			}
		}))

		if e.CanSee(worldX(0), worldY(0), 0, worldX(2), worldY(0), 0) {
			t.Fatal("CanSee() = true, want false")
		}
	})

	t.Run("uses configured obstacle height", func(t *testing.T) {
		makeBlock := func(x, y int) block.Cell {
			if x == 1 && y == 0 {
				return block.Cell{Height: 40, NSWE: block.AllDirections}
			}
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		}

		if newTestEngine(t, complexBlock(makeBlock)).CanSee(worldX(0), worldY(0), 0, worldX(2), worldY(0), 0) {
			t.Fatal("default CanSee() = true over 40-height obstacle, want false")
		}

		e := newTestEngineWithOptions(t, Options{MaxObstacleHeight: 48}, complexBlock(makeBlock))
		if !e.CanSee(worldX(0), worldY(0), 0, worldX(2), worldY(0), 0) {
			t.Fatal("configured CanSee() = false over 40-height obstacle, want true")
		}
	})
}

func newTestEngine(t testing.TB, first block.Block) *Engine {
	return newTestEngineWithOptions(t, DefaultOptions(), first)
}

func newTestEngineWithOptions(t testing.TB, options Options, first block.Block) *Engine {
	t.Helper()

	e := New(options)
	region, err := block.NewRegionFromBlocks([]block.Block{first})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks(): %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
		t.Fatalf("SetRegion(): %v", err)
	}
	return e
}

func TestQueryPathDoesNotAllocate(t *testing.T) {
	e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}))

	allocs := testing.AllocsPerRun(1000, func() {
		_ = e.Height(worldX(0), worldY(0), 0)
		_ = e.CanMove(worldX(0), worldY(0), 0, worldX(1), worldY(0), 0)
		_ = e.CanSee(worldX(0), worldY(0), 0, worldX(3), worldY(0), 0)
	})
	if allocs != 0 {
		t.Fatalf("query allocations = %.0f, want 0", allocs)
	}
}

func BenchmarkQueries(b *testing.B) {
	e := newTestEngine(b, complexBlock(func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}))

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = e.Height(worldX(0), worldY(0), 0)
		_ = e.CanMove(worldX(0), worldY(0), 0, worldX(1), worldY(0), 0)
		_ = e.CanSee(worldX(0), worldY(0), 0, worldX(3), worldY(0), 0)
	}
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

func worldX(geoX int) int {
	return (geoX << 4) + WorldXMin + 8
}

func worldY(geoY int) int {
	return (geoY << 4) + WorldYMin + 8
}
