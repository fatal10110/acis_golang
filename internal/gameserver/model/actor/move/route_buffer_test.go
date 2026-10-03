package move

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// wallRouteGeo is one 8x8 block, open and flat except a wall across x=3 for
// y=0..5, so a straight walk across it is blocked and the search routes
// around its end at y=6..7.
func wallRouteGeo(t testing.TB) (*engine.Engine, *pathfind.Finder) {
	t.Helper()

	var cells [block.CellCount]block.Cell
	for x := range block.CellsX {
		for y := range block.CellsY {
			nswe := block.AllDirections
			if x == 3 && y <= 5 {
				nswe = block.NoDirections
			}
			cells[x*block.CellsY+y] = block.Cell{Height: 0, NSWE: nswe}
		}
	}
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewComplex(cells)})
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New()
	if err := e.SetRegion(engine.TileXMin, engine.TileYMin, region); err != nil {
		t.Fatal(err)
	}
	return e, pathfind.New(e, pathfind.DefaultOptions())
}

func cellAt(gx, gy int) location.Location {
	return location.Location{X: engine.WorldX(gx), Y: engine.WorldY(gy)}
}

func newRoutedMover(t testing.TB, origin location.Location) (*CreatureMove, *engine.Engine, *pathfind.Finder) {
	t.Helper()

	e, finder := wallRouteGeo(t)
	mover, err := NewCreatureMove(origin, 160, NewGeo(e, finder))
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	return mover, e, finder
}

func TestRoutedMoveQueuesFinderPath(t *testing.T) {
	origin, target := cellAt(1, 1), cellAt(6, 2)
	mover, e, finder := newRoutedMover(t, origin)
	if e.CanMove(origin.X, origin.Y, origin.Z, target.X, target.Y, target.Z) {
		t.Fatal("fixture: straight line open, want it blocked by the wall")
	}
	want, _, ok := finder.Find(origin, target)
	if !ok || len(want) < 2 {
		t.Fatalf("fixture: Find() = %v, %v; want a multi-leg route", want, ok)
	}

	ev, outcome, err := mover.MoveToLocationWithPathOutcome(target)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != pathRouted {
		t.Fatalf("outcome = %v, want routed", outcome)
	}
	if ev.Destination != want[0] {
		t.Fatalf("first leg = %+v, want %+v", ev.Destination, want[0])
	}
	if !slices.Equal(mover.waypoints, want[1:]) {
		t.Fatalf("queued legs = %+v, want %+v", mover.waypoints, want[1:])
	}
}

// A new request searches into the route buffer the walked route does not
// use: the route stays intact while the search runs, and after a request
// that never commits its route.
func TestRoutedSearchLeavesWalkedRouteIntact(t *testing.T) {
	origin := cellAt(1, 1)
	mover, _, _ := newRoutedMover(t, origin)
	if _, outcome, err := mover.MoveToLocationWithPathOutcome(cellAt(6, 2)); err != nil || outcome != pathRouted {
		t.Fatalf("first request = %v, %v; want routed", outcome, err)
	}
	walked := slices.Clone(mover.waypoints)

	for _, target := range []location.Location{cellAt(5, 0), cellAt(7, 4), cellAt(4, 1)} {
		mover.mu.Lock()
		_, _, outcome := mover.resolvePathLocked(target)
		got := slices.Clone(mover.waypoints)
		mover.mu.Unlock()
		if outcome != pathRouted {
			t.Fatalf("search to %+v = %v, want routed", target, outcome)
		}
		if !slices.Equal(got, walked) {
			t.Fatalf("walked route after uncommitted search to %+v = %+v, want %+v", target, got, walked)
		}
	}

	// A committed route makes the other buffer current; the next search
	// must not write into it either.
	if _, outcome, err := mover.MoveToLocationWithPathOutcome(cellAt(7, 4)); err != nil || outcome != pathRouted {
		t.Fatalf("second request = %v, %v; want routed", outcome, err)
	}
	walked = slices.Clone(mover.waypoints)
	mover.mu.Lock()
	mover.resolvePathLocked(cellAt(6, 2))
	got := slices.Clone(mover.waypoints)
	mover.mu.Unlock()
	if !slices.Equal(got, walked) {
		t.Fatalf("committed route after a later search = %+v, want %+v", got, walked)
	}
}

// fixedPathGeo answers FindPath from one shared slice and has no
// FindPathInto, so the mover must copy rather than keep its storage.
type fixedPathGeo struct {
	staticGeo
	path []location.Location
}

func (fixedPathGeo) CanMove(_, _, _, _, _, _ int) bool { return false }

func (g fixedPathGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return g.path, true
}

func TestRoutedMoveCopiesPlainFindPathResult(t *testing.T) {
	shared := []location.Location{{X: 50}, {X: 100}, {X: 150}}
	mover, err := NewCreatureMove(location.Location{}, 160, fixedPathGeo{path: shared})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	if _, outcome, err := mover.MoveToLocationWithPathOutcome(location.Location{X: 150}); err != nil || outcome != pathRouted {
		t.Fatalf("request = %v, %v; want routed", outcome, err)
	}
	mover.waypoints[0] = location.Location{X: -1}
	if shared[1] != (location.Location{X: 100}) {
		t.Fatal("queued legs alias the Geo's own FindPath slice")
	}
}
