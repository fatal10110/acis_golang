package block

import (
	"slices"
	"testing"
)

// appendCellsRegion packs one block of every kind: blockY 0 flat, 1 complex,
// 2 multilayer, 3 null.
func appendCellsRegion(t *testing.T) *Region {
	t.Helper()
	var complexCells [CellCount]Cell
	for i := range complexCells {
		complexCells[i] = Cell{Height: int16(i * 8), NSWE: NSWE(i % 16)}
	}
	var layers [CellCount][]Cell
	for i := range layers {
		layers[i] = []Cell{{Height: -64, NSWE: AllDirections}}
	}
	layers[cellIndex(1, 2)] = []Cell{
		{Height: -48, NSWE: AllDirections},
		{Height: 64, NSWE: North | South},
		{Height: 512, NSWE: NoDirections},
	}
	multilayer, err := NewMultilayer(layers)
	if err != nil {
		t.Fatalf("NewMultilayer: %v", err)
	}
	region, err := NewRegionFromBlocks([]Block{NewFlat(-120), NewComplex(complexCells), multilayer, nil})
	if err != nil {
		t.Fatalf("NewRegionFromBlocks: %v", err)
	}
	return region
}

func TestRegionAppendCellsMatchesCells(t *testing.T) {
	region := appendCellsRegion(t)
	for blockY := 0; blockY < 4; blockY++ {
		for cellX := 0; cellX < CellsX; cellX++ {
			for cellY := 0; cellY < CellsY; cellY++ {
				want := region.Cells(0, blockY, cellX, cellY)
				prefix := []Cell{{Height: 7, NSWE: East}}
				got := region.AppendCells(prefix, 0, blockY, cellX, cellY)
				if !slices.Equal(got[:1], prefix) || !slices.Equal(got[1:], want) {
					t.Fatalf("block %d cell (%d,%d): AppendCells = %v, want %v after the prefix", blockY, cellX, cellY, got, want)
				}
			}
		}
	}
	if got := region.Cells(0, 2, 1, 2); len(got) != 3 || got[2].Height != 512 {
		t.Fatalf("multilayer cell layers = %v, want the three stored layers", got)
	}
}

func TestRegionAppendCellsIntoMaxLayersBufferDoesNotAllocate(t *testing.T) {
	region := appendCellsRegion(t)
	var buf [MaxLayers]Cell
	allocs := testing.AllocsPerRun(1000, func() {
		for blockY := 0; blockY < 4; blockY++ {
			_ = region.AppendCells(buf[:0], 0, blockY, 1, 2)
		}
	})
	if allocs != 0 {
		t.Fatalf("AppendCells allocations = %.0f, want 0", allocs)
	}
}
