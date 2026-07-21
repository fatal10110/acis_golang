package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type fakeSelf struct {
	id         int32
	x, y, z    int
	radius     float64
	broadcasts []Event
	stopCalls  int
}

func (f *fakeSelf) ObjectID() int32           { return f.id }
func (f *fakeSelf) Position() (int, int, int) { return f.x, f.y, f.z }
func (f *fakeSelf) CollisionRadius() float64  { return f.radius }
func (f *fakeSelf) BroadcastMove(event Event) { f.broadcasts = append(f.broadcasts, event) }
func (f *fakeSelf) BroadcastStop()            { f.stopCalls++ }
func (f *fakeSelf) SyncPosition(position location.Location) {
	f.x, f.y, f.z = position.X, position.Y, position.Z
}

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

func TestControllerRegistersPositionUpdatesOnMoveStart(t *testing.T) {
	self := &fakeSelf{id: 11, x: 0, y: 0, radius: 5}
	c := newTestController(t, self)
	updates := &fakePositionUpdates{}
	c.SetPositionUpdates(updates)

	target := &fakeTarget{id: 7, x: 1000, y: 0, radius: 5}
	if !c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true")
	}

	if len(updates.added) != 1 || updates.added[0].ObjectID() != self.id {
		t.Fatalf("registered movers = %v, want self", updates.added)
	}
}

func TestControllerRemovesPositionUpdatesOnStop(t *testing.T) {
	self := &fakeSelf{id: 11, x: 0, y: 0, radius: 5}
	c := newTestController(t, self)
	updates := &fakePositionUpdates{}
	c.SetPositionUpdates(updates)

	target := &fakeTarget{id: 7, x: 1000, y: 0, radius: 5}
	if !c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true")
	}
	c.Stop()

	if len(updates.removed) != 1 || updates.removed[0].ObjectID() != self.id {
		t.Fatalf("removed movers = %v, want self", updates.removed)
	}
}

func TestControllerPositionUpdateAdvancesPositionWithoutRebroadcasting(t *testing.T) {
	self := &fakeSelf{id: 11, x: 0, y: 0, radius: 5}
	c := newTestController(t, self)

	target := &fakeTarget{id: 7, x: 1000, y: 0, radius: 5}
	if !c.MaybeStartOffensiveFollow(target, 40) {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true")
	}

	if !c.PositionUpdate() {
		t.Fatal("PositionUpdate() = false, want moving")
	}
	if len(self.broadcasts) != 1 {
		t.Fatalf("BroadcastMove calls = %d, want 1 (only the move-start packet — clients interpolate)", len(self.broadcasts))
	}
	if self.x != 10 || self.y != 0 || self.z != 0 {
		t.Fatalf("synced position = (%d,%d,%d), want (10,0,0)", self.x, self.y, self.z)
	}
}

func TestControllerPositionUpdateRemovesOnArrival(t *testing.T) {
	self := &fakeSelf{id: 11, x: 0, y: 0}
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
	updates := &fakePositionUpdates{}
	c.SetPositionUpdates(updates)

	target := &fakeTarget{id: 7, x: 10, y: 0}
	if !c.MaybeStartOffensiveFollow(target, 0) {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true")
	}

	if c.PositionUpdate() {
		t.Fatal("PositionUpdate() = true, want false after arrival")
	}
	if len(updates.removed) != 1 || updates.removed[0].ObjectID() != self.id {
		t.Fatalf("removed movers = %v, want self", updates.removed)
	}
}

