package pathfind

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// flatOpenBlock returns a region containing only block index 0 (geoX 0-7,
// geoY 0-7), open in every direction at height 0. Every other block index —
// including index 256 (geoX 8-15, geoY 0-7; blocks are indexed
// blockX*RegionBlocksY+blockY per block/region.go:350) — is left unset by
// block.NewRegionFromBlocks, so it reports no geodata (block.KindNull).
func flatOpenBlock() block.Block {
	return complexBlock(func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	})
}

// Reference: GeoEngine.findPath returns Collections.emptyList() when the
// origin's own geo block has no geodata (GeoEngine.java:1758-1761), before
// resolving any height. geoX 8 falls in block index 256, which
// block.NewRegionFromBlocks leaves as block.KindNull.
func TestFindRejectsOriginWithoutGeodata(t *testing.T) {
	finder := New(newTestEngine(t, flatOpenBlock()), DefaultOptions())

	origin := at(8, 0, 0)
	target := at(3, 0, 0)

	path, cost, ok := finder.Find(origin, target)
	if ok {
		t.Fatalf("Find() = %#v, want no path (origin has no geodata)", path)
	}
	if len(path) != 0 || cost != 0 {
		t.Fatalf("Find() = (%#v, %d), want (nil, 0)", path, cost)
	}
	if finder.HasPath(origin, target) {
		t.Fatal("HasPath() = true, want false (origin has no geodata)")
	}
}

// Reference: GeoEngine.findPath returns Collections.emptyList() when the
// target's own geo block has no geodata (GeoEngine.java:1766-1769).
func TestFindRejectsTargetWithoutGeodata(t *testing.T) {
	finder := New(newTestEngine(t, flatOpenBlock()), DefaultOptions())

	origin := at(3, 0, 0)
	target := at(8, 0, 0)

	path, cost, ok := finder.Find(origin, target)
	if ok {
		t.Fatalf("Find() = %#v, want no path (target has no geodata)", path)
	}
	if len(path) != 0 || cost != 0 {
		t.Fatalf("Find() = (%#v, %d), want (nil, 0)", path, cost)
	}
	if finder.HasPath(origin, target) {
		t.Fatal("HasPath() = true, want false (target has no geodata)")
	}
}

// Reference: GeoEngine.findPath rejects when the target's own nearest
// geodata height lies more than 500 units from the requested target Z
// (GeoEngine.java:1771-1773), even though both endpoints have loaded
// geodata and the target cell is otherwise reachable.
func TestFindRejectsTargetHeightFarFromGeodata(t *testing.T) {
	finder := New(newTestEngine(t, flatOpenBlock()), DefaultOptions())

	origin := at(0, 0, 0)
	target := location.Location{X: at(3, 0, 0).X, Y: at(3, 0, 0).Y, Z: 600}

	path, cost, ok := finder.Find(origin, target)
	if ok {
		t.Fatalf("Find() = %#v, want no path (target Z 600 units from geodata height 0)", path)
	}
	if len(path) != 0 || cost != 0 {
		t.Fatalf("Find() = (%#v, %d), want (nil, 0)", path, cost)
	}
}

// Reference: GeoEngine.findPath's post-search collapse tests canMove(A, C)
// unconditionally, with no distance cap, unlike the bounded (maxSmoothCells
// = 32) parent-skip smoothing the A* search itself performs
// (GeoEngine.java:1775-1846). This drives collapseWaypoints directly with
// waypoints far enough apart that the search-time cap could never have
// merged them, over an open field where every straight move succeeds, and
// checks they still collapse away.
func TestCollapseWaypointsHasNoDistanceCap(t *testing.T) {
	const width, height = 220, 4
	e := newGridEngine(t, width, height, func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	})
	finder := New(e, DefaultOptions())

	origin := at(0, 0, 0)
	path := []location.Location{at(70, 0, 0), at(140, 0, 0), at(210, 0, 0)}

	// Sanity: each hop is far past maxSmoothCells, so this only exercises
	// collapseWaypoints's own unconditional canMove check, not any
	// leftover in-search smoothing.
	const hopCells = 70
	if hopCells <= maxSmoothCells {
		t.Fatalf("test hop %d cells, want more than maxSmoothCells %d", hopCells, maxSmoothCells)
	}

	got := finder.collapseWaypoints(origin.X, origin.Y, origin.Z, append([]location.Location{}, path...))
	want := []location.Location{path[len(path)-1]}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("collapseWaypoints() = %#v, want %#v (fully collapsed to the target)", got, want)
	}
}

// TestFindAppliesUnboundedCollapseThroughPublicAPI drives the collapse
// through Find() itself, not the unexported collapseWaypoints helper, so the
// call site at pathfind.go:94-96 is what's under test: a regression that
// drops the call, flips buildResult, or loosens the len(path) >= 3 guard
// fails here even though every other test in this file still passes.
//
// The search around a single wall gap leaves 7 waypoints before collapse
// (geo(27,2) geo(29,7) geo(29,10) geo(31,10) geo(31,7) geo(33,2) geo(55,2));
// collapse must reduce that to the 2 that remain necessary once waypoints
// beyond the gap become mutually visible again.
func TestFindAppliesUnboundedCollapseThroughPublicAPI(t *testing.T) {
	const width, height = 60, 20
	e := newGridEngine(t, width, height, func(x, y int) block.Cell {
		if x == 30 && y < 10 {
			return block.Cell{Height: 0, NSWE: block.NoDirections}
		}
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	})
	finder := New(e, DefaultOptions())

	origin := at(2, 2, 0)
	target := at(55, 2, 0)

	path, _, ok := finder.Find(origin, target)
	if !ok {
		t.Fatal("Find() = no path, want path")
	}

	want := []location.Location{at(31, 10, 0), at(55, 2, 0)}
	if len(path) != len(want) || path[0] != want[0] || path[1] != want[1] {
		t.Fatalf("Find() = %#v, want %#v (collapsed to the gap waypoint plus target)", path, want)
	}
}

// A waypoint that isn't directly reachable from the anchor must survive
// collapse, even past maxSmoothCells: collapseWaypoints only removes a
// waypoint whose CanMove(anchor, next) check actually succeeds.
func TestCollapseWaypointsKeepsWaypointBehindObstacle(t *testing.T) {
	const width, height = 220, 8
	wallX := 100
	e := newGridEngine(t, width, height, func(x, y int) block.Cell {
		if x == wallX && y != 3 {
			return block.Cell{Height: 0, NSWE: block.NoDirections}
		}
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	})
	finder := New(e, DefaultOptions())

	origin := at(0, 0, 0)
	gap := at(wallX, 3, 0)
	path := []location.Location{gap, at(210, 3, 0)}

	got := finder.collapseWaypoints(origin.X, origin.Y, origin.Z, append([]location.Location{}, path...))
	if len(got) != 2 || got[0] != gap || got[1] != path[1] {
		t.Fatalf("collapseWaypoints() = %#v, want the gap waypoint kept: %#v", got, path)
	}
}
