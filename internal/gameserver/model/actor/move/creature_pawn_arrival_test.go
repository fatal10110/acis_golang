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

// stairGeo is open ground with two layers: a floor at Z 0 and, over it, a
// stair rising half a unit per unit along X. Height answers the highest
// layer at or below the probe height, as the geodata does.
type stairGeo struct{}

func (stairGeo) CanMove(_, _, _, _, _, _ int) bool { return true }

func (stairGeo) Height(x, _, z int) int16 {
	if stair := x / 2; stair <= z {
		return int16(stair)
	}
	return 0
}

func (stairGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }

func (stairGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func (stairGeo) Walkable(int, int, int) bool { return true }

func (g stairGeo) CanFly(ox, oy, oz int, _ float64, tx, ty, tz int) bool {
	return g.CanMove(ox, oy, oz, tx, ty, tz)
}

func (g stairGeo) ValidFlyLocation(ox, oy, oz int, _ float64, tx, ty, tz int) location.Location {
	return g.ValidLocation(ox, oy, oz, tx, ty, tz)
}

// The arrival timer's ground steps read each step's floor from the height
// the step before landed on, as the position updates do (PlayerMove.
// updatePosition and CreatureMove.updatePosition: getHeight(nextX, nextY,
// curZ + 2 * CELL_HEIGHT)), so a pawn walk up a stair over a floor ends on
// the stair, on the cell the updates end it on, whether the timer runs the
// whole leg or only its last steps.
func TestPawnArrivalTimerClimbsStairLikeUpdates(t *testing.T) {
	pawnAt := location.Location{X: 300, Z: 150}
	want := location.Location{X: 270, Z: 135}
	walks := []struct {
		name  string
		start func(t *testing.T) (update func(), mover *CreatureMove, sink *eventLog, clock *moveClock)
	}{
		{name: "player tracking walk", start: func(t *testing.T) (func(), *CreatureMove, *eventLog, *moveClock) {
			controller, mover, _, sink, clock := newPlayerStepController(t, 100, stairGeo{})
			if !controller.MoveToPawn(&followTarget{x: pawnAt.X, z: pawnAt.Z}, 40) {
				t.Fatal("MoveToPawn() not accepted")
			}
			return func() { clock.tick(controller) }, mover, sink, clock
		}},
		{name: "npc chase", start: func(t *testing.T) (func(), *CreatureMove, *eventLog, *moveClock) {
			controller, mover, sink, clock := newChaseController(t, &summonChaseSelf{}, stairGeo{})
			if following, err := controller.MaybeStartOffensiveFollow(&followTarget{x: pawnAt.X, z: pawnAt.Z}, 40); err != nil || !following {
				t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the chase", following, err)
			}
			return func() { controller.PositionUpdate() }, mover, sink, clock
		}},
	}
	for _, walk := range walks {
		for _, run := range []struct {
			name    string
			updates int // position updates before the ticks stall; -1 never stall
		}{
			{name: "updates only", updates: -1},
			{name: "timer after 23 updates", updates: 23},
			{name: "timer only", updates: 0},
		} {
			t.Run(walk.name+"/"+run.name, func(t *testing.T) {
				update, mover, sink, clock := walk.start(t)
				for i := 0; mover.Moving() && (run.updates < 0 || i < run.updates); i++ {
					if i == 100 {
						t.Fatalf("walk still under way at %+v", mover.Position())
					}
					update()
				}
				clock.in.Advance(time.Minute)
				if mover.Moving() {
					t.Fatalf("walk still under way at %+v after its arrival timer", mover.Position())
				}
				if got := mover.Position(); got != want {
					t.Fatalf("walk ended at %+v, want %+v on the stair", got, want)
				}
				if got := sink.arrivals(); got != 1 {
					t.Fatalf("Arrived events = %d, want 1", got)
				}
			})
		}
	}
}

// crossWallGeo is flat open ground with a wall along X = wall, once up: a
// line crossing it is closed.
type crossWallGeo struct {
	staticGeo
	wall int
	up   bool
}

func (g *crossWallGeo) CanMove(ox, _, _, tx, _, _ int) bool {
	return !g.up || (ox < g.wall) == (tx < g.wall)
}

// The arrival timer's ground steps meet a closed line where the position
// updates would (PlayerMove.updatePosition and CreatureMove.updatePosition
// check canMoveToTarget(curX, curY, curZ, nextX, nextY, nextZ) on every
// step and stop blocked there): a wall going up across a pawn walk (a door
// closing) ends it blocked on the last open step short of the wall, not
// back where the timer's run started.
func TestPawnArrivalTimerBlocksAtLastOpenStep(t *testing.T) {
	want := location.Location{X: 200}
	walks := []struct {
		name  string
		start func(t *testing.T, geo Geo) (update func(), mover *CreatureMove, clock *moveClock)
	}{
		{name: "player tracking walk", start: func(t *testing.T, geo Geo) (func(), *CreatureMove, *moveClock) {
			controller, mover, _, _, clock := newPlayerStepController(t, 100, geo)
			if !controller.MoveToPawn(&followTarget{x: 300}, 40) {
				t.Fatal("MoveToPawn() not accepted")
			}
			return func() { clock.tick(controller) }, mover, clock
		}},
		{name: "npc chase", start: func(t *testing.T, geo Geo) (func(), *CreatureMove, *moveClock) {
			controller, mover, _, clock := newChaseController(t, &summonChaseSelf{}, geo)
			if following, err := controller.MaybeStartOffensiveFollow(&followTarget{x: 300}, 40); err != nil || !following {
				t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want the chase", following, err)
			}
			return func() { controller.PositionUpdate() }, mover, clock
		}},
	}
	for _, walk := range walks {
		for _, run := range []struct {
			name    string
			updates int // position updates before the ticks stall; -1 never stall
		}{
			{name: "updates only", updates: -1},
			{name: "timer after 5 updates", updates: 5},
			{name: "timer only", updates: 0},
		} {
			t.Run(walk.name+"/"+run.name, func(t *testing.T) {
				geo := &crossWallGeo{staticGeo: staticGeo{canMove: true}, wall: 205}
				update, mover, clock := walk.start(t, geo)
				geo.up = true
				for i := 0; mover.Moving() && (run.updates < 0 || i < run.updates); i++ {
					if i == 100 {
						t.Fatalf("walk still under way at %+v", mover.Position())
					}
					update()
				}
				clock.in.Advance(time.Minute)
				if mover.Moving() {
					t.Fatalf("walk still under way at %+v after its arrival timer", mover.Position())
				}
				if got := mover.Position(); got != want {
					t.Fatalf("walk ended at %+v, want %+v, the last open step short of the wall", got, want)
				}
			})
		}
	}
}
