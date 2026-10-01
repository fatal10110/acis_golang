package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// kindPawn is a pawn of a given kind standing at a fixed point.
type kindPawn struct {
	kind    actor.Kind
	x, y, z int
}

func (*kindPawn) ObjectID() int32             { return 2 }
func (p *kindPawn) Kind() actor.Kind          { return p.kind }
func (p *kindPawn) Position() (int, int, int) { return p.x, p.y, p.z }

// everywhereWater is a water zone covering the whole world, its surface far
// above the test floor.
func everywhereWater(location.Location) (int, bool) { return 10_000, true }

// gateGeo is open geodata whose lines close once closed is set.
type gateGeo struct {
	staticGeo
	closed *bool
}

func (g gateGeo) CanMove(int, int, int, int, int, int) bool { return !*g.closed }

func newTestMover(t *testing.T, geo Geo) (*CreatureMove, *moveClock) {
	t.Helper()
	mover, err := NewCreatureMove(location.Location{}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := newMoveClock()
	mover.SetQueue(clock.q)
	return mover, clock
}

// updateUntilStopped runs position updates until the move stops, and
// reports how many it took.
func updateUntilStopped(t *testing.T, mover *CreatureMove) int {
	t.Helper()
	for n := 1; ; n++ {
		if n > 200 {
			t.Fatalf("move still under way at %+v", mover.Position())
		}
		if _, moving := mover.UpdatePosition(PositionUpdateInterval); !moving {
			return n
		}
	}
}

// A chase toward a creature (CreatureMove.updatePosition) measures by move
// type: a swimming chaser walks the 3D line, its height stepping
// curZ + (int)(dz * fraction + 0.5) with no geodata floor, and stops at the
// first step strictly within the offset in 3D; a ground chaser keeps the
// floor and stops by 2D distance. Expected cells follow the reference
// formula step by step: speed 100 covers 10 units per update.
func TestChasePawnStopMeasuresByMoveType(t *testing.T) {
	tests := []struct {
		name      string
		water     func(location.Location) (int, bool)
		wantType  MoveType
		wantFirst location.Location
		wantStop  location.Location
		wantSteps int
	}{
		{
			name:      "swimming",
			water:     everywhereWater,
			wantType:  MoveSwim,
			wantFirst: location.Location{X: 7, Z: 7},
			wantStop:  location.Location{X: 274, Z: 273},
			wantSteps: 39,
		},
		{
			name:      "ground",
			wantType:  MoveGround,
			wantFirst: location.Location{X: 10},
			wantStop:  location.Location{X: 270},
			wantSteps: 27,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mover, _ := newTestMover(t, staticGeo{canMove: true})
			if tt.water != nil {
				mover.SetWaterSurface(tt.water)
			}
			if got := mover.MoveType(); got != tt.wantType {
				t.Fatalf("MoveType() = %d, want %d", got, tt.wantType)
			}
			pawn := &kindPawn{kind: actor.KindNPC, x: 300, z: 300}
			if _, _, err := mover.ChasePawnWithPathOutcome(pawn, 40); err != nil {
				t.Fatal(err)
			}
			if ev, _ := mover.UpdatePosition(PositionUpdateInterval); ev.Origin != tt.wantFirst {
				t.Fatalf("first step = %+v, want %+v", ev.Origin, tt.wantFirst)
			}
			if steps := 1 + updateUntilStopped(t, mover); steps != tt.wantSteps {
				t.Fatalf("chase took %d updates, want %d", steps, tt.wantSteps)
			}
			if got := mover.Position(); got != tt.wantStop {
				t.Fatalf("chase stopped at %+v, want %+v", got, tt.wantStop)
			}
		})
	}
}

