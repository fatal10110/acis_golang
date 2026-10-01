package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// knowingFollowSelf is a player-shaped follower whose known list the test
// controls and which records every heading set on it.
type knowingFollowSelf struct {
	playerFollowSelf
	forgot   bool
	stopped  int
	headings []int
}

func (s *knowingFollowSelf) Knows(attackable.Combatant) bool { return !s.forgot }
func (s *knowingFollowSelf) BroadcastStop()                  { s.stopped++ }
func (s *knowingFollowSelf) SetHeading(h int)                { s.headings = append(s.headings, h) }

func newPlayerFriendlyFollowController(t *testing.T) (*Controller, *CreatureMove, *knowingFollowSelf) {
	t.Helper()
	self := &knowingFollowSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	controller, err := NewController(mover, self, nil)
	if err != nil {
		t.Fatal(err)
	}
	return controller, mover, self
}

// A player's friendly follow walks at once toward a target farther than the
// offset, with the target-relative movement packet at that offset.
func TestControllerPlayerFriendlyFollowStartsPawnMove(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 200}

	if following, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil || !following {
		t.Fatalf("MaybeStartFriendlyFollow() = %v, %v; want the follow armed", following, err)
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d, want 1", got)
	}
	if ev := self.moves[0]; ev.FollowTarget != target.ObjectID() || ev.FollowOffset != 70 || ev.Destination != (location.Location{X: 200}) {
		t.Fatalf("move = %+v, want a pawn move toward the target at offset 70", ev)
	}
	requireFollow(t, mover, FollowFriendly, target.ObjectID(), "player friendly follow")
}

// Within the offset (footprints not counted) the follow is armed but no walk
// starts; once the target moves away, the next once-a-second tick walks.
func TestControllerPlayerFriendlyFollowInRangeWaitsForTheTick(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 69}

	if following, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil || !following {
		t.Fatalf("MaybeStartFriendlyFollow() = %v, %v; want the follow armed", following, err)
	}
	if got := len(self.moves); got != 0 {
		t.Fatalf("move broadcasts in range = %d, want 0", got)
	}
	requireFollow(t, mover, FollowFriendly, target.ObjectID(), "armed follow in range")

	target.x = 300
	for range 9 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 0 {
		t.Fatalf("move broadcasts before the 1 s tick = %d, want 0", got)
	}
	controller.PositionUpdate()
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts after the 1 s tick = %d, want 1", got)
	}
	if got := self.moves[0].Destination; got != (location.Location{X: 300}) {
		t.Fatalf("tick move destination = %+v, want the target's new position", got)
	}
}

// A tick that finds the player still walking re-issues nothing: the walk
// itself tracks the target to its new position and ends within the offset
// of it, so the next tick finds the player in range.
func TestControllerPlayerFriendlyFollowTickWaitsForTheWalk(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 200}
	if _, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil {
		t.Fatal(err)
	}
	target.x = 400

	// 330 units at speed 100 take 34 updates: the 1 s ticks find it walking.
	for range 33 {
		controller.PositionUpdate()
	}
	if !mover.Moving() {
		t.Fatalf("walk ended at %+v before reaching the offset", mover.Position())
	}
	controller.PositionUpdate()
	if mover.Moving() {
		t.Fatal("walk still under way within the offset of the target")
	}
	if got := mover.Position(); got != (location.Location{X: 340}) {
		t.Fatalf("walk ended at %+v, want X 340: the first step strictly within 70 of the moved target", got)
	}
	// The arrival handler publishes the stop cell, as the player's does.
	self.SyncPosition(mover.Position())
	for range 6 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d (%+v), want the first walk only", got, self.moves)
	}
}

// A target the player no longer knows ends the follow and stops the player.
func TestControllerPlayerFriendlyFollowForgottenTargetStops(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 200}
	if _, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil {
		t.Fatal(err)
	}

	self.forgot = true
	for range 10 {
		controller.PositionUpdate()
	}
	requireFollow(t, mover, FollowNone, 0, "follow of a forgotten target")
	if mover.Moving() {
		t.Fatal("player still walking after its follow target was forgotten")
	}
	if self.stopped != 1 {
		t.Fatalf("stop broadcasts = %d, want 1", self.stopped)
	}
}

// CancelFriendlyFollow ends the follow task and leaves the walk running;
// it leaves an offensive follow alone.
func TestControllerCancelFriendlyFollowKeepsTheWalk(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 200}
	if _, err := controller.MaybeStartFriendlyFollow(target, 70); err != nil {
		t.Fatal(err)
	}
	controller.CancelFriendlyFollow()
	requireFollow(t, mover, FollowNone, 0, "cancelled friendly follow")
	if !mover.Moving() {
		t.Fatal("CancelFriendlyFollow stopped the walk under way")
	}
	target.x = 600
	for range 40 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts after the cancel = %d, want 1", got)
	}

	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}
	controller.CancelFriendlyFollow()
	requireFollow(t, mover, FollowOffensive, target.ObjectID(), "offensive follow after a friendly cancel")
}

