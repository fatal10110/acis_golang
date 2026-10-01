package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ---- from creature_allocs_test.go ----
// TestCreatureMove_FollowTickAllocs locks in FollowTick's zero-steady-state
// allocation property (#421, #425): the no-op path (target already in range,
// or not following) must stay allocation-free as AI/follow call sites are
// added, and the move-triggering path allocates only what arming the arrival
// timer on the queue costs, plus the arrival closure it captures.
func TestCreatureMove_FollowTickAllocs(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	geo := staticGeo{canMove: true, height: 30}

	t.Run("no-op path", func(t *testing.T) {
		mover, err := NewCreatureMove(origin, 50, geo)
		if err != nil {
			t.Fatal(err)
		}
		mover.SetQueue(newMoveClock().q)
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
		clock := newMoveClock()
		mover.SetQueue(clock.q)
		mover.StartFriendlyFollow(2, 70)
		target := TargetSnapshot{
			ObjectID:        2,
			Known:           true,
			Position:        location.Location{X: 111, Y: 20, Z: 999},
			CollisionRadius: 10.9,
		}

		timerAllocs := testing.AllocsPerRun(1000, func() {
			clock.q.After(time.Second, func() {}).Stop()
		})
		wantAllocsCeiling := timerAllocs + 1 // the arrival closure
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

// ---- from creature_follow_test.go ----
func TestCreatureMove_FollowTickUsesCurrentPosition(t *testing.T) {
	spawn := location.Location{X: 0, Y: 0, Z: 0}
	current := location.Location{X: 100, Y: 0, Z: 0}
	target := TargetSnapshot{
		ObjectID:        2,
		Known:           true,
		Position:        location.Location{X: 129, Y: 0, Z: 0},
		CollisionRadius: 5,
	}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(spawn, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	mover.SetPosition(current)
	mover.StartFriendlyFollow(target.ObjectID, 20)

	ev, moved, err := mover.FollowTick(target, 5)
	if err != nil {
		t.Fatal(err)
	}
	if moved {
		t.Fatalf("FollowTick() moved = true with event %+v", ev)
	}
	if len(geo.moveCalls) != 0 {
		t.Fatalf("CanMove() calls = %+v, want none", geo.moveCalls)
	}
}

func TestCreatureMove_FriendlyFollowTick(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	target := TargetSnapshot{
		ObjectID:        2,
		Position:        location.Location{X: 111, Y: 20, Z: 999},
		CollisionRadius: 10.9,
		Known:           true,
	}
	geo := &recordingGeo{canMove: true, height: 30}
	mover, err := NewCreatureMove(origin, 50, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)

	mover.StartFriendlyFollow(target.ObjectID, 70)
	ev, moved, err := mover.FollowTick(target, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick() moved = false, want true")
	}

	want := event.Move{
		Origin:      origin,
		Destination: location.Location{X: 111, Y: 20, Z: 30},
		Speed:       50,
		Duration:    2100 * time.Millisecond,
	}
	if ev != want {
		t.Fatalf("FollowTick() event = %+v, want %+v", ev, want)
	}
	if got := mover.Destination(); got != want.Destination {
		t.Fatalf("Destination() = %+v, want %+v", got, want.Destination)
	}
	if !mover.Following() {
		t.Fatal("Following() = false, want true")
	}
	if got := mover.FollowInterval(); got != time.Second {
		t.Fatalf("FollowInterval() = %v, want %v", got, time.Second)
	}
}

func TestCreatureMove_FriendlyFollowTickMovesAtExactRange(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	target := TargetSnapshot{
		ObjectID:        2,
		Known:           true,
		Position:        location.Location{X: 100, Y: 20, Z: 30},
		CollisionRadius: 10.9,
	}
	mover, err := NewCreatureMove(origin, 50, &recordingGeo{canMove: true, height: 30})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	mover.StartFriendlyFollow(target.ObjectID, 70)

	ev, moved, err := mover.FollowTick(target, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick() moved = false at the exact follow range, want true")
	}
	if ev.Destination != target.Position {
		t.Fatalf("FollowTick() destination = %+v, want %+v", ev.Destination, target.Position)
	}
}

func TestCreatureMove_FollowTickSkipsWhenTargetDoesNotNeedMove(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	tests := []struct {
		name     string
		target   TargetSnapshot
		start    func(*CreatureMove)
		wantMode FollowMode
	}{
		{
			name: "not following",
			target: TargetSnapshot{
				ObjectID: 2,
				Known:    true,
				Position: location.Location{X: 500, Y: 20, Z: 30},
			},
		},
		{
			name: "unknown friendly target",
			target: TargetSnapshot{
				ObjectID: 2,
				Position: location.Location{X: 500, Y: 20, Z: 30},
			},
			start:    func(m *CreatureMove) { m.StartFriendlyFollow(2, 70) },
			wantMode: FollowFriendly,
		},
		{
			name: "different target snapshot",
			target: TargetSnapshot{
				ObjectID: 3,
				Known:    true,
				Position: location.Location{X: 500, Y: 20, Z: 30},
			},
			start:    func(m *CreatureMove) { m.StartFriendlyFollow(2, 70) },
			wantMode: FollowFriendly,
		},
		{
			name: "friendly target in boat",
			target: TargetSnapshot{
				ObjectID: 2,
				Known:    true,
				InBoat:   true,
				Position: location.Location{X: 500, Y: 20, Z: 30},
			},
			start:    func(m *CreatureMove) { m.StartFriendlyFollow(2, 70) },
			wantMode: FollowFriendly,
		},
		{
			name: "inside collision-adjusted range",
			target: TargetSnapshot{
				ObjectID:        2,
				Known:           true,
				Position:        location.Location{X: 99, Y: 20, Z: 30},
				CollisionRadius: 10.9,
			},
			start:    func(m *CreatureMove) { m.StartFriendlyFollow(2, 70) },
			wantMode: FollowFriendly,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			geo := &recordingGeo{canMove: true, height: 30}
			mover, err := NewCreatureMove(origin, 50, geo)
			if err != nil {
				t.Fatal(err)
			}
			mover.SetQueue(newMoveClock().q)
			if test.start != nil {
				test.start(mover)
			}

			ev, moved, err := mover.FollowTick(test.target, 9.9)
			if err != nil {
				t.Fatal(err)
			}
			if moved {
				t.Fatalf("FollowTick() moved = true with event %+v", ev)
			}
			if ev != (event.Move{}) {
				t.Fatalf("FollowTick() event = %+v, want zero", ev)
			}
			if got := mover.Destination(); got != origin {
				t.Fatalf("Destination() = %+v, want %+v", got, origin)
			}
			if len(geo.moveCalls) != 0 {
				t.Fatalf("CanMove() calls = %+v, want none", geo.moveCalls)
			}
			if got := mover.FollowMode(); got != test.wantMode {
				t.Fatalf("FollowMode() = %v, want %v", got, test.wantMode)
			}
		})
	}
}

func TestCreatureMove_OffensiveFollowTick(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)

	mover.StartOffensiveFollow(9, 40)
	if got := mover.FollowInterval(); got != 500*time.Millisecond {
		t.Fatalf("FollowInterval() = %v, want %v", got, 500*time.Millisecond)
	}

	inRange := TargetSnapshot{ObjectID: 9, Known: true, Position: location.Location{X: 58, Y: 0}, CollisionRadius: 10}
	if ev, moved, err := mover.FollowTick(inRange, 9.9); err != nil || moved || ev != (event.Move{}) {
		t.Fatalf("FollowTick(in range) = event %+v moved %v err %v, want no move", ev, moved, err)
	}

	outside := TargetSnapshot{ObjectID: 9, Known: true, Position: location.Location{X: 59, Y: 0}, CollisionRadius: 10}
	ev, moved, err := mover.FollowTick(outside, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick(outside) moved = false, want true")
	}
	want := event.Move{
		Origin:       origin,
		Destination:  location.Location{X: 59, Y: 0, Z: 0},
		Speed:        100,
		Duration:     600 * time.Millisecond,
		FollowTarget: 9,
		FollowOffset: 40,
	}
	if ev != want {
		t.Fatalf("FollowTick(outside) event = %+v, want %+v", ev, want)
	}
}

func TestCreatureMove_CancelFollow(t *testing.T) {
	geo := &recordingGeo{canMove: true}
	mover, err := NewCreatureMove(location.Location{}, 50, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)

	mover.StartFriendlyFollow(2, 70)
	mover.CancelFollow()

	if mover.Following() {
		t.Fatal("Following() = true, want false")
	}
	if got := mover.FollowMode(); got != FollowNone {
		t.Fatalf("FollowMode() = %v, want %v", got, FollowNone)
	}
	if got := mover.FollowInterval(); got != 0 {
		t.Fatalf("FollowInterval() = %v, want 0", got)
	}
}
