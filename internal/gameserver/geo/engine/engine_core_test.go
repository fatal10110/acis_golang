package engine

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ---- from dynamic_test.go ----
type dynamicStub struct {
	x, y, z int
	height  int
	data    [][]block.NSWE
}

func (d dynamicStub) GeoX() int               { return d.x }
func (d dynamicStub) GeoY() int               { return d.y }
func (d dynamicStub) GeoZ() int               { return d.z }
func (d dynamicStub) Height() int             { return d.height }
func (d dynamicStub) GeoData() [][]block.NSWE { return d.data }

func TestEngineDynamicObjectBlocksAndRestoresMovement(t *testing.T) {
	e := New()
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewFlat(0)})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}

	originX, originY := WorldX(0), WorldY(0)
	targetX, targetY := WorldX(1), WorldY(0)
	if !e.CanMove(originX, originY, 0, targetX, targetY, 0) {
		t.Fatal("flat geodata CanMove() = false before adding a dynamic object")
	}

	obj := &dynamicStub{
		x:      0,
		y:      0,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}
	e.AddObject(obj)

	if e.CanMove(originX, originY, 0, targetX, targetY, 0) {
		t.Fatal("CanMove() = true through a closed dynamic object")
	}

	e.RemoveObject(obj)

	if !e.CanMove(originX, originY, 0, targetX, targetY, 0) {
		t.Fatal("CanMove() = false after removing the dynamic object")
	}
}

func TestEngineEvictsDynamicBlockAfterLastObjectRemove(t *testing.T) {
	e := New()
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewFlat(0)})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}

	obj := &dynamicStub{
		x:      0,
		y:      0,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}

	e.AddObject(obj)
	if got := dynamicBlockCount(e); got != 1 {
		t.Fatalf("dynamic block count after AddObject = %d, want 1", got)
	}

	e.RemoveObject(obj)

	if got := dynamicBlockCount(e); got != 0 {
		t.Fatalf("dynamic block count after RemoveObject = %d, want 0", got)
	}
}

// TestEngineConcurrentDoorToggleAndQueries covers #513's correctness
// requirement: swapping dynamicBlocks to a lock-free atomic-pointer read path
// must stay race-safe while a door concurrently opens/closes (toggleObject's
// clone-and-swap on first creation, and repeated in-place Add/Remove on an
// already-created block).
//
// The querying goroutine targets the same block as the toggled door so a
// query can hold a dynamic layer handle across a concurrent Add/Remove. That
// covers both the atomic pointer swap around dynamicBlocks and the dynamic
// block's own stale-handle safety.
func TestEngineConcurrentDoorToggleAndQueries(t *testing.T) {
	e := New()
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewFlat(0)})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
		t.Fatalf("SetRegion: %v", err)
	}

	doorOriginX, doorOriginY := WorldX(0), WorldY(0)
	doorTargetX, doorTargetY := WorldX(1), WorldY(0)
	obj := &dynamicStub{
		x:      0,
		y:      0,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}
	// other shares obj's block so the two toggling goroutines race on the
	// same clone-and-swap insertion the first time either creates the
	// block, then race on in-place Add/Remove against the same
	// *dynamic.Block afterward.
	other := &dynamicStub{
		x:      0,
		y:      0,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}

	queryOriginX, queryOriginY := doorOriginX, doorOriginY
	queryTargetX, queryTargetY := doorTargetX, doorTargetY

	const iterations = 500
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			e.AddObject(obj)
			e.RemoveObject(obj)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			e.AddObject(other)
			e.RemoveObject(other)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = e.CanMove(queryOriginX, queryOriginY, 0, queryTargetX, queryTargetY, 0)
			_ = e.CanSee(queryOriginX, queryOriginY, 0, queryTargetX, queryTargetY, 0)
			_ = e.Height(queryOriginX, queryOriginY, 0)
		}
	}()
	wg.Wait()

	if !e.CanMove(doorOriginX, doorOriginY, 0, doorTargetX, doorTargetY, 0) {
		t.Fatal("CanMove() = false after every toggling goroutine finished on Remove, want the door left open")
	}
}

func dynamicBlockCount(e *Engine) int {
	current := e.dynamicBlocks.Load()
	if current == nil {
		return 0
	}
	return len(*current)
}

// ---- from engine_test.go ----
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

func TestCanSeeWithHeightsIgnoringDynamicObject(t *testing.T) {
	e := newTestEngine(t, block.NewFlat(0))
	door := &dynamicStub{
		x:      1,
		y:      0,
		z:      0,
		height: 40,
		data:   [][]block.NSWE{{block.NoDirections}},
	}
	e.AddObject(door)

	if e.CanSee(worldX(0), worldY(0), 0, worldX(2), worldY(0), 0) {
		t.Fatal("CanSee() = true through closed dynamic object, want false")
	}
	if !e.CanSeeWithHeightsIgnoring(worldX(0), worldY(0), 0, 0, worldX(2), worldY(0), 0, 0, door) {
		t.Fatal("CanSeeWithHeightsIgnoring() = false when target dynamic object is ignored, want true")
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

// ---- from valid_location_test.go ----
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

	t.Run("out of world target walks to the grid border, not the origin", func(t *testing.T) {
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		}))

		ox, oy, oz := worldX(0), worldY(0), 0
		got := e.ValidLocation(ox, oy, oz, WorldXMin-1, oy, oz)
		// Cell (0,0) is open on every side, so the first step west leaves the
		// geodata grid (nx < 0) before any NSWE/obstacle check applies. The
		// reference has no early out-of-world bail-out for the target — it
		// walks the line and stops at the border it exits through
		// (GeoEngine.getValidLocation, GEO_CELLS_X/GEO_CELLS_Y bounds check).
		want := location.Location{X: WorldXMin, Y: oy, Z: oz}
		if got != want {
			t.Fatalf("ValidLocation() = %+v, want %+v", got, want)
		}
	})
}
