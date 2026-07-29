package pathfind

import (
	"math/rand"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func newTestEngine(t testing.TB, first block.Block) *engine.Engine {
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

func newGridEngine(t testing.TB, width, height int, cell func(x, y int) block.Cell) *engine.Engine {
	t.Helper()

	e := engine.New()
	region := block.NewRegion()
	for blockX := 0; blockX*block.CellsX < width; blockX++ {
		for blockY := 0; blockY*block.CellsY < height; blockY++ {
			var cells [block.CellCount]block.Cell
			for x := range block.CellsX {
				for y := range block.CellsY {
					gx := blockX*block.CellsX + x
					gy := blockY*block.CellsY + y
					cells[x*block.CellsY+y] = cell(gx, gy)
				}
			}
			if err := region.SetComplex(blockX*block.RegionBlocksY+blockY, cells); err != nil {
				t.Fatalf("SetComplex(): %v", err)
			}
		}
	}
	if err := e.SetRegion(engine.TileXMin, engine.TileYMin, region); err != nil {
		t.Fatalf("SetRegion(): %v", err)
	}
	return e
}

func newSeededMazeEngine(t testing.TB, width, height int, seed int64) *engine.Engine {
	t.Helper()

	r := rand.New(rand.NewSource(seed))
	walls := make(map[[2]int]bool)
	for x := 0; x < width; x++ {
		walls[[2]int{x, 0}] = true
		walls[[2]int{x, height - 1}] = true
	}
	for y := 0; y < height; y++ {
		walls[[2]int{0, y}] = true
		walls[[2]int{width - 1, y}] = true
	}
	for i := 0; i < 18; i++ {
		if r.Intn(2) == 0 {
			x := 2 + r.Intn(width-4)
			y0 := 1 + r.Intn(height-2)
			y1 := 1 + r.Intn(height-2)
			if y0 > y1 {
				y0, y1 = y1, y0
			}
			gap := y0 + r.Intn(y1-y0+1)
			for y := y0; y <= y1; y++ {
				if y != gap {
					walls[[2]int{x, y}] = true
				}
			}
		} else {
			y := 2 + r.Intn(height-4)
			x0 := 1 + r.Intn(width-2)
			x1 := 1 + r.Intn(width-2)
			if x0 > x1 {
				x0, x1 = x1, x0
			}
			gap := x0 + r.Intn(x1-x0+1)
			for x := x0; x <= x1; x++ {
				if x != gap {
					walls[[2]int{x, y}] = true
				}
			}
		}
	}
	walls[[2]int{2, 2}] = false
	walls[[2]int{width - 3, height - 3}] = false

	return newGridEngine(t, width, height, func(x, y int) block.Cell {
		if walls[[2]int{x, y}] {
			return block.Cell{Height: 0, NSWE: block.NoDirections}
		}
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	})
}

func minimumPathIterationCap(t testing.TB, e *engine.Engine, origin, target location.Location, bidirectional bool) int {
	t.Helper()

	options := DefaultOptions()
	options.Bidirectional = bidirectional
	maxIterations := options.MaxIterations
	for cap := 1; cap <= maxIterations; cap *= 2 {
		options.MaxIterations = cap
		if _, _, ok := New(e, options).Find(origin, target); ok {
			low, high := cap/2+1, cap
			for low < high {
				mid := (low + high) / 2
				options.MaxIterations = mid
				if _, _, ok := New(e, options).Find(origin, target); ok {
					high = mid
				} else {
					low = mid + 1
				}
			}
			return low
		}
	}
	t.Fatalf("Find() never found a path with bidirectional=%v", bidirectional)
	return 0
}

func assertPathCanMove(t testing.TB, e *engine.Engine, origin location.Location, path []location.Location) {
	t.Helper()

	if from, to, ok := firstBlockedSegment(e, origin, path); !ok {
		t.Fatalf("path segment cannot move: from %#v to %#v in %#v", from, to, path)
	}
}

func firstBlockedSegment(e *engine.Engine, origin location.Location, path []location.Location) (location.Location, location.Location, bool) {
	previous := origin
	for _, step := range path {
		if !e.CanMove(previous.X, previous.Y, previous.Z, step.X, step.Y, step.Z) {
			return previous, step, false
		}
		previous = step
	}
	return location.Location{}, location.Location{}, true
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
