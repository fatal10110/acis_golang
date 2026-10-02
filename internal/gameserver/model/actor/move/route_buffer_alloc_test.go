//go:build !race

package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// After warmup a blocked request that path-finds allocates nothing for its
// route: the search runs into the mover's own route storage, so the routed
// request costs no more allocations than a request whose straight line is
// open.
func TestRoutedMoveReusesRouteStorage(t *testing.T) {
	mover, _, _ := newRoutedMover(t, cellAt(1, 1))
	routed, open := cellAt(6, 2), cellAt(1, 6)
	request := func(target location.Location, want pathFindResult) {
		_, outcome, err := mover.MoveToLocationWithPathOutcome(target)
		if err != nil || outcome != want {
			panic("unexpected request outcome")
		}
	}
	routedAllocs := testing.AllocsPerRun(50, func() { request(routed, pathRouted) })
	openAllocs := testing.AllocsPerRun(50, func() { request(open, pathDirect) })
	t.Logf("allocations per request: routed %v, open %v", routedAllocs, openAllocs)
	if routedAllocs > openAllocs {
		t.Fatalf("routed request allocations = %v, open request = %v; want no route allocations", routedAllocs, openAllocs)
	}

	allocs := testing.AllocsPerRun(50, func() {
		mover.mu.Lock()
		_, _, outcome := mover.resolvePathLocked(routed)
		mover.mu.Unlock()
		if outcome != pathRouted {
			panic("route not found")
		}
	})
	if allocs != 0 {
		t.Fatalf("route resolution allocations = %v, want 0", allocs)
	}
}
