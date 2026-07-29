package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestCreatureMove_FollowTickAllocs locks in FollowTick's zero-steady-state
// allocation property (#421, #425): the no-op path (target already in range,
// or not following) must stay allocation-free as AI/follow call sites are
// added, and the move-triggering path's ceiling is the one allocation that's
// inherent to scheduling a new arrival timer through the afterFunc
// indirection (the closure captured for time.AfterFunc-shaped calls always
// escapes to heap, since the compiler can't prove an indirect call won't
// retain it).
func TestCreatureMove_FollowTickAllocs(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	geo := staticGeo{canMove: true, height: 30}

	t.Run("no-op path", func(t *testing.T) {
		mover, err := NewCreatureMove(origin, 50, geo)
		if err != nil {
			t.Fatal(err)
		}
		mover.afterFunc = func(time.Duration, func()) scheduledTimer { return noAllocTimer{} }
		target := TargetSnapshot{ObjectID: 2, Known: true, Position: location.Location{X: 500, Y: 20, Z: 30}}

		allocs := testing.AllocsPerRun(1000, func() {
			if _, moved, err := mover.FollowTick(target, 9.9); err != nil || moved {
				t.Fatalf("FollowTick() = moved %v err %v, want no move", moved, err)
			}
		})
		if allocs != 0 {
			t.Fatalf("FollowTick() no-op path allocs/run = %v, want 0", allocs)
		}
	})

	t.Run("move-triggering path", func(t *testing.T) {
		mover, err := NewCreatureMove(origin, 50, geo)
		if err != nil {
			t.Fatal(err)
		}
		mover.afterFunc = func(time.Duration, func()) scheduledTimer { return noAllocTimer{} }
		mover.StartFriendlyFollow(2, 70)
		target := TargetSnapshot{
			ObjectID:        2,
			Known:           true,
			Position:        location.Location{X: 111, Y: 20, Z: 999},
			CollisionRadius: 10.9,
		}

		// One allocation: the closure scheduling the arrival timer, captured
		// for time.AfterFunc-shaped call through the afterFunc indirection.
		const wantAllocsCeiling = 1
		allocs := testing.AllocsPerRun(1000, func() {
			if _, moved, err := mover.FollowTick(target, 9.9); err != nil || !moved {
				t.Fatalf("FollowTick() = moved %v err %v, want a move", moved, err)
			}
		})
		if allocs != wantAllocsCeiling {
			t.Fatalf("FollowTick() move-triggering path allocs/run = %v, want %v", allocs, wantAllocsCeiling)
		}
	})
}
