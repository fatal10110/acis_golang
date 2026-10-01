package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// The reference has no arrival timer: a pawn walk only ends on a position
// update (CreatureMove.updatePosition, CreatureMove.java:387; PlayerMove.
// updatePosition, PlayerMove.java:323), at the first step strictly within
// the offset of where the pawn stands then. The arrival timer standing in
// when no update got there first ends the walk there too.

// A chase pawn walk whose arrival timer fires before the update that would
// end it stops within the offset of the pawn, not on the pawn's cell the
// destination holds: the same cell the updates stop at
// (TestControllerChaseStopsWithinOffsetOfPawnWithoutReaim).
func TestChaseArrivalTimerStopsWithinOffsetOfPawn(t *testing.T) {
	tests := []struct {
		name  string
		moved location.Location
		want  location.Location
	}{
		{name: "standing target", moved: location.Location{X: 300}, want: location.Location{X: 270}},
		{name: "target stepped beside the path", moved: location.Location{X: 150, Y: 30}, want: location.Location{X: 130}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, mover, sink, clock := newChaseController(t, &summonChaseSelf{}, staticGeo{canMove: true})
			target := &followTarget{x: 300}
			if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
				t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the chase", following, err)
			}
			for range 5 {
				controller.PositionUpdate()
			}
			target.x, target.y = tt.moved.X, tt.moved.Y
			// The updates stall; only the leg's arrival timer runs.
			clock.in.Advance(time.Minute)
			if mover.Moving() {
				t.Fatalf("chase still under way at %+v after its arrival timer", mover.Position())
			}
			if got := mover.Position(); got != tt.want {
				t.Fatalf("arrival timer ended the chase at %+v, want %+v (the first step within 40 of %+v)", got, tt.want, tt.moved)
			}
			if got := sink.arrivals(); got != 1 {
				t.Fatalf("Arrived events = %d, want 1", got)
			}
		})
	}
}

// A player's tracking pawn walk whose pawn moved since the last update
// ends, on the arrival timer, on the cell the position updates would end it
// on: re-aimed at where the pawn stands now and at the first step strictly
// within the offset of it, not the offset short of a stale destination.
func TestTrackingArrivalTimerEndsWhereUpdatesWould(t *testing.T) {
	moved := location.Location{X: 300, Y: 100}
	run := func(t *testing.T, timerOnly bool) location.Location {
		t.Helper()
		controller, mover, _, sink, clock := newPlayerStepController(t, 100, staticGeo{canMove: true})
		pawn := &followTarget{x: 300}
		if !controller.MoveToPawn(pawn, 40) {
			t.Fatal("MoveToPawn() not accepted")
		}
		for range 10 {
			clock.tick(controller)
		}
		pawn.x, pawn.y = moved.X, moved.Y
		for i := 0; mover.Moving(); i++ {
			if i == 100 {
				t.Fatalf("walk still under way at %+v", mover.Position())
			}
			if timerOnly {
				clock.in.Advance(PositionUpdateInterval)
			} else {
				clock.tick(controller)
			}
		}
		if got := sink.arrivals(); got != 1 {
			t.Fatalf("Arrived events = %d, want 1", got)
		}
		return mover.Position()
	}
	updates := run(t, false)
	timer := run(t, true)
	if timer != updates {
		t.Fatalf("arrival timer ended the walk at %+v, the position updates at %+v", timer, updates)
	}
	if !timer.In2DRadius(moved, 40) || timer.In2DRadius(moved, 30) {
		t.Fatalf("walk ended at %+v, %.1f from the pawn; want the first step within 40", timer, timer.Distance2D(moved))
	}
}

