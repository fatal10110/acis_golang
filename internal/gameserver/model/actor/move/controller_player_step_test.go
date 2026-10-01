package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// wallGeo is open ground with a wall along X = 100: a straight line crossing
// it is closed, and the pathfinder offers a detour around it.
func wallGeo() *recordingGeo {
	return &recordingGeo{
		canMoveAt: func(ox, _, _, tx, _, _ int) bool {
			return (ox < 100) == (tx < 100)
		},
		findPath:   []location.Location{{X: 0, Y: 300}, {X: 300, Y: 300}, {X: 300, Y: 0}},
		findPathOK: true,
	}
}

// newPlayerStepController is a player controller at the origin whose mover
// steps by the player rules at speed (start speed included), on a virtual
// clock, reporting to a recorded sink.
func newPlayerStepController(t *testing.T, speed float64, geo Geo) (*Controller, *CreatureMove, *knowingFollowSelf, *eventLog, *moveClock) {
	t.Helper()
	self := &knowingFollowSelf{}
	mover, err := NewCreatureMove(location.Location{}, speed, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := newMoveClock()
	mover.SetQueue(clock.q)
	mover.SetSpeeds(speed, speed)
	sink := &eventLog{}
	controller, err := NewController(mover, self, sink)
	if err != nil {
		t.Fatal(err)
	}
	return controller, mover, self, sink, clock
}

// A player's step measures its delta from the current cell and rounds the
// cell it lands on, half up (PlayerMove.updatePosition, PlayerMove.java:
// 240-241 and 265-273: dx = destination - curX, _xAccurate += dx * fraction,
// nextX = Math.round(_xAccurate)). 11.5 units per update toward (-1000, 377)
// land on (-11, 4), (-22, 8), (-32, 12), (-43, 16); the NPC formula would
// truncate to (-10, 4) and (-21, 8) first.
func TestPlayerStepRoundsFromCurrentCell(t *testing.T) {
	_, mover, _, _, _ := newPlayerStepController(t, 115, staticGeo{canMove: true})
	if _, err := mover.MoveToLocation(location.Location{X: -1000, Y: 377}); err != nil {
		t.Fatal(err)
	}
	for i, want := range []location.Location{{X: -11, Y: 4}, {X: -22, Y: 8}, {X: -32, Y: 12}, {X: -43, Y: 16}} {
		mover.UpdatePosition(PositionUpdateInterval)
		if got := mover.Position(); got != want {
			t.Fatalf("position after update %d = %+v, want %+v", i+1, got, want)
		}
	}
}

// Retargeting a player's walk in flight first advances it by the time passed
// since its last update, toward the old destination, and counts that as a
// position update (PlayerMove.moveToLocation, PlayerMove.java:113-116:
// updatePosition(true) then _instant = now; :218-228 the elapsed
// milliseconds and _moveTimeStamp++). The next update covers only the rest
// of its interval (Duration.between(_instant, now)).
func TestPlayerRetargetAdvancesByElapsedTime(t *testing.T) {
	controller, mover, self, _, clock := newPlayerStepController(t, 100, staticGeo{canMove: true})
	mover.SetSpeeds(200, 100)
	if ok, err := controller.MoveToLocation(location.Location{X: 1000}); err != nil || !ok {
		t.Fatalf("MoveToLocation() = %v, %v; want accepted", ok, err)
	}
	controller.PositionUpdate()
	if got := mover.Position(); got != (location.Location{X: 10}) {
		t.Fatalf("position after the first update = %+v, want X 10", got)
	}

	clock.in.Advance(40 * time.Millisecond)
	if ok, err := controller.MoveToLocation(location.Location{X: 10, Y: 1000}); err != nil || !ok {
		t.Fatalf("retarget MoveToLocation() = %v, %v; want accepted", ok, err)
	}
	// 40 ms at the start speed toward the old destination: 4 units.
	moved := location.Location{X: 14}
	if got := self.moves[len(self.moves)-1].Origin; got != moved {
		t.Fatalf("retarget walk origin = %+v, want %+v", got, moved)
	}
	if x, y, z := self.Position(); (location.Location{X: x, Y: y, Z: z}) != moved {
		t.Fatalf("world position after the retarget = (%d,%d,%d), want synced to %+v", x, y, z, moved)
	}

	// The rest of the interval, 60 ms: 6 units. The retarget update was the
	// second one, so updates 3 to 5 run at the start speed and the sixth at
	// the full speed.
	for i, want := range []location.Location{{X: 14, Y: 6}, {X: 14, Y: 16}, {X: 14, Y: 26}, {X: 14, Y: 46}} {
		controller.PositionUpdate()
		if got := mover.Position(); got != want {
			t.Fatalf("position after update %d = %+v, want %+v", i+3, got, want)
		}
	}
}

// arrivalHeadings records the walker's last heading as each Arrived lands.
type arrivalHeadings struct {
	self *knowingFollowSelf
	got  []int
}

func (a *arrivalHeadings) Emit(e event.Event) {
	if _, ok := e.(event.Arrived); ok && len(a.self.headings) > 0 {
		a.got = append(a.got, a.self.headings[len(a.self.headings)-1])
	}
}

// The step that ends a pawn walk within the offset turns the walker toward
// it too, before the arrival runs (PlayerMove.updatePosition, PlayerMove.
// java:291-293 setHeadingTo(nextX, nextY) ahead of setXYZ and the
// in-radius return at :323).
func TestPlayerPawnWalkStoppingStepTurns(t *testing.T) {
	controller, mover, self, _, _ := newPlayerStepController(t, 100, staticGeo{canMove: true})
	arrivals := &arrivalHeadings{self: self}
	controller.sink = arrivals
	pawn := &followTarget{x: 300}
	if !controller.MoveToPawn(pawn, 100) {
		t.Fatal("MoveToPawn() = false, want the walk accepted")
	}
	for range 20 {
		controller.PositionUpdate()
	}
	before := mover.Position()
	if before != (location.Location{X: 200}) || !mover.Moving() {
		t.Fatalf("walk at %+v (moving %v), want under way at X 200", before, mover.Moving())
	}
	// The pawn steps aside: the last step re-aims at it and ends within 100.
	pawn.x, pawn.y = 210, 95
	self.headings = nil
	controller.PositionUpdate()
	if mover.Moving() {
		t.Fatalf("walk still under way at %+v", mover.Position())
	}
	after := mover.Position()
	want := before.HeadingTo(after)
	if want == 0 {
		t.Fatalf("test setup: the last step %+v -> %+v keeps the walk's heading", before, after)
	}
	if len(self.headings) != 1 || self.headings[0] != want {
		t.Fatalf("headings set on the stopping step = %v, want [%d] toward %+v", self.headings, want, after)
	}
	if len(arrivals.got) != 1 || arrivals.got[0] != want {
		t.Fatalf("heading at the arrival = %v, want [%d]: turned before the walk's milestone", arrivals.got, want)
	}
}

// A player's pawn walk never path-finds: behind a wall it heads straight for
// the pawn (PlayerMove.moveToPawn, PlayerMove.java:76 clears the geo path
// and :101 aims at the pawn) and the first step into the wall ends it blocked
// (:285-289). The attack approach ends there: no follow recheck walks the
// player into the wall again.
func TestPlayerPawnWalkStopsAtWall(t *testing.T) {
	geo := wallGeo()
	controller, mover, self, sink, _ := newPlayerStepController(t, 100, geo)
	target := &followTarget{x: 300}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the approach", following, err)
	}
	if len(geo.findPathCalls) != 0 {
		t.Fatalf("pathfinder asked %v, want the pawn walk to go straight", geo.findPathCalls)
	}
	if ev := self.moves[0]; ev.Destination != (location.Location{X: 300}) || ev.FollowTarget != target.ObjectID() {
		t.Fatalf("approach = %+v, want a pawn walk straight at the target", ev)
	}

	for range 30 {
		controller.PositionUpdate()
	}
	if mover.Moving() {
		t.Fatalf("walk still under way at %+v", mover.Position())
	}
	if got := mover.Position(); got != (location.Location{X: 90}) {
		t.Fatalf("walk stopped at %+v, want X 90, the last cell short of the wall", got)
	}
	blocked := 0
	for _, e := range sink.events {
		if _, ok := e.(event.MoveBlocked); ok {
			blocked++
		}
	}
	if blocked != 1 {
		t.Fatalf("MoveBlocked events = %d, want 1 (events %v)", blocked, sink.events)
	}
	if mover.Following() {
		t.Fatal("offensive follow still armed after the blocked approach")
	}
	sent := len(self.moves)
	for range 20 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != sent || mover.Position() != (location.Location{X: 90}) {
		t.Fatalf("after the block: %d more moves, at %+v; want the player left at X 90", got-sent, mover.Position())
	}
}

// A non-pawn walk still routes around the wall.
func TestPlayerPlainWalkStillRoutes(t *testing.T) {
	geo := wallGeo()
	controller, _, self, _, _ := newPlayerStepController(t, 100, geo)
	if ok, err := controller.MoveToLocation(location.Location{X: 300}); err != nil || !ok {
		t.Fatalf("MoveToLocation() = %v, %v; want accepted", ok, err)
	}
	if len(geo.findPathCalls) != 1 {
		t.Fatalf("pathfinder calls = %d, want 1", len(geo.findPathCalls))
	}
	if got := self.moves[0].Destination; got != (location.Location{X: 0, Y: 300}) {
		t.Fatalf("first leg = %+v, want the detour's first corner", got)
	}
}