// A pawn move walks toward the target's current position and broadcasts the
// target-relative movement packet at the given offset, without arming a
// follow task that would re-aim or re-issue it.
func TestControllerMoveToPawnBroadcastsPawnMoveWithoutFollow(t *testing.T) {
	controller, mover, self := newPlayerFriendlyFollowController(t)
	target := &followTarget{x: 200}

	if !controller.MoveToPawn(target, 100) {
		t.Fatal("MoveToPawn() = false, want the walk accepted")
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d, want 1", got)
	}
	if ev := self.moves[0]; ev.FollowTarget != target.ObjectID() || ev.FollowOffset != 100 || ev.Destination != (location.Location{X: 200}) {
		t.Fatalf("move = %+v, want a pawn move toward the target at offset 100", ev)
	}
	if !mover.Moving() {
		t.Fatal("Moving() = false after MoveToPawn, want the walk under way")
	}
	if mover.Following() {
		t.Fatalf("follow mode = %v after MoveToPawn, want none", mover.FollowMode())
	}
}

// A pawn move replaces a running offensive follow: the follow's recheck must
// not steer the walk back toward the earlier target.
func TestControllerMoveToPawnDropsRunningFollow(t *testing.T) {
	controller, mover, _ := newPlayerFriendlyFollowController(t)
	chased := &followTarget{x: 500}
	if following, err := controller.MaybeStartOffensiveFollow(chased, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}

	pawn := &followTarget{x: -500}
	if !controller.MoveToPawn(pawn, 100) {
		t.Fatal("MoveToPawn() = false, want the walk accepted")
	}
	if mover.Following() {
		t.Fatalf("follow mode = %v after MoveToPawn, want the offensive follow dropped", mover.FollowMode())
	}
	if controller.tracksFollowLocked() {
		t.Fatal("controller still rechecks a follow after MoveToPawn")
	}
	if got := mover.Destination(); got != (location.Location{X: -500}) {
		t.Fatalf("destination = %+v, want the pawn's cell", got)
	}
}

// eventLog records the events a controller emits.
type eventLog struct{ events []event.Event }

func (l *eventLog) Emit(e event.Event) { l.events = append(l.events, e) }

func (l *eventLog) arrivals() int {
	n := 0
	for _, e := range l.events {
		if _, ok := e.(event.Arrived); ok {
			n++
		}
	}
	return n
}

// newPawnWalkController is a player-shaped controller at the origin moving
// 10 units per position update, reporting to a recorded sink.
func newPawnWalkController(t *testing.T) (*Controller, *CreatureMove, *knowingFollowSelf, *eventLog, *moveClock) {
	t.Helper()
	self := &knowingFollowSelf{}
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
	return controller, mover, self, sink, clock
}

// A pawn walk re-aims at the pawn's current position on every update
// (PlayerMove.updatePosition's _destination.set(_pawn.getPosition())) and
// ends at the first step strictly within the offset of it, 2D
// (isOnLastPawnMoveGeoPath() && isIn2DRadius(_pawn, _offset)), not at the
// cell the pawn stood on when the walk started.
func TestControllerPawnWalkReaimsAndStopsWithinOffset(t *testing.T) {
	controller, mover, _, sink, _ := newPawnWalkController(t)
	pawn := &followTarget{x: 500}
	if !controller.MoveToPawn(pawn, 100) {
		t.Fatal("MoveToPawn() = false, want the walk accepted")
	}

	for range 10 {
		controller.PositionUpdate()
	}
	if got := mover.Position(); got != (location.Location{X: 100}) {
		t.Fatalf("position after 10 updates = %+v, want X 100", got)
	}
	// The pawn walks away along the same line: the walk follows it.
	pawn.x = 800
	controller.PositionUpdate()
	if got := mover.Destination(); got != (location.Location{X: 800}) {
		t.Fatalf("destination after the pawn moved = %+v, want its new position", got)
	}
	for range 59 {
		controller.PositionUpdate()
	}
	if !mover.Moving() {
		t.Fatalf("walk ended at %+v, want it still 100 or more from the pawn", mover.Position())
	}
	controller.PositionUpdate()
	if mover.Moving() {
		t.Fatalf("walk still under way at %+v within 100 of the pawn", mover.Position())
	}
	if got := mover.Position(); got != (location.Location{X: 710}) {
		t.Fatalf("walk ended at %+v, want X 710: the first step strictly within 100 of X 800", got)
	}
	if got := sink.arrivals(); got != 1 {
		t.Fatalf("Arrived events = %d, want 1", got)
	}
}

