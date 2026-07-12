package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type fakeSelf struct {
	x, y, z int
	radius  float64
}

func (f *fakeSelf) Position() (int, int, int) { return f.x, f.y, f.z }
func (f *fakeSelf) CollisionRadius() float64  { return f.radius }

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

func TestControllerStopCancelsFollow(t *testing.T) {
	self := &fakeSelf{}
	c := newTestController(t, self)
	c.move.StartOffensiveFollow(7, 40)

	c.Stop()

	if c.move.Following() {
		t.Fatal("follow task still active after Stop")
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
