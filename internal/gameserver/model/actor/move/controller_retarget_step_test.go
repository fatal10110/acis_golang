package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// retargetSelf is a player-shaped mover that records every movement step
// synced to its world presence and whose visibility the test controls.
type retargetSelf struct {
	knowingFollowSelf
	hidden bool
	synced []location.Location
}

func (s *retargetSelf) Visible() bool { return !s.hidden }

func (s *retargetSelf) SyncPosition(pos location.Location) {
	s.synced = append(s.synced, pos)
	s.knowingFollowSelf.SyncPosition(pos)
}

// switchGeo is open terrain whose straight lines the test can close.
type switchGeo struct {
	staticGeo
	closed *bool
}

func (g switchGeo) CanMove(int, int, int, int, int, int) bool { return !*g.closed }

func newRetargetController(t *testing.T, speed float64) (*Controller, *CreatureMove, *retargetSelf, *moveClock, *bool) {
	t.Helper()
	closed := new(bool)
	mover, err := NewCreatureMove(location.Location{}, speed, switchGeo{closed: closed})
	if err != nil {
		t.Fatal(err)
	}
	clock := newMoveClock()
	mover.SetQueue(clock.q)
	mover.SetSpeeds(speed, speed)
	self := &retargetSelf{}
	controller, err := NewController(mover, self, &eventLog{})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := controller.MoveToLocation(location.Location{X: 5000}); err != nil || !ok {
		t.Fatalf("MoveToLocation() = %v, %v; want accepted", ok, err)
	}
	if len(self.synced) != 0 {
		t.Fatalf("a fresh walk synced %v, want no step", self.synced)
	}
	return controller, mover, self, clock, closed
}

// A retarget of a walk in flight runs updatePosition(true) first
// (PlayerMove.java:61 and :114), which ends in revalidateZone(false) (:321)
// once it is past the visibility, pawn and geodata checks: a movement step
// even when less than one cell was covered. A catch-up that stays on the
// same cell syncs that cell; one that changes cell syncs it once.
func TestRetargetCatchUpIsAStep(t *testing.T) {
	controller, mover, self, clock, _ := newRetargetController(t, 100)
	clock.in.Advance(time.Millisecond)
	if ok, err := controller.MoveToLocation(location.Location{Y: 5000}); err != nil || !ok {
		t.Fatalf("same-cell retarget = %v, %v; want accepted", ok, err)
	}
	if want := []location.Location{{}}; len(self.synced) != 1 || self.synced[0] != want[0] {
		t.Fatalf("same-cell retarget synced %v, want %v", self.synced, want)
	}

	clock.in.Advance(100 * time.Millisecond)
	if !controller.MoveToPawn(&followTarget{x: 5000}, 0) {
		t.Fatal("cell-changing retarget refused")
	}
	at := mover.Position()
	if at == (location.Location{}) {
		t.Fatal("100 ms catch-up stayed on the start cell")
	}
	if len(self.synced) != 2 || self.synced[1] != at {
		t.Fatalf("cell-changing retarget synced %v, want one more step at %+v", self.synced, at)
	}
}

// A catch-up whose line is closed leaves the walker in place, blocked,
// before revalidateZone (PlayerMove.java:285-289): no step.
func TestBlockedRetargetCatchUpIsNoStep(t *testing.T) {
	controller, mover, self, clock, closed := newRetargetController(t, 100)
	clock.in.Advance(100 * time.Millisecond)
	*closed = true
	controller.MoveToLocation(location.Location{Y: 5000})
	if len(self.synced) != 0 || mover.Position() != (location.Location{}) {
		t.Fatalf("blocked catch-up synced %v at %+v, want no step", self.synced, mover.Position())
	}
}

// updatePosition returns before stepping while the actor is not visible or
// does not know the request's pawn (PlayerMove.java:207-212; moveToPawn sets
// the new pawn first, :57): the walker stays where it stands and takes no
// step.
func TestHeldRetargetCatchUpIsNoStep(t *testing.T) {
	t.Run("hidden", func(t *testing.T) {
		controller, mover, self, clock, _ := newRetargetController(t, 100)
		self.hidden = true
		clock.in.Advance(100 * time.Millisecond)
		controller.MoveToLocation(location.Location{Y: 5000})
		if len(self.synced) != 0 || mover.Position() != (location.Location{}) {
			t.Fatalf("hidden catch-up synced %v at %+v, want no step", self.synced, mover.Position())
		}
	})
	t.Run("unknown pawn", func(t *testing.T) {
		controller, mover, self, clock, _ := newRetargetController(t, 100)
		self.forgot = true
		clock.in.Advance(100 * time.Millisecond)
		controller.MoveToPawn(&followTarget{y: 5000}, 0)
		if len(self.synced) != 0 || mover.Position() != (location.Location{}) {
			t.Fatalf("catch-up toward an unknown pawn synced %v at %+v, want no step", self.synced, mover.Position())
		}
	})
}
