package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestCreatureMove_MoveToLocationRoutesThroughPathfoundWaypoints covers the
// tier-2 case: a blocked direct line that the geopath resolves into three
// segments. The accepted request walks each segment in turn, broadcasts a
// per-segment destination, and fires the arrived hook exactly once — at the
// final segment's completion — not once per intermediate waypoint.
func TestCreatureMove_MoveToLocationRoutesThroughPathfindWaypoints(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 30}
	// The pathfinder returns corners plus the final cell, omitting the
	// origin: three cells → three segments to walk sequentially.
	waypoints := []location.Location{
		{X: 50, Y: 0, Z: 30},
		{X: 50, Y: 50, Z: 30},
		{X: 100, Y: 50, Z: 30},
	}
	geo := &recordingGeo{
		canMove:    false, // direct line blocked → tier 2 pathfinding
		height:     30,
		findPath:   waypoints,
		findPathOK: true,
	}
	mover, err := NewCreatureMove(origin, 50, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	event, err := mover.MoveToLocation(location.Location{X: 100, Y: 50, Z: 30})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v, want nil", err)
	}
	// The active destination is the first geopath entry; the remaining two
	// are queued inside CreatureMove.
	wantFirst := location.Location{X: 50, Y: 0, Z: 30}
	if event.Destination != wantFirst {
		t.Fatalf("event.Destination = %+v, want %+v (first waypoint)", event.Destination, wantFirst)
	}
	if got := mover.Destination(); got != wantFirst {
		t.Fatalf("Destination() = %+v, want %+v", got, wantFirst)
	}
	if got := len(geo.findPathCalls); got != 1 {
		t.Fatalf("FindPath() calls = %d, want 1", got)
	}
	if geo.findPathCalls[0].origin != origin || geo.findPathCalls[0].target != (location.Location{X: 100, Y: 50, Z: 30}) {
		t.Fatalf("FindPath() args = %+v -> %+v, want origin -> target", geo.findPathCalls[0].origin, geo.findPathCalls[0].target)
	}
	// The partial-fallback query never runs when pathfinding succeeds.
	if got := len(geo.validLocationCalls); got != 0 {
		t.Fatalf("ValidLocation() calls = %d, want 0", got)
	}

	// Walk each segment by firing its arrival timer. Each fire() runs
	// finishLocked: the first two advance to the next waypoint (returning
	// nil, no arrival), and the third exhausts the queue and fires the
	// arrived hook once. Every segment shares the same duration since the
	// geometry is uniform, so firing event.Duration repeatedly is correct.
	for i := range waypoints {
		if !mover.Moving() {
			t.Fatalf("Moving() = false before firing segment %d", i)
		}
		clock.fire(event.Duration)
	}

	// After the last segment finishes, the arrived hook fires exactly once,
	// and the actor rests at the final waypoint.
	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls = %d, want 1 (final segment only)", arrivedCalls)
	}
	if mover.Moving() {
		t.Fatal("Moving() = true after final segment, want false")
	}
	if got := mover.Position(); got != waypoints[len(waypoints)-1] {
		t.Fatalf("Position() = %+v, want %+v (final waypoint)", got, waypoints[len(waypoints)-1])
	}
}

// TestCreatureMove_MoveToLocationPartialFallbackWalksPartialRoute covers the
// tier-3 case: blocked direct line, no pathfind route, but a partial-progress
// fall-back point exists further along the line. The request succeeds with
// the fall-back as the destination and no waypoint queue.
func TestCreatureMove_MoveToLocationPartialFallbackWalksPartialRoute(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 30}
	fallback := location.Location{X: 25, Y: 0, Z: 30}
	geo := &recordingGeo{
		canMove:       false,
		height:        30,
		findPath:      nil,
		findPathOK:    false,
		validLocation: fallback,
	}
	mover, err := NewCreatureMove(origin, 50, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	event, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 30})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v, want nil (tier 3 fall-back)", err)
	}
	if event.Destination != fallback {
		t.Fatalf("event.Destination = %+v, want fall-back %+v", event.Destination, fallback)
	}
	if got := mover.Destination(); got != fallback {
		t.Fatalf("Destination() = %+v, want %+v", got, fallback)
	}
	if got := len(geo.validLocationCalls); got != 1 {
		t.Fatalf("ValidLocation() calls = %d, want 1", got)
	}

	clock.fire(event.Duration)
	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls = %d, want 1", arrivedCalls)
	}
	if got := mover.Position(); got != fallback {
		t.Fatalf("Position() = %+v, want fall-back %+v", got, fallback)
	}
}

// TestCreatureMove_MoveToLocationNoProgressFallbackStartsZeroDistanceArrival
// covers the tier-3 fully-blocked edge: the straight line is blocked,
// pathfinding finds no route, and the partial fallback resolves to the origin.
func TestCreatureMove_MoveToLocationNoProgressFallbackStartsZeroDistanceArrival(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 30}
	prior := location.Location{X: -42, Y: -42, Z: 30}
	geo := &recordingGeo{
		canMove:    false,
		height:     30,
		findPath:   nil,
		findPathOK: false,
		// ValidLocation left zero → stub returns the call's origin.
	}
	mover, err := NewCreatureMove(origin, 50, geo)
	if err != nil {
		t.Fatal(err)
	}
	// Seed an in-flight destination so the new request must replace it.
	mover.destination = prior
	mover.moving = true

	event, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 30})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v, want nil", err)
	}
	if want := (Event{Origin: origin, Destination: origin, Speed: 50}); event != want {
		t.Fatalf("MoveToLocation() event = %+v, want %+v", event, want)
	}
	if got := mover.Destination(); got != origin {
		t.Fatalf("Destination() = %+v, want origin %+v", got, origin)
	}
	if !mover.Moving() {
		t.Fatal("Moving() = false, want zero-distance arrival pending")
	}
}
