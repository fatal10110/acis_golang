package pathfind

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
)

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

// TestFindOnRandomMultilayerCellsAlwaysSatisfiesCanMove fuzzes multilayer
// terrain (cells with 1-3 ascending height layers, each step one of 0, 16,
// 32, or 48 units — steep enough that corner candidates on adjacent cells
// can land on different layers, but not so steep that Find() rarely
// returns a path to check) and asserts that every consecutive pair in a
// returned path satisfies CanMove. It once found seeds where a corner
// (diagonal) candidate's own NSWE mutual-mask gate was satisfied while a
// straight line from the expanding node to that candidate still crossed a
// blocked edge on multilayer terrain — see addCandidateTo's diagonal-only
// CanMove gate. wantMinFound guards against the check going vacuous again:
// an over-pruning regression that made Find() always fail would still pass
// an assertion that only fires when ok is true.
