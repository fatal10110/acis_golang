package pathfind

import (
	"math/rand"
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

	t.Run("open field path is smoothed to target", func(t *testing.T) {
		finder := New(newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		})), DefaultOptions())

		path, cost, ok := finder.Find(at(0, 0, 0), at(7, 3, 0))
		if !ok {
			t.Fatal("Find() = no path, want path")
		}
		if got := path[len(path)-1]; got != at(7, 3, 0) {
			t.Fatalf("Find() last = %#v, want %#v", got, at(7, 3, 0))
		}
		const gridLockedCorners = 5
		if len(path) >= gridLockedCorners {
			t.Fatalf("Find() path = %#v, want fewer than %d grid-locked corners", path, gridLockedCorners)
		}
		const gridLockedCost = 82
		if cost >= gridLockedCost {
			t.Fatalf("Find() cost = %d, want less than grid-locked cost %d", cost, gridLockedCost)
		}
	})

	t.Run("blocked path returns no path", func(t *testing.T) {
		// Fully enclose the target in a ring of walled (NoDirections)
		// cells on all 8 sides, cardinal and diagonal. A single walled
		// column isn't enough under the reference's own gating: candidate
		// generation no longer carries CanMove's whole-route
		// height-continuity check (that was a Go-only invariant, not part
		// of the reference), so a lone wall can be routed around through
		// the always-open unloaded area outside the test's 8x8 block. A
		// walled cell can never itself expand (expand() returns
		// immediately when a node's own mask is NoDirections), so a full
		// ring around the target is genuinely unreachable regardless of
		// what's open beyond it.
		ring := map[[2]int]bool{
			{3, 3}: true, {4, 3}: true, {5, 3}: true,
			{3, 4}: true, {5, 4}: true,
			{3, 5}: true, {4, 5}: true, {5, 5}: true,
		}
		finder := New(newTestEngine(t, complexBlock(func(x, y int) block.Cell {
			if ring[[2]int{x, y}] {
				return block.Cell{Height: 0, NSWE: block.NoDirections}
			}
			return block.Cell{Height: 0, NSWE: block.AllDirections}
		})), DefaultOptions())

		path, _, ok := finder.Find(at(0, 0, 0), at(4, 4, 0))
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
func TestExpandCornerCutting(t *testing.T) {
	const cx, cy = 4, 4

	tests := []struct {
		name                           string
		currentMask                    block.NSWE // defaults to AllDirections when zero
		north, south, west, east       block.NSWE
		wantN, wantS, wantW, wantE     bool
		wantNW, wantNE, wantSW, wantSE bool
	}{
		{
			name:  "open room allows every direction",
			north: block.AllDirections, south: block.AllDirections,
			west: block.AllDirections, east: block.AllDirections,
			wantN: true, wantS: true, wantW: true, wantE: true,
			wantNW: true, wantNE: true, wantSW: true, wantSE: true,
		},
		{
			// The cardinal candidate itself is gated only by CURRENT's own
			// mask (always open here), not the neighbor's — a walled
			// neighbor still gets created as an obstacle-weighted dead end
			// (matches addNode: the target cell's mask never gates its own
			// creation). Only the diagonals, which mutually test the
			// neighbors' own masks, are affected.
			name:  "walled west neighbor blocks both west diagonals",
			north: block.AllDirections, south: block.AllDirections,
			west: block.NoDirections, east: block.AllDirections,
			wantN: true, wantS: true, wantW: true, wantE: true,
			wantNW: false, wantNE: true, wantSW: false, wantSE: true,
		},
		{
			name:  "walled north neighbor blocks both north diagonals",
			north: block.NoDirections, south: block.AllDirections,
			west: block.AllDirections, east: block.AllDirections,
			wantN: true, wantS: true, wantW: true, wantE: true,
			wantNW: false, wantNE: false, wantSW: true, wantSE: true,
		},
		{
			name:  "walled south and east neighbors leave only the NW corner open",
			north: block.AllDirections, south: block.NoDirections,
			west: block.AllDirections, east: block.NoDirections,
			wantN: true, wantS: true, wantW: true, wantE: true,
			wantNW: true, wantNE: false, wantSW: false, wantSE: false,
		},
		{
			// Pins the OTHER gate in expand: current's own mask, not just
			// the neighbors'. North/West open on an otherwise fully open
			// room still must not generate S/E (or any corner needing
			// them), because current.nswe itself never allows those
			// directions to be considered at all — addDirectionalNode's
			// short-circuit, before any neighbor is even queried.
			name:        "partial current mask suppresses the closed cardinals and their corners",
			currentMask: block.North | block.West,
			north:       block.AllDirections, south: block.AllDirections,
			west: block.AllDirections, east: block.AllDirections,
			wantN: true, wantS: false, wantW: true, wantE: false,
			wantNW: true, wantNE: false, wantSW: false, wantSE: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := newTestEngine(t, complexBlock(func(x, y int) block.Cell {
				switch {
				case x == cx && y == cy-1:
					return block.Cell{NSWE: test.north}
				case x == cx && y == cy+1:
					return block.Cell{NSWE: test.south}
				case x == cx-1 && y == cy:
					return block.Cell{NSWE: test.west}
				case x == cx+1 && y == cy:
					return block.Cell{NSWE: test.east}
				default:
					return block.Cell{NSWE: block.AllDirections}
				}
			}))
			f := New(e, DefaultOptions())

			scratch := &searchScratch{}
			scratch.reset()
			current := scratch.newNode(cx, cy, 0)
			current.nswe = test.currentMask
			if current.nswe == block.NoDirections {
				current.nswe = block.AllDirections
			}
			goal := scratch.newNode(cx+5, cy+5, 0)
			seq := int64(1)

			f.expand(current, goal, &seq, scratch)

			for _, candidate := range []struct {
				name   string
				gx, gy int
				want   bool
			}{
				{"N", cx, cy - 1, test.wantN},
				{"S", cx, cy + 1, test.wantS},
				{"W", cx - 1, cy, test.wantW},
				{"E", cx + 1, cy, test.wantE},
				{"NW", cx - 1, cy - 1, test.wantNW},
				{"NE", cx + 1, cy - 1, test.wantNE},
				{"SW", cx - 1, cy + 1, test.wantSW},
				{"SE", cx + 1, cy + 1, test.wantSE},
			} {
				_, got := scratch.openSet[nodeKey{gx: candidate.gx, gy: candidate.gy, z: 0}]
				if got != candidate.want {
					t.Errorf("%s candidate queued = %v, want %v", candidate.name, got, candidate.want)
				}
			}
		})
	}
}

func TestAddCandidateKeepsCheaperGridParent(t *testing.T) {
	finder := New(newTestEngine(t, complexBlock(func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	})), DefaultOptions())
	scratch := &searchScratch{}
	scratch.reset()
	parent := scratch.newNode(0, 0, 0)
	current := scratch.newNode(1, 0, 0)
	current.g = finder.options.MoveWeight
	current.parent = parent
	goal := scratch.newNode(3, 0, 0)
	seq := int64(1)

	finder.addCandidate(current, goal, &seq, scratch, 2, 0, 0, block.East|block.West, false)

	got := &scratch.nodes[len(scratch.nodes)-1]
	if got.parent != current {
		t.Fatal("addCandidate() re-parented to a more expensive smoothed link")
	}
	wantCost := finder.options.MoveWeight + finder.options.ObstacleWeight
	if got.g != wantCost {
		t.Fatalf("candidate g = %d, want grid cost %d", got.g, wantCost)
	}
}

