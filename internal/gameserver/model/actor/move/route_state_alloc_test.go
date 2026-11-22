//go:build !race

package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// A mover that is never asked to move carries no route state, whatever
// reads, stops and debug toggles it sees; the first request creates it and
// later stops keep it.
func TestStationaryMoverHoldsNoRouteState(t *testing.T) {
	origin := location.Location{X: 100, Y: 200, Z: -50}
	mover, err := NewCreatureMove(origin, 120, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)

	_ = mover.Position()
	mover.SetPosition(origin)
	mover.CancelMove()
	if _, moving := mover.UpdatePosition(100 * time.Millisecond); moving {
		t.Fatal("stationary mover reports moving")
	}
	if pawn, _ := mover.walkingPawn(); pawn != nil {
		t.Fatalf("walkingPawn() = %v, want none", pawn)
	}
	if n := mover.catchUpCount(); n != 0 {
		t.Fatalf("catchUpCount() = %d, want 0", n)
	}
	mover.ToggleDebug(false)
	mover.flushDebug()
	mover.ToggleDebug(false)
	mover.SetSpeed(150)
	if mover.rs != nil {
		t.Fatal("never-moved mover holds route state")
	}

	if _, err := mover.MoveToLocation(location.Location{X: 900, Y: 200, Z: -50}); err != nil {
		t.Fatal(err)
	}
	rs := mover.rs
	if rs == nil {
		t.Fatal("move request left no route state")
	}
	mover.CancelMove()
	if mover.rs != rs {
		t.Fatal("stop dropped the route state, want it kept for the next move")
	}
}

// A position update of a walk already in flight allocates nothing.
func TestInFlightPositionUpdateAllocatesNothing(t *testing.T) {
	mover, err := NewCreatureMove(location.Location{}, 120, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	if _, err := mover.MoveToLocation(location.Location{X: 60000}); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(50, func() {
		if _, moving := mover.UpdatePosition(PositionUpdateInterval); !moving {
			panic("walk ended early")
		}
	})
	if allocs != 0 {
		t.Fatalf("position update allocations = %v, want 0", allocs)
	}
}
