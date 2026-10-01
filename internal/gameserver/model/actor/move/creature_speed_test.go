package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestCreatureMove_SetSpeedRetimesInFlightArrival: a speed change mid-leg
// re-times the arrival for the distance left, so a slowed walk does not
// snap to its destination at the old arrival time, and a stalled one (zero
// speed) never arrives until its speed returns.
func TestCreatureMove_SetSpeedRetimesInFlightArrival(t *testing.T) {
	geo := &recordingGeo{canMove: true, height: 30}
	mover, err := NewCreatureMove(location.Location{X: 0, Y: 0, Z: 30}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := newMoveClock()
	mover.SetQueue(clock.q)
	arrived := 0
	mover.setOwner(&hookOwner{onArrived: func() { arrived++ }})

	ev, err := mover.MoveToLocation(location.Location{X: 200, Y: 0, Z: 30})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Duration != 2*time.Second {
		t.Fatalf("leg duration = %v, want 2s at speed 100", ev.Duration)
	}
	clock.in.Advance(time.Second)
	mover.UpdatePosition(time.Second) // 100 units walked, 100 left
	mover.SetSpeed(50)
	clock.in.Advance(time.Second) // the old 2s arrival time
	if arrived != 0 {
		t.Fatal("slowed walk arrived at the old arrival time")
	}
	mover.SetSpeed(0)
	clock.in.Advance(time.Minute)
	if arrived != 0 || !mover.Moving() {
		t.Fatalf("stalled walk arrived %d times, moving %v; want still moving", arrived, mover.Moving())
	}
	mover.SetSpeed(50)
	clock.in.Advance(2 * time.Second)
	if arrived != 1 {
		t.Fatalf("arrived hook calls = %d after the remaining 100 units at speed 50, want 1", arrived)
	}
	if got, want := mover.Position(), (location.Location{X: 200, Y: 0, Z: 30}); got != want {
		t.Fatalf("Position() = %+v, want %+v", got, want)
	}
}

// TestCreatureMove_StallDropsDequeuedArrival: an arrival callback already
// dequeued when SetSpeed(0) stalls the leg (the timer fired on the owner's
// queue while a foreign queue held mu) must not finish the stalled leg.
func TestCreatureMove_StallDropsDequeuedArrival(t *testing.T) {
	geo := &recordingGeo{canMove: true, height: 30}
	mover, err := NewCreatureMove(location.Location{X: 0, Y: 0, Z: 30}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := newMoveClock()
	mover.SetQueue(clock.q)
	arrived := 0
	mover.setOwner(&hookOwner{onArrived: func() { arrived++ }})

	if _, err := mover.MoveToLocation(location.Location{X: 200, Y: 0, Z: 30}); err != nil {
		t.Fatal(err)
	}
	clock.in.Advance(time.Second)
	mover.UpdatePosition(time.Second)
	before := mover.Position()

	mover.mu.Lock()
	staleSeq := mover.moveSeq
	mover.mu.Unlock()

	mover.SetSpeed(0)
	mover.onArrive(staleSeq) // the arrival that was already running

	if arrived != 0 {
		t.Fatalf("stalled leg arrived %d times via a dequeued callback, want 0", arrived)
	}
	if !mover.Moving() {
		t.Fatal("stalled leg stopped moving, want still moving")
	}
	if got := mover.Position(); got != before {
		t.Fatalf("Position() = %+v after stale arrival, want unchanged %+v", got, before)
	}
}

// A mover with a start speed covers the first five position updates of a
// move at it (PlayerMove.updatePosition: getRealMoveSpeed(_moveTimeStamp <=
// 5)), the rest at its speed; retargeting a walk in flight keeps the count,
// and only a stopped move restarts it (PlayerMove.cancelMoveTask).
func TestCreatureMoveStartSpeedCoversFirstFiveUpdates(t *testing.T) {
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	mover.SetSpeeds(200, 100)

	if _, err := mover.MoveToLocation(location.Location{X: 1000}); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		mover.UpdatePosition(PositionUpdateInterval)
		if got, want := mover.Position().X, 10*(i+1); got != want {
			t.Fatalf("X after update %d = %d, want %d at the start speed", i+1, got, want)
		}
	}
	mover.UpdatePosition(PositionUpdateInterval)
	if got := mover.Position().X; got != 70 {
		t.Fatalf("X after update 6 = %d, want 70 at the full speed", got)
	}

	if _, err := mover.MoveToLocation(location.Location{X: 70, Y: 1000}); err != nil {
		t.Fatal(err)
	}
	mover.UpdatePosition(PositionUpdateInterval)
	if got := mover.Position(); got != (location.Location{X: 70, Y: 20}) {
		t.Fatalf("position after the retarget = %+v, want (70, 20): the full speed goes on", got)
	}

	mover.CancelMove()
	if _, err := mover.MoveToLocation(location.Location{X: 1000, Y: 20}); err != nil {
		t.Fatal(err)
	}
	mover.UpdatePosition(PositionUpdateInterval)
	if got := mover.Position(); got != (location.Location{X: 80, Y: 20}) {
		t.Fatalf("position after a new move = %+v, want (80, 20): the start speed again", got)
	}
}

// The arrival timer counts the slower start: 300 units at a 100 start speed
// and a 200 speed take 5 updates for the first 50 and 13 for the other 250,
// 1.8 s instead of the 1.5 s the full speed alone would take.
func TestCreatureMoveStartSpeedTimesTheArrival(t *testing.T) {
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	clock := newMoveClock()
	mover.SetQueue(clock.q)
	mover.SetSpeeds(200, 100)

	ev, err := mover.MoveToLocation(location.Location{X: 300})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Duration != 1800*time.Millisecond {
		t.Fatalf("move duration = %v, want 1.8s", ev.Duration)
	}
	clock.in.Advance(1700 * time.Millisecond)
	if !mover.Moving() {
		t.Fatal("arrived before the slower start was covered")
	}
	clock.in.Advance(100 * time.Millisecond)
	if mover.Moving() || mover.Position() != (location.Location{X: 300}) {
		t.Fatalf("at 1.8s: moving %v at %+v, want arrived at X 300", mover.Moving(), mover.Position())
	}

	// A short walk ends inside the start updates: 30 units, 3 updates.
	if ev, err = mover.MoveToLocation(location.Location{X: 330}); err != nil {
		t.Fatal(err)
	}
	if ev.Duration != 300*time.Millisecond {
		t.Fatalf("short move duration = %v, want 300ms", ev.Duration)
	}
}

// Without SetSpeeds a mover (an NPC, a summon) moves at its speed from the
// first update.
func TestCreatureMoveWithoutStartSpeedMovesAtFullSpeed(t *testing.T) {
	mover, err := NewCreatureMove(location.Location{}, 200, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	if _, err := mover.MoveToLocation(location.Location{X: 1000}); err != nil {
		t.Fatal(err)
	}
	mover.UpdatePosition(PositionUpdateInterval)
	if got := mover.Position().X; got != 20 {
		t.Fatalf("X after the first update = %d, want 20", got)
	}
}
