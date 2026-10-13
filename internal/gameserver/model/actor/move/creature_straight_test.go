package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// A straight walk (CreatureMove.moveToLocation with path-finding off) heads
// for its point across a closed line, with no route search or fall-back
// point, and stops blocked on the last open step short of the wall, whether
// the position updates or the arrival timer run its steps.
func TestStraightWalkStopsAtWall(t *testing.T) {
	for _, run := range []struct {
		name  string
		timer bool
	}{
		{name: "position updates"},
		{name: "arrival timer", timer: true},
	} {
		t.Run(run.name, func(t *testing.T) {
			geo := &crossWallGeo{staticGeo: staticGeo{canMove: true}, wall: 205, up: true}
			mover, clock := newTestMover(t, geo)
			target := location.Location{X: 300}
			ev, err := mover.MoveStraightToLocation(target)
			if err != nil {
				t.Fatal(err)
			}
			if ev.Destination != target {
				t.Fatalf("walk destination = %+v, want straight for %+v", ev.Destination, target)
			}
			if run.timer {
				clock.in.Advance(time.Minute)
			} else {
				updateUntilStopped(t, mover)
			}
			if mover.Moving() {
				t.Fatal("walk still under way")
			}
			if want := (location.Location{X: 200}); mover.Position() != want {
				t.Fatalf("walk stopped at %+v, want %+v short of the wall", mover.Position(), want)
			}
		})
	}
}
