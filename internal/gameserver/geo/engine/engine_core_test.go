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
	assertDynamicMaskMatchesBlocks(t, e)
}

// TestEngineDynamicMaskGatesUncoveredBlocks covers #2251: a door registered
// anywhere on the map must not change what a query on an uncovered block
// resolves to, and the bitmap that lets those queries skip the map must stay
// in step with the map through add and remove.
func TestEngineDynamicMaskGatesUncoveredBlocks(t *testing.T) {
	e := New()
	cell := func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}
	// Block index 1 is blockX 0, blockY 1, so it covers geoY 8..15 while the
	// queried cells stay in block index 0.
	region, err := block.NewRegionFromBlocks([]block.Block{complexBlock(cell), complexBlock(cell)})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks(): %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
		t.Fatalf("SetRegion(): %v", err)
	}

	if mask := e.dynamicMask[0][0].Load(); mask != nil {
		t.Fatal("dynamicMask allocated for a region with no dynamic object")
	}

	obj := &dynamicStub{
		x:      0,
		y:      block.CellsY,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	}
	e.AddObject(obj)
	assertDynamicMaskMatchesBlocks(t, e)

	// The uncovered block must still resolve through static geodata, and its
	// bit must stay clear so blockAtGeo never reaches the map for it.
	if mask := e.dynamicMask[0][0].Load(); mask == nil || mask.has(0, 0) {
		t.Fatal("dynamicMask bit set for a block with no dynamic overlay")
	}
	if !e.CanMove(worldX(0), worldY(0), 0, worldX(1), worldY(0), 0) {
		t.Fatal("CanMove() = false on an uncovered block while a door exists elsewhere")
	}
	// The covered block keeps its overlay.
	if e.CanMove(worldX(0), worldY(block.CellsY), 0, worldX(1), worldY(block.CellsY), 0) {
		t.Fatal("CanMove() = true through the closed dynamic object")
	}

	e.RemoveObject(obj)
	assertDynamicMaskMatchesBlocks(t, e)
	if mask := e.dynamicMask[0][0].Load(); mask != nil && mask.has(0, 1) {
		t.Fatal("dynamicMask bit still set after the last object was removed")
	}
	if !e.CanMove(worldX(0), worldY(block.CellsY), 0, worldX(1), worldY(block.CellsY), 0) {
		t.Fatal("CanMove() = false after removing the dynamic object")
	}
}

// assertDynamicMaskMatchesBlocks checks the gate invariant: a bit is set for
// exactly the blocks dynamicBlocks holds an overlay for. A missing bit hides a
// live overlay from every query; a stale bit only wastes a lookup, but either
// way it means the clone-and-swap discipline leaked.
func assertDynamicMaskMatchesBlocks(t *testing.T, e *Engine) {
	t.Helper()

	want := map[blockKey]bool{}
	if current := e.dynamicBlocks.Load(); current != nil {
		for key := range *current {
			want[key] = true
		}
	}
	set := 0
	for tileX := range regionTilesX {
		for tileY := range regionTilesY {
			mask := e.dynamicMask[tileX][tileY].Load()
			if mask == nil {
				continue
			}
			for blockX := range block.RegionBlocksX {
				for blockY := range block.RegionBlocksY {
					if !mask.has(blockX, blockY) {
						continue
					}
					set++
					key := blockKey{tileX*block.RegionBlocksX + blockX, tileY*block.RegionBlocksY + blockY}
					if !want[key] {
						t.Errorf("dynamicMask bit set for %+v, which has no dynamic block", key)
					}
				}
			}
		}
	}
	if set != len(want) {
		t.Errorf("dynamicMask has %d bits set, want %d (one per dynamic block)", set, len(want))
	}
}

