package engine

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

// newTwoTileEngine loads two adjacent all-flat regions so doors can sit in
// different region tiles.
func newTwoTileEngine(t testing.TB) *Engine {
	t.Helper()
	e := New()
	blocks := make([]block.Block, block.RegionBlockCount)
	for i := range blocks {
		blocks[i] = block.NewFlat(0)
	}
	for i := range 2 {
		region, err := block.NewRegionFromBlocks(blocks)
		if err != nil {
			t.Fatalf("NewRegionFromBlocks: %v", err)
		}
		if err := e.SetRegion(TileXMin+i, TileYMin, region); err != nil {
			t.Fatalf("SetRegion: %v", err)
		}
	}
	return e
}

func closedDoorAt(geoX, geoY int) *dynamicStub {
	return &dynamicStub{x: geoX, y: geoY, height: 32, data: [][]block.NSWE{{block.NoDirections}}}
}

// TestDoorToggleReusesItsBlockWithoutAllocating covers #5239: once a door
// has closed once, opening and closing it again neither copies the dynamic
// block map nor the region bitmap, however many other doors are live.
func TestDoorToggleReusesItsBlockWithoutAllocating(t *testing.T) {
	e := newTwoTileEngine(t)
	for i := range 64 {
		e.AddObject(closedDoorAt(i*block.CellsX, 3*block.CellsY))
	}
	door := closedDoorAt(regionCellsX+5, 9)
	e.AddObject(door)
	e.RemoveObject(door)

	allocs := testing.AllocsPerRun(100, func() {
		e.AddObject(door)
		e.RemoveObject(door)
	})
	if allocs != 0 {
		t.Fatalf("allocations per door close+open = %v, want 0", allocs)
	}

	x, y := WorldX(regionCellsX+5), WorldY(9)
	if got := e.Height(x, y, 0); got != 0 {
		t.Fatalf("Height on the open door cell = %d, want the static 0", got)
	}
	e.AddObject(door)
	if got := e.Height(x, y, 0); got != 32 {
		t.Fatalf("Height on the closed door cell = %d, want the door top 32", got)
	}
	if got := dynamicBlockCount(e); got != 65 {
		t.Fatalf("live dynamic blocks = %d, want 65", got)
	}
	assertDynamicMaskMatchesBlocks(t, e)
}

// TestConcurrentDoorTogglesAcrossTilesAndQueries toggles doors in several
// blocks of two region tiles, one a seam-straddling two-block door, while
// readers run height, movement and sight checks over every door cell. Each
// read sees a door either open (static height 0) or closed (door top 32),
// never anything else, and the final state matches the last toggle.
func TestConcurrentDoorTogglesAcrossTilesAndQueries(t *testing.T) {
	e := newTwoTileEngine(t)
	doors := []*dynamicStub{
		closedDoorAt(0, 0),
		closedDoorAt(1, 0), // shares the first door's block
		closedDoorAt(3*block.CellsX+2, 5*block.CellsY+1),
		closedDoorAt(regionCellsX+block.CellsX, 2*block.CellsY),
		{x: regionCellsX - 1, y: 7, height: 32, data: [][]block.NSWE{{block.NoDirections}, {block.NoDirections}}},
	}

	const iterations = 300
	var togglers, readers sync.WaitGroup
	var done atomic.Bool
	for _, d := range doors {
		togglers.Go(func() {
			for range iterations {
				e.AddObject(d)
				e.RemoveObject(d)
			}
		})
	}
	for range 2 {
		readers.Go(func() {
			for !done.Load() {
				for _, d := range doors {
					for dx := range len(d.data) {
						x, y := WorldX(d.x+dx), WorldY(d.y)
						if h := e.Height(x, y, 0); h != 0 && h != 32 {
							t.Errorf("Height(%d,%d) = %d during toggles, want 0 or 32", d.x+dx, d.y, h)
							return
						}
						_ = e.CanMove(x, y, 0, WorldX(d.x+dx+1), y, 0)
						_ = e.CanSee(WorldX(d.x+dx-1), y, 0, WorldX(d.x+dx+1), y, 0)
					}
				}
			}
		})
	}
	togglers.Wait()
	done.Store(true)
	readers.Wait()

	if got := dynamicBlockCount(e); got != 0 {
		t.Fatalf("live dynamic blocks after every door opened = %d, want 0", got)
	}
	assertDynamicMaskMatchesBlocks(t, e)
	for _, d := range doors {
		for dx := range len(d.data) {
			if got := e.Height(WorldX(d.x+dx), WorldY(d.y), 0); got != 0 {
				t.Fatalf("Height on open door cell (%d,%d) = %d, want 0", d.x+dx, d.y, got)
			}
		}
	}

	for _, d := range doors {
		e.AddObject(d)
	}
	assertDynamicMaskMatchesBlocks(t, e)
	for _, d := range doors {
		for dx := range len(d.data) {
			if got := e.Height(WorldX(d.x+dx), WorldY(d.y), 0); got != 32 {
				t.Fatalf("Height on closed door cell (%d,%d) = %d, want 32", d.x+dx, d.y, got)
			}
		}
	}
}

// BenchmarkDoorToggle measures one door close+open with 256 other doors
// live.
func BenchmarkDoorToggle(b *testing.B) {
	e := newTwoTileEngine(b)
	for i := range 256 {
		e.AddObject(closedDoorAt((i%32)*block.CellsX, (i/32)*block.CellsY))
	}
	door := closedDoorAt(regionCellsX+5, 9)
	b.ReportAllocs()
	for b.Loop() {
		e.AddObject(door)
		e.RemoveObject(door)
	}
}
