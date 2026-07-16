package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type fakeSelf struct {
	x, y, z    int
	radius     float64
	broadcasts []Event
	stopCalls  int
}

func (f *fakeSelf) Position() (int, int, int) { return f.x, f.y, f.z }
func (f *fakeSelf) CollisionRadius() float64  { return f.radius }
func (f *fakeSelf) BroadcastMove(event Event) { f.broadcasts = append(f.broadcasts, event) }
func (f *fakeSelf) BroadcastStop()            { f.stopCalls++ }

type fakeTarget struct {
	id      int32
	x, y, z int
	radius  float64
}

func (f *fakeTarget) ObjectID() int32           { return f.id }
func (f *fakeTarget) SiegeGuard() bool          { return false }
func (f *fakeTarget) AlikeDead() bool           { return false }
func (f *fakeTarget) Position() (int, int, int) { return f.x, f.y, f.z }
func (f *fakeTarget) CollisionRadius() float64  { return f.radius }

func newTestController(t *testing.T, self *fakeSelf) *Controller {
	t.Helper()
	cm, err := NewCreatureMove(location.Location{X: self.x, Y: self.y, Z: self.z}, 100, &recordingGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewController(cm, self)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestControllerRejectsInvalidDependencies(t *testing.T) {
	cm, err := NewCreatureMove(location.Location{}, 100, &recordingGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewController(nil, &fakeSelf{}); err == nil {
		t.Fatal("NewController() error = nil, want error for nil move")
	}
	if _, err := NewController(cm, nil); err == nil {
		t.Fatal("NewController() error = nil, want error for nil self")
	}
}

func TestControllerMaybeStartOffensiveFollowStartsWhenOutOfRange(t *testing.T) {
	self := &fakeSelf{x: 0, y: 0, radius: 5}
	c := newTestController(t, self)
	target := &fakeTarget{id: 7, x: 1000, y: 0, radius: 5}

	if !c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true for out-of-range target")
	}
	if !c.move.Following() || c.move.FollowMode() != FollowOffensive {
		t.Fatalf("follow state = (%v, %v), want offensive follow active", c.move.Following(), c.move.FollowMode())
	}
}

func TestControllerMaybeStartOffensiveFollowStopsWhenInRange(t *testing.T) {
	self := &fakeSelf{x: 0, y: 0, radius: 5}
	c := newTestController(t, self)
	c.move.StartOffensiveFollow(7, 40)

	target := &fakeTarget{id: 7, x: 10, y: 0, radius: 5}
	if c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("MaybeStartOffensiveFollow() = true, want false for in-range target")
	}
	if c.move.Following() {
		t.Fatal("follow task still active after target came into range")
	}
}

func TestControllerMaybeStartOffensiveFollowRejectsNegativeRange(t *testing.T) {
	self := &fakeSelf{x: 0, y: 0}
	c := newTestController(t, self)
	target := &fakeTarget{id: 7, x: 1000, y: 0}

	if c.MaybeStartOffensiveFollow(target, -1) {
		t.Fatal("MaybeStartOffensiveFollow() = true, want false for negative range")
	}
}

func TestControllerMaybeStartOffensiveFollowIgnoresUnlocatedTarget(t *testing.T) {
	self := &fakeSelf{x: 0, y: 0}
	c := newTestController(t, self)

	var target attackable.Combatant = &fakeTarget{id: 7, x: 1000, y: 0}
	// Wrap in a type without Position/CollisionRadius to simulate an
	// opaque combatant.
	type bare struct{ attackable.Combatant }
	if c.MaybeStartOffensiveFollow(bare{target}, 40) {
		t.Fatal("MaybeStartOffensiveFollow() = true, want false for a target with no known footprint")
	}
}

func TestControllerMaybeStartOffensiveFollowReportsFalseWhenRouteIsBlocked(t *testing.T) {
	self := &fakeSelf{x: 0, y: 0, radius: 5}
	cm, err := NewCreatureMove(location.Location{X: self.x, Y: self.y}, 100, &recordingGeo{canMove: false})
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewController(cm, self)
	if err != nil {
		t.Fatal(err)
	}
	target := &fakeTarget{id: 7, x: 1000, y: 0, radius: 5}

	if c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("MaybeStartOffensiveFollow() = true, want false when the route is blocked — a caller must not wait forever on movement that will never happen")
	}
	if c.move.Moving() {
		t.Fatal("Moving() = true after a blocked route, want false")
	}
	if len(self.broadcasts) != 0 {
		t.Fatalf("BroadcastMove calls after a blocked route = %d, want 0", len(self.broadcasts))
	}
}