// A swimming player (PlayerMove.updatePosition) carries an accurate height:
// each update adds dz * fraction to it, measuring dz from the current cell,
// and lands on Math.round of it.
func TestSwimmingPlayerStepRoundsAccurateHeight(t *testing.T) {
	mover, _ := newTestMover(t, staticGeo{canMove: true})
	mover.SetWaterSurface(everywhereWater)
	mover.SetSpeeds(100, 100)
	if _, err := mover.MoveToLocation(location.Location{X: 300, Z: 100}); err != nil {
		t.Fatal(err)
	}
	for i, want := range []location.Location{
		{X: 9, Z: 3}, {X: 19, Z: 6}, {X: 28, Z: 9}, {X: 38, Z: 13}, {X: 47, Z: 16},
	} {
		if ev, _ := mover.UpdatePosition(PositionUpdateInterval); ev.Origin != want {
			t.Fatalf("update %d landed at %+v, want %+v", i+1, ev.Origin, want)
		}
	}
}

// A flying mover keeps its own height toward the destination's, and no
// closed geodata line stops it; a ground mover meeting the same line stops
// blocked.
func TestFlyingMoverSkipsGroundGeodata(t *testing.T) {
	for _, flying := range []bool{true, false} {
		closed := false
		mover, _ := newTestMover(t, gateGeo{staticGeo: staticGeo{canMove: true, height: -50}, closed: &closed})
		mover.SetFlying(flying)
		if _, err := mover.MoveToLocation(location.Location{X: 0, Y: 300, Z: 300}); err != nil {
			t.Fatal(err)
		}
		closed = true
		ev, moving := mover.UpdatePosition(PositionUpdateInterval)
		if !flying {
			if moving || mover.Position() != (location.Location{}) {
				t.Fatalf("ground mover through a closed line: moving %v at %+v, want stopped in place", moving, mover.Position())
			}
			continue
		}
		if want := (location.Location{Y: 7, Z: 7}); !moving || ev.Origin != want {
			t.Fatalf("flying step = %+v (moving %v), want %+v", ev.Origin, moving, want)
		}
	}
}

// A same-cell request of a swimmer to another height is a real move, not a
// same-cell arrival.
func TestSwimmingMoverClimbsInPlace(t *testing.T) {
	mover, _ := newTestMover(t, staticGeo{canMove: true})
	mover.SetWaterSurface(everywhereWater)
	ev, err := mover.MoveToLocation(location.Location{Z: 25})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Duration != 3*PositionUpdateInterval {
		t.Fatalf("climb duration = %v, want 3 updates", ev.Duration)
	}
	if ev, _ := mover.UpdatePosition(PositionUpdateInterval); ev.Origin != (location.Location{Z: 10}) {
		t.Fatalf("first climb step = %+v, want Z 10", ev.Origin)
	}
}

