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

func TestSightHeight(t *testing.T) {
	tests := []struct {
		name                  string
		collisionHeight       float64
		partOfCharacterHeight int
		want                  float64
	}{
		// Java reference: creature.getCollisionHeight() * 2 * Config.PART_OF_CHARACTER_HEIGHT / 100.
		{"default 75 percent", 20, 75, 30},
		{"100 percent doubles collision height", 20, 100, 40},
		{"0 percent", 20, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(Options{PartOfCharacterHeight: tt.partOfCharacterHeight})
			if got := e.SightHeight(tt.collisionHeight); got != tt.want {
				t.Fatalf("SightHeight(%v) = %v, want %v", tt.collisionHeight, got, tt.want)
			}
		})
	}
}

func TestCanSeeActor(t *testing.T) {
	// A height-40 wall sits between the two actors, matching TestCanSee's
	// "blocks wall crossing" fixture.
	makeBlock := func(x, y int) block.Cell {
		if x == 1 && y == 0 {
			return block.Cell{Height: 40, NSWE: block.AllDirections}
		}
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}

	t.Run("blocked at ground-level eye height", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(makeBlock))
		if e.CanSeeActor(worldX(0), worldY(0), 0, 0, worldX(2), worldY(0), 0, 0) {
			t.Fatal("CanSeeActor() = true over 40-height wall at 0 collision height, want false")
		}
	})

	t.Run("clears wall once actor eye height accounts for it", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(makeBlock))
		if !e.CanSeeActor(worldX(0), worldY(0), 0, 20, worldX(2), worldY(0), 0, 20) {
			t.Fatal("CanSeeActor() = false with 20 collision height over 40-height wall, want true")
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

// BenchmarkQueriesParallel exercises the contention #513 targets: many
// goroutines hammering CanMove/CanSee/Height concurrently, the actual
// AI-tick-population shape rather than a single-goroutine ns/op number.
func BenchmarkQueriesParallel(b *testing.B) {
	e := newTestEngine(b, complexBlock(func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}))

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = e.Height(worldX(0), worldY(0), 0)
			_ = e.CanMove(worldX(0), worldY(0), 0, worldX(1), worldY(0), 0)
			_ = e.CanSee(worldX(0), worldY(0), 0, worldX(3), worldY(0), 0)
		}
	})
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
