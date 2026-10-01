package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// chaseSelf is an NPC-shaped chaser (no target-relative movement packet)
// whose known list the test controls.
type chaseSelf struct{ knowingFollowSelf }

func (*chaseSelf) OffensiveFollowIsPawnMove() bool { return false }

// summonChaseSelf is a summon-shaped chaser: its own AI rechecks the follow.
type summonChaseSelf struct{ chaseSelf }

func (*summonChaseSelf) OwnsOffensiveFollowTicker() bool { return true }

func newChaseController(t *testing.T, self Actor, geo Geo) (*Controller, *CreatureMove, *eventLog, *moveClock) {
	t.Helper()
	mover, err := NewCreatureMove(location.Location{}, 100, geo)
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
	return controller, mover, sink, clock
}

// An NPC or summon chase is a pawn walk without the player's re-aim
// (CreatureMove.offensiveFollowTask and SummonMove.offensiveFollowTask set
// _pawn/_offset, then moveToLocation(destination, true)): it is broadcast as
// a plain walk to where the target stood, keeps that destination, and ends
// at the first step strictly within the offset of where the target stands
// now (CreatureMove.updatePosition, isOnLastPawnMoveGeoPath() &&
// isIn2DRadius(_pawn, _offset)).
func TestControllerChaseStopsWithinOffsetOfPawnWithoutReaim(t *testing.T) {
	tests := []struct {
		name  string
		moved location.Location
		want  location.Location
	}{
		// 270 is 30 from the target; 260 would be exactly 40, not inside.
		{name: "standing target", moved: location.Location{X: 300}, want: location.Location{X: 270}},
		// 130 is the first step within 40 of (150, 30): |x-150| < 26.5.
		{name: "target stepped beside the path", moved: location.Location{X: 150, Y: 30}, want: location.Location{X: 130}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			self := &summonChaseSelf{}
			controller, mover, sink, _ := newChaseController(t, self, staticGeo{canMove: true})
			target := &followTarget{x: 300}
			if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
				t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the chase", following, err)
			}
			if got := len(self.moves); got != 1 {
				t.Fatalf("move broadcasts = %d, want 1", got)
			}
			if ev := self.moves[0]; ev.FollowTarget != 0 || ev.Destination != (location.Location{X: 300}) {
				t.Fatalf("chase move = %+v, want a plain walk to the target's cell", ev)
			}
			target.x, target.y = tt.moved.X, tt.moved.Y

			for i := 0; mover.Moving(); i++ {
				if i == 40 {
					t.Fatalf("chase still under way at %+v", mover.Position())
				}
				controller.PositionUpdate()
			}
			if got := mover.Position(); got != tt.want {
				t.Fatalf("chase ended at %+v, want %+v", got, tt.want)
			}
			if got := mover.Destination(); got != (location.Location{X: 300}) {
				t.Fatalf("destination = %+v, want the cell the target stood on at the request", got)
			}
			if got := sink.arrivals(); got != 1 {
				t.Fatalf("Arrived events = %d, want 1", got)
			}
			if got := len(self.headings); got != 0 {
				t.Fatalf("headings set = %d, want none: only a player's pawn walk turns per step", got)
			}
		})
	}
}

// A chase's arrival timer covers its whole leg, not the leg less the
// offset: the reference has no arrival timer, so with no position update
// ending it the walk keeps going until the leg's full travel time.
func TestControllerChaseArrivalTimerCoversTheWholeLeg(t *testing.T) {
	controller, mover, _, clock := newChaseController(t, &summonChaseSelf{}, staticGeo{canMove: true})
	if following, err := controller.MaybeStartOffensiveFollow(&followTarget{x: 300}, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the chase", following, err)
	}
	clock.in.Advance(2900 * time.Millisecond)
	if !mover.Moving() {
		t.Fatalf("chase ended at %+v before its 3 s leg ran out", mover.Position())
	}
	clock.in.Advance(100 * time.Millisecond)
	if mover.Moving() {
		t.Fatal("chase still under way after its leg ran out")
	}
}

// routedChaseGeo closes the straight line from the origin to (300, 0), so a
// chase there routes through (250, 100).
func routedChaseGeo() *recordingGeo {
	return &recordingGeo{
		canMoveAt: func(ox, oy, _, tx, ty, _ int) bool {
			return ox != 0 || oy != 0 || tx != 300 || ty != 0
		},
		findPath:   []location.Location{{X: 250, Y: 100}, {X: 300}},
		findPathOK: true,
	}
}