func TestControllerPositionUpdateKeepsRegistrationWhenArrivedHookRestartsMovement(t *testing.T) {
	self := &fakeSelf{id: 11, x: 0, y: 0}
	c := newTestController(t, self)
	updates := &fakePositionUpdates{}
	c.SetPositionUpdates(updates)

	firstTarget := &fakeTarget{id: 7, x: 10, y: 0}
	if !c.MaybeStartOffensiveFollow(firstTarget, 0) {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true")
	}

	// An arrived hook that behaves like an NPC's AI: on arrival, it
	// immediately re-evaluates and starts chasing a new, out-of-range
	// target — re-registering this controller before PositionUpdate
	// returns.
	secondTarget := &fakeTarget{id: 8, x: 1000, y: 0, radius: 5}
	c.SetArrived(func() {
		if !c.MaybeStartOffensiveFollow(secondTarget, 40) {
			t.Fatal("arrived hook: MaybeStartOffensiveFollow() = false, want true")
		}
	})

	if !c.PositionUpdate() {
		t.Fatal("PositionUpdate() = false, want true: the arrived hook started a new move")
	}
	if !c.move.Moving() || c.move.Destination() != (location.Location{X: 1000, Y: 0, Z: 0}) {
		t.Fatalf("destination = %+v, moving = %v, want the new move still in flight toward the second target", c.move.Destination(), c.move.Moving())
	}
	if len(updates.removed) != 1 {
		t.Fatalf("removed movers = %v, want exactly the stale first-move registration, not the fresh re-registration", updates.removed)
	}
	if len(updates.added) != 2 {
		t.Fatalf("added movers = %v, want the initial registration plus the arrived hook's re-registration", updates.added)
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
	updates := &fakePositionUpdates{}
	c.SetPositionUpdates(updates)

	home := location.Location{X: 500, Y: 500, Z: 0}
	c.MoveHome(home)

	if got := c.move.Destination(); got != home {
		t.Fatalf("destination = %+v, want %+v", got, home)
	}
	if len(self.broadcasts) != 1 {
		t.Fatalf("BroadcastMove calls = %d, want 1: a return-home walk must be visible to clients", len(self.broadcasts))
	}
	if len(updates.added) != 1 || updates.added[0].ObjectID() != self.id {
		t.Fatalf("registered movers = %v, want self: a return-home walk must keep world presence current", updates.added)
	}
}

func TestControllerMoveHomeDropsBlockedRoute(t *testing.T) {
	self := &fakeSelf{}
	cm, err := NewCreatureMove(location.Location{X: self.x, Y: self.y}, 100, &recordingGeo{canMove: false})
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewController(cm, self)
	if err != nil {
		t.Fatal(err)
	}
	updates := &fakePositionUpdates{}
	c.SetPositionUpdates(updates)

	c.MoveHome(location.Location{X: 500, Y: 500, Z: 0})

	if len(self.broadcasts) != 0 {
		t.Fatalf("BroadcastMove calls = %d, want 0 for a blocked route", len(self.broadcasts))
	}
	if len(updates.added) != 0 {
		t.Fatalf("registered movers = %v, want none for a blocked route", updates.added)
	}
}

// TestControllerBroadcastsMoveOnEachSegmentAdvance covers a route split across
// geopath waypoints: the client must be told about every leg, not just the
// first, or it keeps predicting the original (obstacle-crossing) straight
// line instead of the server-routed path.
func TestControllerBroadcastsMoveOnEachSegmentAdvance(t *testing.T) {
	self := &fakeSelf{x: 0, y: 0, z: 30}
	waypoints := []location.Location{
		{X: 50, Y: 0, Z: 30},
		{X: 50, Y: 50, Z: 30},
		{X: 100, Y: 50, Z: 30},
	}
	geo := &recordingGeo{
		canMove:    false, // direct line blocked -> tier 2 pathfinding
		height:     30,
		findPath:   waypoints,
		findPathOK: true,
	}
	cm, err := NewCreatureMove(location.Location{X: self.x, Y: self.y, Z: self.z}, 50, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	cm.afterFunc = clock.AfterFunc
	c, err := NewController(cm, self)
	if err != nil {
		t.Fatal(err)
	}
	updates := &fakePositionUpdates{}
	c.SetPositionUpdates(updates)

	c.MoveHome(location.Location{X: 100, Y: 50, Z: 30})
	if len(self.broadcasts) != 1 {
		t.Fatalf("BroadcastMove calls after move start = %d, want 1 (first segment)", len(self.broadcasts))
	}

	// Fire each segment's arrival timer in turn: the first two advance to
	// the next waypoint (one BroadcastMove each), the third exhausts the
	// queue and stops without an extra broadcast.
	segmentDuration := clock.timers[len(clock.timers)-1].delay
	clock.fire(segmentDuration)
	if len(self.broadcasts) != 2 {
		t.Fatalf("BroadcastMove calls after segment 1 advance = %d, want 2", len(self.broadcasts))
	}
	if got := c.move.Destination(); got != waypoints[1] {
		t.Fatalf("Destination() = %+v, want %+v", got, waypoints[1])
	}

	clock.fire(segmentDuration)
	if len(self.broadcasts) != 3 {
		t.Fatalf("BroadcastMove calls after segment 2 advance = %d, want 3", len(self.broadcasts))
	}
	if got := c.move.Destination(); got != waypoints[2] {
		t.Fatalf("Destination() = %+v, want %+v", got, waypoints[2])
	}

	clock.fire(segmentDuration)
	if len(self.broadcasts) != 3 {
		t.Fatalf("BroadcastMove calls after final segment = %d, want 3 (no re-broadcast on arrival)", len(self.broadcasts))
	}
	if c.move.Moving() {
		t.Fatal("Moving() = true after final segment, want false")
	}
}

type fakePositionUpdates struct {
	added   []PositionUpdater
	removed []PositionUpdater
}

func (f *fakePositionUpdates) Add(actor PositionUpdater) {
	f.added = append(f.added, actor)
}

func (f *fakePositionUpdates) Remove(actor PositionUpdater) {
	f.removed = append(f.removed, actor)
}
