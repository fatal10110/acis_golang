package pathfind

import (
	"math/rand"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFindOnRandomMultilayerCellsAlwaysSatisfiesCanMove(t *testing.T) {
	options := DefaultOptions()
	options.HeuristicWeight = 0
	options.MaxIterations = 10000

	const seeds = 40
	const wantMinFound = 25
	found := 0

	for seed := int64(1); seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(seed))
		var cells [block.CellCount][]block.Cell
		for x := range block.CellsX {
			for y := range block.CellsY {
				n := r.Intn(3) + 1
				layers := make([]block.Cell, n)
				h := int16(0)
				for i := 0; i < n; i++ {
					h += int16(r.Intn(4) * 16)
					layers[i] = block.Cell{Height: h, NSWE: block.AllDirections}
				}
				cells[x*block.CellsY+y] = layers
			}
		}
		ml, err := block.NewMultilayer(cells)
		if err != nil {
			t.Fatalf("seed %d: NewMultilayer(): %v", seed, err)
		}
		e := newTestEngine(t, ml)

		topHeight := func(x, y int) int {
			layers := cells[x*block.CellsY+y]
			return int(layers[len(layers)-1].Height)
		}
		origin := at(0, 0, topHeight(0, 0))
		target := at(7, 7, topHeight(7, 7))

		for _, bidirectional := range []bool{false, true} {
			options.Bidirectional = bidirectional
			path, _, ok := New(e, options).Find(origin, target)
			if !ok {
				continue
			}
			found++
			if from, to, good := firstBlockedSegment(e, origin, path); !good {
				t.Errorf("seed %d bidirectional=%v: CanMove rejects segment %#v -> %#v in path %#v", seed, bidirectional, from, to, path)
			}
		}
	}

	if found < wantMinFound {
		t.Fatalf("found a path in %d/%d seed-runs, want at least %d — terrain generator may be too steep to exercise the corner-cut check", found, seeds*2, wantMinFound)
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