func TestControllerStopCancelsFollow(t *testing.T) {
	self := &fakeSelf{}
	c := newTestController(t, self)
	c.move.StartOffensiveFollow(7, 40)

	c.Stop()

	if c.move.Following() {
		t.Fatal("follow task still active after Stop")
	}
	if self.stopCalls != 1 {
		t.Fatalf("BroadcastStop calls = %d, want 1", self.stopCalls)
	}
}

func TestControllerStopIsSilentWhenNothingWasMoving(t *testing.T) {
	self := &fakeSelf{}
	c := newTestController(t, self)

	c.Stop()

	if self.stopCalls != 0 {
		t.Fatalf("BroadcastStop calls = %d, want 0 when nothing was moving", self.stopCalls)
	}
}

func TestControllerMaybeStartOffensiveFollowIssuesMovementTowardTarget(t *testing.T) {
	self := &fakeSelf{x: 0, y: 0, radius: 5}
	c := newTestController(t, self)
	target := &fakeTarget{id: 7, x: 1000, y: 0, radius: 5}

	if !c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true for out-of-range target")
	}
	if !c.move.Moving() {
		t.Fatal("Moving() = false, want true: MaybeStartOffensiveFollow must actually start closing the distance")
	}
	if got := c.move.Destination(); got != (location.Location{X: 1000, Y: 0, Z: 0}) {
		t.Fatalf("Destination() = %+v, want the target's position", got)
	}
	if len(self.broadcasts) != 1 {
		t.Fatalf("BroadcastMove calls = %d, want 1", len(self.broadcasts))
	}
	if got := self.broadcasts[0].Destination; got != (location.Location{X: 1000, Y: 0, Z: 0}) {
		t.Fatalf("broadcast destination = %+v, want the target's position", got)
	}
}

func TestControllerMaybeStartOffensiveFollowDoesNotReissueAnAlreadyConvergingMove(t *testing.T) {
	self := &fakeSelf{x: 0, y: 0, radius: 5}
	c := newTestController(t, self)
	target := &fakeTarget{id: 7, x: 1000, y: 0, radius: 5}

	if !c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("first MaybeStartOffensiveFollow() = false, want true")
	}
	firstDestination := c.move.Destination()

	// A second re-evaluation of the same stationary target before arrival
	// must not restart the in-flight move (which would reset its timer).
	if !c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("second MaybeStartOffensiveFollow() = false, want true")
	}
	if got := c.move.Destination(); got != firstDestination {
		t.Fatalf("Destination() changed across a redundant re-evaluation: got %+v, want %+v", got, firstDestination)
	}
	if len(self.broadcasts) != 1 {
		t.Fatalf("BroadcastMove calls across two evaluations = %d, want 1 (no re-broadcast for a redundant move)", len(self.broadcasts))
	}
}

func TestControllerSetArrivedFiresOnceMovementCompletes(t *testing.T) {
	self := &fakeSelf{x: 0, y: 0, radius: 5}
	cm, err := NewCreatureMove(location.Location{X: self.x, Y: self.y}, 100, &recordingGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	cm.afterFunc = clock.AfterFunc
	c, err := NewController(cm, self)
	if err != nil {
		t.Fatal(err)
	}
	arrivedCalls := 0
	c.SetArrived(func() { arrivedCalls++ })

	target := &fakeTarget{id: 7, x: 1000, y: 0, radius: 5}
	if !c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true")
	}

	for _, timer := range clock.timers {
		if !timer.stopped {
			timer.stopped = true
			timer.f()
		}
	}

	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls = %d, want 1", arrivedCalls)
	}
}

func TestControllerStopCancelsInFlightMovement(t *testing.T) {
	self := &fakeSelf{x: 0, y: 0, radius: 5}
	c := newTestController(t, self)
	target := &fakeTarget{id: 7, x: 1000, y: 0, radius: 5}
	if !c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true")
	}

	c.Stop()

	if c.move.Moving() {
		t.Fatal("Moving() = true after Stop, want false")
	}
	if self.stopCalls != 1 {
		t.Fatalf("BroadcastStop calls = %d, want 1", self.stopCalls)
	}
}

func TestControllerMoveHomeRequestsMovement(t *testing.T) {
	self := &fakeSelf{}
	c := newTestController(t, self)

	home := location.Location{X: 500, Y: 500, Z: 0}
	c.MoveHome(home)

	if got := c.move.Destination(); got != home {
		t.Fatalf("destination = %+v, want %+v", got, home)
	}
}