// TestEngineDynamicMaskAcrossRegionSeam exercises rebuildMasks' multi-region
// path: an object spanning the geo-cell seam between two adjacent regions
// touches blocks in two different tiles in one toggleObject call, which is
// the only input that can catch tileX/tileY transposed with local blockX/
// blockY, or the dedupe loop in rebuildMasks matching the wrong tile.
// Regressed against a reviewer finding on #2292: every other test in this
// file builds its engine at a single tile (TileXMin, TileYMin), so blockX ==
// blockX % block.RegionBlocksX and blockX / block.RegionBlocksX == 0
// everywhere they touch — an identity that would hide a transposed index or
// a divisor/modulus swap.
func TestEngineDynamicMaskAcrossRegionSeam(t *testing.T) {
	e := New()
	cell := func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}
	flatComplex := func() block.Block { return complexBlock(cell) }

	var blocksA, blocksB [block.RegionBlocksX * block.RegionBlocksY]block.Block
	for i := range blocksA {
		blocksA[i] = flatComplex()
		blocksB[i] = flatComplex()
	}
	regionA, err := block.NewRegionFromBlocks(blocksA[:])
	if err != nil {
		t.Fatalf("NewRegionFromBlocks(A): %v", err)
	}
	regionB, err := block.NewRegionFromBlocks(blocksB[:])
	if err != nil {
		t.Fatalf("NewRegionFromBlocks(B): %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, regionA); err != nil {
		t.Fatalf("SetRegion(A): %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin+1, regionB); err != nil {
		t.Fatalf("SetRegion(B): %v", err)
	}

	// Global block index crosses from region A's last block row (255) into
	// region B's first (256) at geo-cell regionCellsY; a 2-cell-tall object
	// straddling that boundary touches one block on each side.
	seamGeoY := regionCellsY - 1
	obj := &dynamicStub{
		x:      0,
		y:      seamGeoY,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections, block.NoDirections}},
	}

	e.AddObject(obj)
	if got := dynamicBlockCount(e); got != 2 {
		t.Fatalf("dynamic block count after cross-seam AddObject = %d, want 2", got)
	}
	assertDynamicMaskMatchesBlocks(t, e)

	if mask := e.dynamicMask[0][0].Load(); mask == nil || !mask.has(0, block.RegionBlocksY-1) {
		t.Fatal("dynamicMask bit not set on region A's last block row after cross-seam AddObject")
	}
	if mask := e.dynamicMask[0][1].Load(); mask == nil || !mask.has(0, 0) {
		t.Fatal("dynamicMask bit not set on region B's first block row after cross-seam AddObject")
	}

	e.RemoveObject(obj)
	if got := dynamicBlockCount(e); got != 0 {
		t.Fatalf("dynamic block count after cross-seam RemoveObject = %d, want 0", got)
	}
	assertDynamicMaskMatchesBlocks(t, e)
	if mask := e.dynamicMask[0][0].Load(); mask != nil && mask.has(0, block.RegionBlocksY-1) {
		t.Fatal("dynamicMask bit still set on region A after cross-seam RemoveObject")
	}
	if mask := e.dynamicMask[0][1].Load(); mask != nil && mask.has(0, 0) {
		t.Fatal("dynamicMask bit still set on region B after cross-seam RemoveObject")
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

	t.Run("mirrors Java's 32-bit overflow on long segments", func(t *testing.T) {
		// Java's canSee squares dx/dy as 32-bit int before Math.sqrt; past
		// ~46340 units in one axis that overflows to a negative int, handing
		// Math.sqrt a NaN and silently disabling the vertical obstacle check
		// for the whole cast (see issue #2049). A wall that would otherwise
		// block "blocks wall crossing" above must stay invisible once the
		// endpoint is far enough away to trigger the overflow.
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

		farX := worldX(0) + 50000
		if !e.CanSee(worldX(0), worldY(0), 0, farX, worldY(0), 0) {
			t.Fatal("CanSee() = false across an overflow-length segment behind a wall, want true (Java's overflow bug hides it)")
		}
	})
}

