package pathfind

import (
	"math/rand"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

func TestFindBidirectionalMeetsUnderLowerIterationCap(t *testing.T) {
	const width, height = 64, 48
	e := newSeededMazeEngine(t, width, height, 1)
	origin := at(2, 2, 0)
	target := at(width-3, height-3, 0)
	bidirectionalCap := minimumPathIterationCap(t, e, origin, target, true)
	singleCap := minimumPathIterationCap(t, e, origin, target, false)
	t.Logf("minimum iteration cap: bidirectional=%d single-frontier=%d", bidirectionalCap, singleCap)
	if bidirectionalCap >= singleCap {
		t.Fatalf("minimum iteration cap: bidirectional=%d single-frontier=%d, want bidirectional lower", bidirectionalCap, singleCap)
	}

	options := DefaultOptions()
	options.MaxIterations = bidirectionalCap
	options.Bidirectional = true
	finder := New(e, options)

	path, _, ok := finder.Find(origin, target)
	if !ok {
		t.Fatal("Find() = no path under bidirectional cap, want path")
	}
	if got := path[len(path)-1]; got != target {
		t.Fatalf("Find() last = %#v, want %#v", got, target)
	}
	assertPathCanMove(t, e, origin, path)

	options.Bidirectional = false
	if path, _, ok := New(e, options).Find(origin, target); ok {
		t.Fatalf("single-frontier Find() under same cap = %#v, want no path", path)
	}
}

func TestFindBidirectionalMatchesForwardCostWhenBackwardExpands(t *testing.T) {
	e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}))
	options := DefaultOptions()
	options.HeuristicWeight = 0

	options.Bidirectional = false
	_, wantCost, ok := New(e, options).Find(at(0, 0, 0), at(2, 0, 0))
	if !ok {
		t.Fatal("single-frontier Find() = no path, want path")
	}

	options.Bidirectional = true
	path, gotCost, ok := New(e, options).Find(at(0, 0, 0), at(2, 0, 0))
	if !ok {
		t.Fatal("bidirectional Find() = no path, want path")
	}
	if gotCost != wantCost {
		t.Fatalf("bidirectional cost = %d, want forward cost %d for path %#v", gotCost, wantCost, path)
	}
}

func TestFindBidirectionalTraversesLargeDrop(t *testing.T) {
	e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
		if y != 0 || x > 2 {
			return block.Cell{Height: 0, NSWE: block.NoDirections}
		}
		if x < 2 {
			return block.Cell{Height: 80, NSWE: block.AllDirections}
		}
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	}))
	options := DefaultOptions()
	options.HeuristicWeight = 0

	options.Bidirectional = false
	if _, _, ok := New(e, options).Find(at(0, 0, 80), at(2, 0, 0)); !ok {
		t.Fatal("single-frontier Find() = no path across drop, want path")
	}

	options.Bidirectional = true
	path, _, ok := New(e, options).Find(at(0, 0, 80), at(2, 0, 0))
	if !ok {
		t.Fatal("bidirectional Find() = no path across drop, want path")
	}
	if got := path[len(path)-1]; got != at(2, 0, 0) {
		t.Fatalf("Find() last = %#v, want %#v", got, at(2, 0, 0))
	}
	assertPathCanMove(t, e, at(0, 0, 80), path)
}

func TestFindBidirectionalMatchesForwardReachabilityOnRandomHeights(t *testing.T) {
	options := DefaultOptions()
	options.HeuristicWeight = 0
	options.MaxIterations = 10000

	for seed := int64(1); seed <= 40; seed++ {
		r := rand.New(rand.NewSource(seed))
		var heights [block.CellsX][block.CellsY]int16
		for x := range block.CellsX {
			for y := range block.CellsY {
				heights[x][y] = int16(r.Intn(7) * 24)
			}
		}
		e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			return block.Cell{Height: heights[x][y], NSWE: block.AllDirections}
		}))
		origin := at(0, 0, int(heights[0][0]))
		target := at(7, 7, int(heights[7][7]))

		options.Bidirectional = false
		_, _, wantOK := New(e, options).Find(origin, target)
		options.Bidirectional = true
		path, _, gotOK := New(e, options).Find(origin, target)
		if gotOK != wantOK {
			t.Fatalf("seed %d: bidirectional ok = %v, want forward ok %v", seed, gotOK, wantOK)
		}
		if gotOK {
			if got := path[len(path)-1]; got != target {
				t.Fatalf("seed %d: Find() last = %#v, want %#v", seed, got, target)
			}
		}
	}
}

// TestExpandCornerCutting pins expand's diagonal gating to
// addCornerNode's exact mutual-mask-plus-corner-check rule: a diagonal
// candidate is only generated when BOTH orthogonal neighbors' own masks
// mutually permit it (each open toward the other's axis) — never derived
// from a CanMove probe into the corner cell itself. White-box (same
// package) so it can call expand directly and inspect which candidates
// were queued, rather than inferring gating from an emergent A* route that
// could route around a blocked corner and mask the assertion.
