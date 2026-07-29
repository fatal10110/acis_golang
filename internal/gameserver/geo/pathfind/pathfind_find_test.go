package pathfind

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
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