// TestNullGeodataCrossingLandsAtHeightZero guards the null-block height
// contract (issue #516). Expected values are hand-traced from the reference,
// not from the Go code: GeoEngine.canMove/canSee
// (aCis_gameserver/java/net/sf/l2j/gameserver/geoengine/GeoEngine.java) step
// into the next cell with block.getIndexBelow then block.getHeight, and
// BlockNull (geoengine/geodata/BlockNull.java) answers getIndexBelow = 0 and
// getHeight = 0 for any input, while getHeightNearest echoes (short) worldZ.
// In the reference BlockNull only ever fills a whole region
// (GeoEngine.loadNullBlocks, for a missing or unreadable region file); region
// files never carry single null blocks. Go answers that case with regionBlock
// (region == nil). Go's Region also answers its empty (null) entries the same
// way through its default branches; that layout is reachable only through
// NewRegion/NewRegionFromBlocks, but both null paths are covered here.
//
// Loaded ground sits at -3000 so the snap to 0 cannot be confused with the
// walker keeping its own Z. canMove's final check compares the walked
// height (0) against getHeight(tx, ty, tz) = (short) tz on the null target.
// canSee's walk reads the null cell as ground at 0, above the sight line
// from -3000 (losz = -3000 + MaxObstacleHeight 32), and fails.
//
// CanSee is mutual, and the return cast starting on null fails on its own,
// so the forward cast is also asserted alone: only it proves the step into
// null snaps to 0.
func TestNullGeodataCrossingLandsAtHeightZero(t *testing.T) {
	const ground = -3000

	layouts := []struct {
		name string
		// origin and target geodata X; loaded ground covers origin and
		// origin+1, null geodata covers target-1 and target.
		originX, targetX int
		engine           func(t *testing.T, height int16) *Engine
	}{
		{
			name:    "empty entry inside a loaded region",
			originX: 6,
			targetX: 9,
			engine: func(t *testing.T, height int16) *Engine {
				// Block (0,0) covers geoX 0..7; block (1,0) is empty.
				return newTestEngine(t, block.NewFlat(height))
			},
		},
		{
			name:    "unloaded neighbouring region",
			originX: regionCellsX - 2,
			targetX: regionCellsX + 1,
			engine: func(t *testing.T, height int16) *Engine {
				// Last X block of the first region is loaded; the next
				// region is never set.
				e := New()
				region := block.NewRegion()
				region.SetFlat((block.RegionBlocksX-1)*block.RegionBlocksY, height)
				if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
					t.Fatalf("SetRegion(): %v", err)
				}
				return e
			},
		},
	}

	for _, l := range layouts {
		t.Run(l.name, func(t *testing.T) {
			e := l.engine(t, ground)
			ox, oy := worldX(l.originX), worldY(0)
			tx, ty := worldX(l.targetX), worldY(0)

			if e.HasGeo(tx, ty) {
				t.Fatal("target HasGeo() = true, want null geodata")
			}
			if got := e.Height(tx, ty, ground); got != ground {
				t.Fatalf("null Height(tz=%d) = %d, want the queried Z", ground, got)
			}

			if e.CanMove(ox, oy, ground, tx, ty, ground) {
				t.Error("CanMove(loaded -3000 -> null at -3000) = true, want false: walk lands at 0, not the walker's Z")
			}
			if !e.CanMove(ox, oy, ground, tx, ty, 0) {
				t.Error("CanMove(loaded -3000 -> null at 0) = false, want true: walk lands at 0")
			}

			if e.CanSee(ox, oy, ground, tx, ty, ground) {
				t.Error("CanSee(loaded -3000 -> null at -3000) = true, want false: null ground reads as 0, above the sight line")
			}
			if e.canSee(ox, oy, ground, 0, tx, ty, ground, 0, nil) {
				t.Error("forward canSee(loaded -3000 -> null at -3000) = true, want false: the step into null must read ground at 0")
			}

			flat := l.engine(t, 0)
			if !flat.CanSee(ox, oy, 0, tx, ty, 0) {
				t.Error("CanSee(loaded 0 -> null at 0) = false, want true")
			}
			if !flat.CanMove(ox, oy, 0, tx, ty, 0) {
				t.Error("CanMove(loaded 0 -> null at 0) = false, want true")
			}
		})
	}

	// A null strip between two loaded blocks at -3000: the sight line never
	// ends on null, so only the step into the strip (ground 0 > losz -2968)
	// can block it. Blocks (0,0) and (2,0) are loaded, (1,0) is empty.
	t.Run("null strip between loaded ground", func(t *testing.T) {
		e := New()
		region := block.NewRegion()
		region.SetFlat(0, ground)
		region.SetFlat(2*block.RegionBlocksY, ground)
		if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
			t.Fatalf("SetRegion(): %v", err)
		}
		ox, oy := worldX(6), worldY(0)
		tx, ty := worldX(17), worldY(0)

		if e.canSee(ox, oy, ground, 0, tx, ty, ground, 0, nil) {
			t.Error("forward canSee(-3000 across null strip -> -3000) = true, want false: null ground reads as 0")
		}
		if e.CanSee(ox, oy, ground, tx, ty, ground) {
			t.Error("CanSee(-3000 across null strip -> -3000) = true, want false: null ground reads as 0")
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
	if e.CanSeeActor(worldX(0), worldY(0), 0, 0, worldX(2), worldY(0), 0, 0) {
		t.Fatal("CanSeeActor() = true through closed dynamic object, want false")
	}
	if !e.CanSeeActorIgnoring(worldX(0), worldY(0), 0, 0, worldX(2), worldY(0), 0, 0, door) {
		t.Fatal("CanSeeActorIgnoring() = false when target dynamic object is ignored, want true")
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

// BenchmarkQueriesWithDynamicObject is the production shape #2251 targets: a
// dynamic (door) block exists somewhere on the map, but the queried cells are
// not covered by it. Every cell step still has to decide "is there an overlay
// here?", and that decision must not cost a map hash.
func BenchmarkQueriesWithDynamicObject(b *testing.B) {
	e := newTestEngineWithDoorElsewhere(b)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = e.Height(worldX(0), worldY(0), 0)
		_ = e.CanMove(worldX(0), worldY(0), 0, worldX(1), worldY(0), 0)
		_ = e.CanSee(worldX(0), worldY(0), 0, worldX(3), worldY(0), 0)
	}
}

func BenchmarkHeightWithDynamicObject(b *testing.B) {
	e := newTestEngineWithDoorElsewhere(b)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = e.Height(worldX(0), worldY(0), 0)
	}
}

// newTestEngineWithDoorElsewhere builds a two-block region and registers a
// dynamic object on the second block, leaving the first block — the one the
// benchmarks query — without an overlay.
func newTestEngineWithDoorElsewhere(b testing.TB) *Engine {
	b.Helper()

	cell := func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}
	e := New()
	// Block index 1 is blockX 0, blockY 1, so it covers geoY 8..15.
	region, err := block.NewRegionFromBlocks([]block.Block{complexBlock(cell), complexBlock(cell)})
	if err != nil {
		b.Fatalf("NewRegionFromBlocks(): %v", err)
	}
	if err := e.SetRegion(TileXMin, TileYMin, region); err != nil {
		b.Fatalf("SetRegion(): %v", err)
	}
	e.AddObject(&dynamicStub{
		x:      0,
		y:      block.CellsY,
		z:      0,
		height: 32,
		data:   [][]block.NSWE{{block.NoDirections}},
	})
	if got := dynamicBlockCount(e); got != 1 {
		b.Fatalf("dynamic block count = %d, want 1", got)
	}
	return e
}