// TestFindCrossesBridgeWithNoFloorBeneath covers the multilayer case: a
// bridge column (x=3..5) whose cells have a single layer at height 40 —
// deliberately no ground layer underneath, the unambiguous multilayer
// shape (a span over a void, not over walkable ground, which existing
// Below/Height/NSWE resolution — unchanged by this PR — always prefers the
// lowest qualifying layer, so a scenario with both a ground and a bridge
// layer wouldn't isolate which one candidate generation used). Because no
// ground layer exists at those cells, Find() can only succeed if
// NodeBelow correctly resolved the bridge layer's own height and NSWE mask
// for each bridge-column candidate, mirroring the reference's
// getIndexBelow/getHeight/getNswe sequence.
func TestFindCrossesBridgeWithNoFloorBeneath(t *testing.T) {
	// bridgeHeight must stay below block.CellIgnoreHeight (48 = CellHeight(8)
	// x 6) for NodeBelow to resolve the bridge layer at all when stepping on
	// from ground level (query z = ground height + CellIgnoreHeight) — if
	// this ever exceeds CellIgnoreHeight independently of the fixture, the
	// bridge becomes unreachable and this test would start failing for an
	// unrelated reason.
	const bridgeHeight = 40

	var cells [block.CellCount][]block.Cell
	for x := range block.CellsX {
		for y := range block.CellsY {
			ci := x*block.CellsY + y
			if x >= 3 && x <= 5 {
				cells[ci] = []block.Cell{{Height: bridgeHeight, NSWE: block.AllDirections}}
			} else {
				cells[ci] = []block.Cell{{Height: 0, NSWE: block.AllDirections}}
			}
		}
	}
	bridge, err := block.NewMultilayer(cells)
	if err != nil {
		t.Fatalf("NewMultilayer(): %v", err)
	}

	finder := New(newTestEngine(t, bridge), DefaultOptions())

	path, cost, ok := finder.Find(at(0, 0, 0), at(7, 0, 0))
	if !ok {
		t.Fatal("Find() = no path, want a path across the bridge (no ground layer exists under x=3..5)")
	}
	if cost <= 0 {
		t.Fatalf("Find() cost = %d, want a positive cost", cost)
	}
	if got := path[len(path)-1]; got != at(7, 0, 0) {
		t.Fatalf("Find() last = %#v, want %#v", got, at(7, 0, 0))
	}
}

func BenchmarkFinder(b *testing.B) {
	finder := New(newTestEngine(b, complexBlock(func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	})), DefaultOptions())
	origin := at(0, 0, 0)
	target := at(3, 0, 0)
	dst := make([]location.Location, 0, 8)

	b.Run("Find", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			path, _, ok := finder.Find(origin, target)
			if !ok || len(path) == 0 {
				b.Fatal("Find() = no path")
			}
		}
	})
	b.Run("FindInto", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			path, _, ok := finder.FindInto(dst[:0], origin, target)
			if !ok || len(path) == 0 {
				b.Fatal("FindInto() = no path")
			}
		}
	})
	b.Run("HasPath", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !finder.HasPath(origin, target) {
				b.Fatal("HasPath() = false")
			}
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
		Bidirectional:   true,
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
