package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// tick lets one position-update interval pass on the clock, firing what
// comes due, then runs the controller's production position update.
func (c *moveClock) tick(controller *Controller) {
	c.in.Advance(PositionUpdateInterval)
	controller.PositionUpdate()
}

// A player's position update walks the queue-clock time since its last
// update, not the nominal interval (PlayerMove.updatePosition, PlayerMove.
// java:214-222 timePassed = Duration.between(_instant, now), _instant = now;
// :246 passedDistance = speed / (1000d / timePassed)). An update run 150 ms
// after the move starts walks 150 ms worth; the next one, on time 50 ms
// later, walks only those 50 ms, so the late update's extra time is not
// walked twice.
func TestPlayerUpdateWalksMeasuredTime(t *testing.T) {
	controller, mover, _, _, clock := newPlayerStepController(t, 100, staticGeo{canMove: true})
	if ok, err := controller.MoveToLocation(location.Location{X: 1000}); err != nil || !ok {
		t.Fatalf("MoveToLocation() = %v, %v; want accepted", ok, err)
	}
	clock.in.Advance(150 * time.Millisecond)
	controller.PositionUpdate()
	if got := mover.Position(); got != (location.Location{X: 15}) {
		t.Fatalf("position after an update 150 ms late = %+v, want X 15 (150 ms at 100/s)", got)
	}
	clock.in.Advance(50 * time.Millisecond)
	controller.PositionUpdate()
	if got := mover.Position(); got != (location.Location{X: 20}) {
		t.Fatalf("position after the next update 50 ms later = %+v, want X 20 (200 ms walked in all)", got)
	}
	clock.tick(controller)
	if got := mover.Position(); got != (location.Location{X: 30}) {
		t.Fatalf("position after an on-time update = %+v, want X 30", got)
	}
}

// An update run 1 ms after a retarget walks 1 ms worth, and one run at the
// same instant counts as 1 ms (PlayerMove.java:218-219: timePassed 0 is 1).
func TestPlayerUpdateAfterRetargetWalksTimeSinceIt(t *testing.T) {
	controller, mover, _, _, clock := newPlayerStepController(t, 1000, staticGeo{canMove: true})
	if ok, err := controller.MoveToLocation(location.Location{X: 5000}); err != nil || !ok {
		t.Fatalf("MoveToLocation() = %v, %v; want accepted", ok, err)
	}
	clock.tick(controller)
	if got := mover.Position(); got != (location.Location{X: 100}) {
		t.Fatalf("position after the first update = %+v, want X 100", got)
	}
	clock.in.Advance(40 * time.Millisecond)
	if ok, err := controller.MoveToLocation(location.Location{X: 140, Y: 5000}); err != nil || !ok {
		t.Fatalf("retarget MoveToLocation() = %v, %v; want accepted", ok, err)
	}
	if got := mover.Position(); got != (location.Location{X: 140}) {
		t.Fatalf("position after the retarget catch-up = %+v, want X 140 (40 ms at 1000/s)", got)
	}
	clock.in.Advance(time.Millisecond)
	controller.PositionUpdate()
	if got := mover.Position(); got != (location.Location{X: 140, Y: 1}) {
		t.Fatalf("position after an update 1 ms after the retarget = %+v, want Y 1", got)
	}
	controller.PositionUpdate()
	if got := mover.Position(); got != (location.Location{X: 140, Y: 2}) {
		t.Fatalf("position after an update at the same instant = %+v, want Y 2 (0 ms counts as 1)", got)
	}
}

// An NPC or summon mover (no SetSpeeds) still walks the nominal interval
// per update, however late the update runs.
func TestNonPlayerUpdateWalksNominalInterval(t *testing.T) {
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	clock := newMoveClock()
	mover.SetQueue(clock.q)
	controller, err := NewController(mover, &knowingFollowSelf{}, &eventLog{})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := controller.MoveToLocation(location.Location{X: 1000}); err != nil || !ok {
		t.Fatalf("MoveToLocation() = %v, %v; want accepted", ok, err)
	}
	clock.in.Advance(150 * time.Millisecond)
	controller.PositionUpdate()
	if got := mover.Position(); got != (location.Location{X: 10}) {
		t.Fatalf("NPC position after an update 150 ms late = %+v, want X 10 (one nominal interval)", got)
	}
	controller.PositionUpdate()
	if got := mover.Position(); got != (location.Location{X: 20}) {
		t.Fatalf("NPC position after an update at the same instant = %+v, want X 20", got)
	}
}
