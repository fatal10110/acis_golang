package engine

import (
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

// nodeAtOrAboveCellsReference is NodeAtOrAbove as it was before #3273, over
// a freshly copied Cells slice; the buffered walk must answer identically.
func nodeAtOrAboveCellsReference(e *Engine, geoX, geoY, worldZ int, accept func(int16) bool) (int16, block.NSWE, bool) {
	cells := e.blockAtGeo(geoX, geoY).Cells(localCell(geoX), localCell(geoY))
	for i := len(cells) - 1; i >= 0; i-- {
		if int(cells[i].Height) >= worldZ && accept(cells[i].Height) {
			return cells[i].Height, cells[i].NSWE, true
		}
	}
	return 0, 0, false
}

// newMultilayerTestEngine loads one multilayer block at the first region's
// origin: cells (1,2) and (3,3) carry three layers, every other cell one.
func newMultilayerTestEngine(t *testing.T) *Engine {
	t.Helper()
	var layers [block.CellCount][]block.Cell
	for i := range layers {
		layers[i] = []block.Cell{{Height: -64, NSWE: block.AllDirections}}
	}
	stack := []block.Cell{
		{Height: -48, NSWE: block.AllDirections},
		{Height: 64, NSWE: block.North | block.South},
		{Height: 512, NSWE: block.East},
	}
	layers[1*block.CellsY+2] = stack
	layers[3*block.CellsY+3] = slices.Clone(stack)
	multilayer, err := block.NewMultilayer(layers)
	if err != nil {
		t.Fatalf("NewMultilayer: %v", err)
	}
	return newTestEngine(t, multilayer)
}

func TestNodeAtOrAboveMatchesCellsWalk(t *testing.T) {
	e := newMultilayerTestEngine(t)
	door := &dynamicStub{x: 1, y: 2, z: 64, height: 32, data: [][]block.NSWE{{block.NoDirections}}}

	accepts := map[string]func(int16) bool{
		"all":        func(int16) bool { return true },
		"none":       func(int16) bool { return false },
		"not top":    func(h int16) bool { return h != 512 },
		"below 100":  func(h int16) bool { return h < 100 },
		"only floor": func(h int16) bool { return h < 0 },
	}
	check := func(t *testing.T, state string) {
		t.Helper()
		for _, cell := range [][2]int{{1, 2}, {3, 3}, {0, 0}, {9, 9}} {
			for z := -128; z <= 640; z += 8 {
				for name, accept := range accepts {
					h, n, ok := e.NodeAtOrAbove(cell[0], cell[1], z, accept)
					wh, wn, wok := nodeAtOrAboveCellsReference(e, cell[0], cell[1], z, accept)
					if h != wh || n != wn || ok != wok {
						t.Fatalf("%s: cell %v z=%d accept=%s: NodeAtOrAbove = (%d,%v,%v), want (%d,%v,%v)",
							state, cell, z, name, h, n, ok, wh, wn, wok)
					}
				}
			}
		}
	}

	check(t, "static")
	if h, n, ok := e.NodeAtOrAbove(1, 2, -100, accepts["not top"]); !ok || h != 64 || n != block.North|block.South {
		t.Fatalf("static: NodeAtOrAbove skipping the top layer = (%d,%v,%v), want (64,NS,true)", h, n, ok)
	}

	e.AddObject(door)
	if dynamicBlockCount(e) != 1 {
		t.Fatalf("dynamic block count = %d, want 1", dynamicBlockCount(e))
	}
	// Cell (1,2) reads the door's override; (3,3), (0,0) sit in the same
	// dynamic block untouched and must read the static layers.
	check(t, "door closed")
	if _, n, ok := e.NodeAtOrAbove(1, 2, -100, accepts["below 100"]); !ok || n != block.NoDirections {
		t.Fatalf("door closed: NodeAtOrAbove NSWE = %v (ok %v), want the door's closed layer", n, ok)
	}

	e.RemoveObject(door)
	check(t, "door removed")
}

func TestNodeAtOrAboveDoesNotAllocate(t *testing.T) {
	e := newMultilayerTestEngine(t)
	notTop := func(h int16) bool { return h != 512 }
	measure := func(state string) {
		t.Helper()
		allocs := testing.AllocsPerRun(1000, func() {
			_, _, _ = e.NodeAtOrAbove(1, 2, -100, notTop)
			_, _, _ = e.NodeAtOrAbove(3, 3, -100, notTop)
			_, _, _ = e.NodeAtOrAbove(0, 0, -100, notTop)
			_, _, _ = e.NodeAtOrAbove(regionCellsX+1, 0, -100, notTop) // unloaded region
		})
		if allocs != 0 {
			t.Fatalf("%s: NodeAtOrAbove allocations = %.0f, want 0", state, allocs)
		}
	}
	measure("static")
	e.AddObject(&dynamicStub{x: 1, y: 2, z: 64, height: 32, data: [][]block.NSWE{{block.NoDirections}}})
	measure("door closed")
}

// TestAppendCellsSnapshotUnderConcurrentToggle reads a door cell's layers
// while the door toggles: every read must be exactly the open or the closed
// layer set, never a mix.
func TestAppendCellsSnapshotUnderConcurrentToggle(t *testing.T) {
	e := newMultilayerTestEngine(t)
	door := &dynamicStub{x: 1, y: 2, z: 64, height: 32, data: [][]block.NSWE{{block.NoDirections}}}
	read := func() []block.Cell {
		var buf [block.MaxLayers]block.Cell
		return slices.Clone(e.blockAtGeo(1, 2).AppendCells(buf[:0], 1, 2))
	}
	open := read()
	e.AddObject(door)
	closed := read()
	e.RemoveObject(door)
	if slices.Equal(open, closed) {
		t.Fatalf("door did not change the cell: %v", open)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.AddObject(door)
			e.RemoveObject(door)
		}
		close(stop)
	}()
	for done := false; !done; {
		select {
		case <-stop:
			done = true
		default:
		}
		if got := read(); !slices.Equal(got, open) && !slices.Equal(got, closed) {
			t.Fatalf("AppendCells mid-toggle = %v, want %v or %v", got, open, closed)
		}
	}
	wg.Wait()
}