// A chase path-finds like any NPC walk (moveToLocation(destination, true)),
// and only its last geo leg stops at the pawn: the first leg passes within
// the offset of it and still runs to its waypoint.
func TestControllerChaseRoutesAndStopsOnlyOnTheLastLeg(t *testing.T) {
	self := &summonChaseSelf{}
	controller, mover, sink, _ := newChaseController(t, self, routedChaseGeo())
	if following, err := controller.MaybeStartOffensiveFollow(&followTarget{x: 300}, 150); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the chase", following, err)
	}
	if got := self.moves[0].Destination; got != (location.Location{X: 250, Y: 100}) {
		t.Fatalf("chase first leg = %+v, want the routed waypoint", got)
	}
	for i := 0; len(self.moves) < 2; i++ {
		if i == 40 || !mover.Moving() {
			t.Fatalf("chase stopped at %+v before its last leg", mover.Position())
		}
		controller.PositionUpdate()
	}
	if got := self.moves[1]; got.Origin != (location.Location{X: 250, Y: 100}) || got.Destination != (location.Location{X: 300}) {
		t.Fatalf("segment move = %+v, want the last leg from the waypoint", got)
	}
	controller.PositionUpdate()
	if mover.Moving() {
		t.Fatalf("chase still under way at %+v within 150 of the target", mover.Position())
	}
	if got := mover.Position(); got != (location.Location{X: 254, Y: 91}) {
		t.Fatalf("chase ended at %+v, want the last leg's first step (254, 91)", got)
	}
	if got := sink.arrivals(); got != 1 {
		t.Fatalf("Arrived events = %d, want 1", got)
	}
}

// Once the chaser no longer knows its target, each position update moves it
// no further: the walk heads for its next geo leg from where it stands
// (updatePosition returns true, then moveToNextRoutePoint) and, with none
// left, ends there as an arrival.
func TestControllerChaseOnUnknownTargetDrainsItsRouteInPlace(t *testing.T) {
	self := &summonChaseSelf{}
	controller, mover, sink, _ := newChaseController(t, self, routedChaseGeo())
	if following, err := controller.MaybeStartOffensiveFollow(&followTarget{x: 300}, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the chase", following, err)
	}
	for range 3 {
		controller.PositionUpdate()
	}
	at := mover.Position()
	self.forgot = true

	controller.PositionUpdate()
	if !mover.Moving() || mover.Position() != at {
		t.Fatalf("after losing the target: moving %v at %+v, want still under way at %+v", mover.Moving(), mover.Position(), at)
	}
	if got := len(self.moves); got != 2 || self.moves[1].Origin != at || self.moves[1].Destination != (location.Location{X: 300}) {
		t.Fatalf("moves = %+v, want the last leg broadcast from %+v", self.moves, at)
	}
	controller.PositionUpdate()
	if mover.Moving() || mover.Position() != at {
		t.Fatalf("after the route drained: moving %v at %+v, want stopped at %+v", mover.Moving(), mover.Position(), at)
	}
	if got := sink.arrivals(); got != 1 {
		t.Fatalf("Arrived events = %d, want 1", got)
	}
}

// An NPC's follow task skips a target it no longer knows and goes on once
// it is known again (CreatureMove.offensiveFollowTask returns without
// cancelling the task); its chase walk meanwhile ends where it stands.
func TestControllerNPCChaseFollowSurvivesUnknownTarget(t *testing.T) {
	self := &chaseSelf{}
	controller, mover, sink, _ := newChaseController(t, self, staticGeo{canMove: true})
	target := &followTarget{x: 300}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the chase", following, err)
	}
	controller.PositionUpdate()
	self.forgot = true
	for range 10 {
		controller.PositionUpdate()
	}
	if mover.Moving() {
		t.Fatal("chase still under way after losing its target")
	}
	if got := mover.Position(); got != (location.Location{X: 10}) {
		t.Fatalf("chase ended at %+v, want where it stood (X 10)", got)
	}
	if got := sink.arrivals(); got != 1 {
		t.Fatalf("Arrived events = %d, want 1", got)
	}
	requireFollow(t, mover, FollowOffensive, target.ObjectID(), "follow of an unknown target")
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts = %d, want no rechase while the target is unknown", got)
	}

	self.forgot = false
	for range 5 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 2 {
		t.Fatalf("move broadcasts = %d, want the follow to chase again once the target is known", got)
	}
}