// A pawn walk stops short of its pawn only when the pawn is a creature
// (isOnLastPawnMoveGeoPath: _pawn instanceof Creature). Toward a static
// object it runs onto the object's point, on the position updates and on
// the arrival-timer fallback alike.
func TestPawnWalkStopsShortOnlyOfCreatures(t *testing.T) {
	tests := []struct {
		kind actor.Kind
		want location.Location
	}{
		{actor.KindStatic, location.Location{X: 500}},
		{actor.KindNPC, location.Location{X: 410}},
		{actor.KindDoor, location.Location{X: 410}},
	}
	for _, tt := range tests {
		self := &playerFollowSelf{}
		mover, _ := newTestMover(t, staticGeo{canMove: true})
		controller, err := NewController(mover, self, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !controller.MoveToPawn(&kindPawn{kind: tt.kind, x: 500}, 100) {
			t.Fatal("MoveToPawn() not accepted")
		}
		for i := 0; mover.Moving(); i++ {
			if i == 100 {
				t.Fatalf("kind %d: walk still under way at %+v", tt.kind, mover.Position())
			}
			controller.PositionUpdate()
		}
		if got := mover.Position(); got != tt.want {
			t.Fatalf("kind %d: walk ended at %+v, want %+v", tt.kind, got, tt.want)
		}
	}
}

func TestPawnWalkArrivalTimerStopsShortOnlyOfCreatures(t *testing.T) {
	tests := []struct {
		kind actor.Kind
		want location.Location
	}{
		{actor.KindStatic, location.Location{X: 500}},
		{actor.KindPlayer, location.Location{X: 400}},
	}
	for _, tt := range tests {
		mover, clock := newTestMover(t, staticGeo{canMove: true})
		if _, _, err := mover.MoveToPawnWithPathOutcome(&kindPawn{kind: tt.kind, x: 500}, 100); err != nil {
			t.Fatal(err)
		}
		clock.in.Advance(time.Minute)
		if mover.Moving() {
			t.Fatalf("kind %d: walk still under way after its arrival timer", tt.kind)
		}
		if got := mover.Position(); got != tt.want {
			t.Fatalf("kind %d: arrival timer stopped the walk at %+v, want %+v", tt.kind, got, tt.want)
		}
	}
}

// The follow tasks' "already in reach" check (offensiveFollowTask and
// friendlyFollowTask) measures in 3D for a swimmer: a target straight above
// within the 2D range but beyond it in 3D is followed. Starting an offensive
// follow (maybeStartOffensiveFollow) still measures in 2D.
func TestFollowTaskRangeMeasuresByMoveType(t *testing.T) {
	above := func() *followTarget { return &followTarget{z: 100} }
	t.Run("FollowTick", func(t *testing.T) {
		for _, swim := range []bool{true, false} {
			mover, _ := newTestMover(t, staticGeo{canMove: true})
			if swim {
				mover.SetWaterSurface(everywhereWater)
			}
			mover.StartFriendlyFollow(2, 40)
			_, moved, err := mover.FollowTick(TargetSnapshot{ObjectID: 2, Position: location.Location{Z: 100}, Known: true}, 0)
			if err != nil {
				t.Fatal(err)
			}
			if moved != swim {
				t.Fatalf("swim %v: FollowTick moved = %v, want %v", swim, moved, swim)
			}
		}
	})
	t.Run("summon offensive recheck", func(t *testing.T) {
		for _, swim := range []bool{true, false} {
			self := &summonChaseSelf{}
			controller, mover, _, _ := newChaseController(t, self, staticGeo{canMove: true})
			if swim {
				mover.SetWaterSurface(everywhereWater)
			}
			target := above()
			if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || following {
				t.Fatalf("swim %v: MaybeStartOffensiveFollow() = %v, %v; want in reach (2D)", swim, following, err)
			}
			following, err := controller.RecheckOffensiveFollow(target, 40)
			if err != nil {
				t.Fatal(err)
			}
			if following != swim {
				t.Fatalf("swim %v: RecheckOffensiveFollow() = %v, want %v", swim, following, swim)
			}
		}
	})
	t.Run("friendly follow", func(t *testing.T) {
		for _, swim := range []bool{true, false} {
			self := &summonChaseSelf{}
			controller, mover, _, _ := newChaseController(t, self, staticGeo{canMove: true})
			if swim {
				mover.SetWaterSurface(everywhereWater)
			}
			if _, err := controller.MaybeStartFriendlyFollow(above(), 40); err != nil {
				t.Fatal(err)
			}
			if mover.Moving() != swim {
				t.Fatalf("swim %v: friendly follow moving = %v, want %v", swim, mover.Moving(), swim)
			}
		}
	})
	t.Run("player friendly follow", func(t *testing.T) {
		for _, swim := range []bool{true, false} {
			controller, mover, _ := newPlayerFriendlyFollowController(t)
			if swim {
				mover.SetWaterSurface(everywhereWater)
			}
			if _, err := controller.MaybeStartFriendlyFollow(above(), 40); err != nil {
				t.Fatal(err)
			}
			if mover.Moving() != swim {
				t.Fatalf("swim %v: player friendly follow moving = %v, want %v", swim, mover.Moving(), swim)
			}
		}
	})
}
