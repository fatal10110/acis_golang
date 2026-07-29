package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestCreatureMove_FollowTickUsesCurrentPosition(t *testing.T) {
	spawn := location.Location{X: 0, Y: 0, Z: 0}
	current := location.Location{X: 100, Y: 0, Z: 0}
	target := TargetSnapshot{
		ObjectID:        2,
		Known:           true,
		Position:        location.Location{X: 130, Y: 0, Z: 0},
		CollisionRadius: 5,
	}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(spawn, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetPosition(current)
	mover.StartFriendlyFollow(target.ObjectID, 20)

	event, moved, err := mover.FollowTick(target, 5)
	if err != nil {
		t.Fatal(err)
	}
	if moved {
		t.Fatalf("FollowTick() moved = true with event %+v", event)
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

	mover.StartFriendlyFollow(target.ObjectID, 70)
	event, moved, err := mover.FollowTick(target, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick() moved = false, want true")
	}

	want := Event{
		Origin:      origin,
		Destination: location.Location{X: 111, Y: 20, Z: 30},
		Speed:       50,
		Duration:    2100 * time.Millisecond,
	}
	if event != want {
		t.Fatalf("FollowTick() event = %+v, want %+v", event, want)
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
				Position:        location.Location{X: 100, Y: 20, Z: 30},
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
			if test.start != nil {
				test.start(mover)
			}

			event, moved, err := mover.FollowTick(test.target, 9.9)
			if err != nil {
				t.Fatal(err)
			}
			if moved {
				t.Fatalf("FollowTick() moved = true with event %+v", event)
			}
			if event != (Event{}) {
				t.Fatalf("FollowTick() event = %+v, want zero", event)
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

	mover.StartOffensiveFollow(9, 40)
	if got := mover.FollowInterval(); got != 500*time.Millisecond {
		t.Fatalf("FollowInterval() = %v, want %v", got, 500*time.Millisecond)
	}

	inRange := TargetSnapshot{ObjectID: 9, Known: true, Position: location.Location{X: 59, Y: 0}, CollisionRadius: 10}
	if event, moved, err := mover.FollowTick(inRange, 9.9); err != nil || moved || event != (Event{}) {
		t.Fatalf("FollowTick(in range) = event %+v moved %v err %v, want no move", event, moved, err)
	}

	outside := TargetSnapshot{ObjectID: 9, Known: true, Position: location.Location{X: 60, Y: 0}, CollisionRadius: 10}
	event, moved, err := mover.FollowTick(outside, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick(outside) moved = false, want true")
	}
	want := Event{
		Origin:       origin,
		Destination:  location.Location{X: 60, Y: 0, Z: 0},
		Speed:        100,
		Duration:     600 * time.Millisecond,
		FollowTarget: 9,
		FollowOffset: 40,
	}
	if event != want {
		t.Fatalf("FollowTick(outside) event = %+v, want %+v", event, want)
	}
}

func TestCreatureMove_CancelFollow(t *testing.T) {
	geo := &recordingGeo{canMove: true}
	mover, err := NewCreatureMove(location.Location{}, 50, geo)
	if err != nil {
		t.Fatal(err)
	}

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