// The re-aim is 2D and follows a pawn moving sideways too; the stop keeps
// strictly inside the offset. Each continuing step turns the walker toward
// the step it took (PlayerMove.updatePosition's setHeadingTo(nextX, nextY)
// when _pawn != null), so the heading follows the pawn rather than keeping
// the walk's starting +X heading.
func TestControllerPawnWalkFollowsSidewaysPawn(t *testing.T) {
	controller, mover, self, _, _ := newPawnWalkController(t)
	pawn := &followTarget{x: 300}
	if !controller.MoveToPawn(pawn, 40) {
		t.Fatal("MoveToPawn() = false, want the walk accepted")
	}
	start := location.Location{}.HeadingTo(location.Location{X: 300})
	controller.PositionUpdate()
	pawn.x, pawn.y = 0, 300
	self.headings = nil
	before := mover.Position()
	controller.PositionUpdate()
	after := mover.Position()
	want := before.HeadingTo(after)
	if want == start {
		t.Fatalf("test setup: the sideways step %+v -> %+v kept the start heading %d", before, after, start)
	}
	if len(self.headings) != 1 || self.headings[0] != want {
		t.Fatalf("headings set on the sideways step = %v, want [%d] (toward %+v, not the start heading %d)", self.headings, want, after, start)
	}
	for range 100 {
		if !mover.Moving() {
			break
		}
		controller.PositionUpdate()
	}
	if mover.Moving() {
		t.Fatalf("walk never reached the pawn; at %+v", mover.Position())
	}
	pos := mover.Position()
	target := location.Location{X: pawn.x, Y: pawn.y}
	if !pos.In2DRadius(target, 40) || pos.In2DRadius(target, 30) {
		t.Fatalf("walk ended at %+v, %.1f from the pawn; want within 40 but not within 30", pos, pos.Distance2D(target))
	}
}

// A pawn the player no longer knows ends the walk where the player stands,
// as an arrival (the reference returns from updatePosition and notifies
// ARRIVED), with no stop packet.
func TestControllerPawnWalkEndsOnUnknownPawn(t *testing.T) {
	controller, mover, self, sink, _ := newPawnWalkController(t)
	pawn := &followTarget{x: 500}
	if !controller.MoveToPawn(pawn, 100) {
		t.Fatal("MoveToPawn() = false, want the walk accepted")
	}
	for range 3 {
		controller.PositionUpdate()
	}
	self.forgot = true
	controller.PositionUpdate()
	if mover.Moving() {
		t.Fatal("walk still under way after its pawn was forgotten")
	}
	if got := mover.Position(); got != (location.Location{X: 30}) {
		t.Fatalf("position = %+v, want the walk left where it stood (X 30)", got)
	}
	if got := sink.arrivals(); got != 1 {
		t.Fatalf("Arrived events = %d, want 1", got)
	}
	if self.stopped != 0 {
		t.Fatalf("stop broadcasts = %d, want none", self.stopped)
	}
}

// A walk to a fixed point ignores the known list: only a pawn walk tracks
// its pawn.
func TestControllerPlainWalkIgnoresKnownList(t *testing.T) {
	controller, mover, self, _, _ := newPawnWalkController(t)
	if ok, err := controller.MoveToLocation(location.Location{X: 500}); err != nil || !ok {
		t.Fatalf("MoveToLocation() = %v, %v; want accepted", ok, err)
	}
	self.forgot = true
	self.headings = nil
	controller.PositionUpdate()
	if !mover.Moving() {
		t.Fatal("a walk to a fixed point ended on a known-list check")
	}
	// Only a pawn walk turns the walker on each update.
	if len(self.headings) != 0 {
		t.Fatalf("headings set by a plain walk's position update = %v, want none", self.headings)
	}
}

// With no position update ending it first, a pawn walk's arrival timer
// ends the walk where the position updates would, at the first step
// strictly within the offset of the pawn, and a re-aim moves the timer
// with the pawn instead of snapping the walker onto a stale cell.
func TestControllerPawnWalkArrivalTimer(t *testing.T) {
	controller, mover, _, sink, clock := newPawnWalkController(t)
	pawn := &followTarget{x: 300}
	if !controller.MoveToPawn(pawn, 100) {
		t.Fatal("MoveToPawn() = false, want the walk accepted")
	}
	clock.in.Advance(2 * time.Second)
	if !mover.Moving() {
		t.Fatalf("walk ended at %+v on the 200 units to the offset, before the step within it", mover.Position())
	}
	clock.in.Advance(PositionUpdateInterval)
	if mover.Moving() {
		t.Fatal("walk still under way after the step within the offset")
	}
	if got := mover.Position(); got != (location.Location{X: 210}) {
		t.Fatalf("timer stop = %+v, want X 210, the first step strictly within 100 of the pawn", got)
	}
	if got := sink.arrivals(); got != 1 {
		t.Fatalf("Arrived events = %d, want 1", got)
	}

	pawn.x = 600
	if !controller.MoveToPawn(pawn, 100) {
		t.Fatal("second MoveToPawn() = false, want the walk accepted")
	}
	controller.PositionUpdate()
	pawn.x = 900
	controller.PositionUpdate()
	clock.in.Advance(2 * time.Second)
	if !mover.Moving() {
		t.Fatalf("walk ended at %+v on the timer armed before the pawn moved on", mover.Position())
	}
	clock.in.Advance(4 * time.Second)
	if got := mover.Position(); mover.Moving() || got != (location.Location{X: 810}) {
		t.Fatalf("walk at %+v (moving %v), want stopped at X 810", got, mover.Moving())
	}
}