// The arrival timer stands for the updates it was armed for and no more: a
// pawn that moved far away since does not pull the walker the whole way in
// one go. The walk goes on, re-timed, and ends within the offset of the
// pawn once the time to get there has passed.
func TestTrackingArrivalTimerWalksOnlyItsUpdates(t *testing.T) {
	controller, mover, _, sink, clock := newPawnWalkController(t)
	pawn := &followTarget{x: 300}
	if !controller.MoveToPawn(pawn, 100) {
		t.Fatal("MoveToPawn() not accepted")
	}
	pawn.x = 2000
	clock.in.Advance(2100 * time.Millisecond)
	if got := mover.Position(); !mover.Moving() || got != (location.Location{X: 210}) {
		t.Fatalf("after the first timer at %+v (moving %v), want under way at X 210 (21 updates of 10)", got, mover.Moving())
	}
	clock.in.Advance(time.Minute)
	if got := mover.Position(); mover.Moving() || got != (location.Location{X: 1910}) {
		t.Fatalf("walk at %+v (moving %v), want stopped at X 1910, the first step within 100 of the pawn", got, mover.Moving())
	}
	if got := sink.arrivals(); got != 1 {
		t.Fatalf("Arrived events = %d, want 1", got)
	}
}

// The steps the arrival timer takes for a tracking pawn walk turn the
// walker like the updates' steps do (PlayerMove.updatePosition,
// PlayerMove.java:291-293 setHeadingTo(nextX, nextY) when _pawn != null): a
// walk that re-aims at a pawn that stepped aside ends facing along its last
// step, not the walk's starting heading.
func TestTrackingArrivalTimerTurnsAlongItsLastStep(t *testing.T) {
	controller, mover, self, _, clock := newPlayerStepController(t, 100, staticGeo{canMove: true})
	pawn := &followTarget{x: 300}
	if !controller.MoveToPawn(pawn, 40) {
		t.Fatal("MoveToPawn() not accepted")
	}
	for range 10 {
		clock.tick(controller)
	}
	pawn.x, pawn.y = 100, 300
	self.headings = nil
	clock.in.Advance(time.Minute)
	if mover.Moving() {
		t.Fatalf("walk still under way at %+v", mover.Position())
	}
	end := mover.Position()
	if !end.In2DRadius(location.Location{X: 100, Y: 300}, 40) {
		t.Fatalf("walk ended at %+v, want within 40 of the pawn at (100, 300)", end)
	}
	if len(self.headings) == 0 {
		t.Fatal("arrival timer steps turned the walker no way")
	}
	// The last step heads up the Y axis toward the pawn standing straight
	// ahead of it.
	if got, want := self.headings[len(self.headings)-1], (location.Location{X: 100}).HeadingTo(location.Location{X: 100, Y: 300}); got != want {
		t.Fatalf("heading after the timer = %d, want %d along the last step toward the pawn", got, want)
	}
}

// A pawn walk whose pawn the walker no longer knows ends where the walker
// stands, as an arrival, on the arrival timer as on the position update
// (TestControllerPawnWalkEndsOnUnknownTrackedPawn): the timer does not walk
// on toward a pawn the walker lost.
func TestPawnWalkArrivalTimerEndsOnUnknownPawn(t *testing.T) {
	state := world.New()
	self := &trackedWalkSelf{}
	state.Spawn(self, 0, 0, 0, 0)
	pawn := &trackedPawn{}
	state.Spawn(pawn, 500, 0, 0, 0)

	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	clock := newMoveClock()
	mover.SetQueue(clock.q)
	sink := &eventLog{}
	controller, err := NewController(mover, self, sink)
	if err != nil {
		t.Fatal(err)
	}
	if !controller.MoveToPawn(pawn, 100) {
		t.Fatal("MoveToPawn() = false, want the walk accepted")
	}
	for range 3 {
		controller.PositionUpdate()
	}
	state.Despawn(pawn)
	clock.in.Advance(time.Minute)
	if mover.Moving() {
		t.Fatal("walk still under way after its arrival timer")
	}
	if got := mover.Position(); got != (location.Location{X: 30}) {
		t.Fatalf("position = %+v, want the walk left where it stood (X 30)", got)
	}
	if got := sink.arrivals(); got != 1 {
		t.Fatalf("Arrived events = %d, want 1", got)
	}
}
