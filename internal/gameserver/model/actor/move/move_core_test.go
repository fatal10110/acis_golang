package move

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ---- from controller_3d_follow_test.go ----
type playerFollowSelf struct {
	x, y, z int
	moves   []Event
}

type tickerOwnedFollowSelf struct{ playerFollowSelf }

func (*tickerOwnedFollowSelf) OwnsOffensiveFollowTicker() bool { return true }

func (s *playerFollowSelf) ObjectID() int32                    { return 1 }
func (s *playerFollowSelf) Position() (int, int, int)          { return s.x, s.y, s.z }
func (s *playerFollowSelf) CollisionRadius() float64           { return 0 }
func (s *playerFollowSelf) SetHeading(int)                     {}
func (s *playerFollowSelf) SyncPosition(pos location.Location) { s.x, s.y, s.z = pos.X, pos.Y, pos.Z }
func (s *playerFollowSelf) BroadcastMove(event Event) error {
	s.moves = append(s.moves, event)
	return nil
}
func (s *playerFollowSelf) BroadcastStop() error            { return nil }
func (s *playerFollowSelf) OffensiveFollowIsPawnMove() bool { return true }

type followTarget struct {
	x, y, z int
}

func (t *followTarget) ObjectID() int32           { return 2 }
func (t *followTarget) SiegeGuard() bool          { return false }
func (t *followTarget) AlikeDead() bool           { return false }
func (t *followTarget) Position() (int, int, int) { return t.x, t.y, t.z }
func (t *followTarget) CollisionRadius() float64  { return 0 }

var _ attackable.Combatant = (*followTarget)(nil)

func TestControllerPlayerOffensiveFollowUses3DRange(t *testing.T) {
	self := &playerFollowSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewController(mover, self)
	if err != nil {
		t.Fatal(err)
	}

	following, err := controller.MaybeStartOffensiveFollow(&followTarget{z: 41}, 40)
	if err != nil {
		t.Fatal(err)
	}
	if !following {
		t.Fatal("MaybeStartOffensiveFollow() = false, want true when vertical distance exceeds attack range")
	}
}

func TestControllerOffensiveFollowRechecksMovingTargetEveryFivePositionUpdates(t *testing.T) {
	self := &playerFollowSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.afterFunc = func(time.Duration, func()) scheduledTimer { return noAllocTimer{} }
	controller, err := NewController(mover, self)
	if err != nil {
		t.Fatal(err)
	}
	target := &followTarget{x: 100}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}

	target.x = 200
	for range 5 {
		controller.PositionUpdate()
	}

	if got := len(self.moves); got != 2 {
		t.Fatalf("move broadcasts = %d, want 2 after the 500 ms follow recheck", got)
	}
	if got := self.moves[1].Destination; got != (location.Location{X: 200}) {
		t.Fatalf("follow recheck destination = %+v, want target's latest position", got)
	}
}

func TestControllerStopCancelsOffensiveFollowRechecks(t *testing.T) {
	self := &playerFollowSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.afterFunc = func(time.Duration, func()) scheduledTimer { return noAllocTimer{} }
	controller, err := NewController(mover, self)
	if err != nil {
		t.Fatal(err)
	}
	target := &followTarget{x: 100}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}
	if err := controller.Stop(); err != nil {
		t.Fatal(err)
	}

	target.x = 200
	for range 5 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("move broadcasts after Stop() = %d, want 1", got)
	}
}

func TestControllerDefersToActorOwnedOffensiveFollowTicker(t *testing.T) {
	self := &tickerOwnedFollowSelf{}
	mover, err := NewCreatureMove(location.Location{}, 100, staticGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.afterFunc = func(time.Duration, func()) scheduledTimer { return noAllocTimer{} }
	controller, err := NewController(mover, self)
	if err != nil {
		t.Fatal(err)
	}
	target := &followTarget{x: 100}
	if following, err := controller.MaybeStartOffensiveFollow(target, 40); err != nil || !following {
		t.Fatalf("MaybeStartOffensiveFollow() = %v, %v; want active follow", following, err)
	}

	target.x = 200
	for range 5 {
		controller.PositionUpdate()
	}
	if got := len(self.moves); got != 1 {
		t.Fatalf("controller move broadcasts = %d, want only the initial move when actor owns the follow ticker", got)
	}
}

// ---- from creature_allocs_test.go ----
// TestCreatureMove_FollowTickAllocs locks in FollowTick's zero-steady-state
// allocation property (#421, #425): the no-op path (target already in range,
// or not following) must stay allocation-free as AI/follow call sites are
// added, and the move-triggering path's ceiling is the one allocation that's
// inherent to scheduling a new arrival timer through the afterFunc
// indirection (the closure captured for time.AfterFunc-shaped calls always
// escapes to heap, since the compiler can't prove an indirect call won't
// retain it).
func TestCreatureMove_FollowTickAllocs(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	geo := staticGeo{canMove: true, height: 30}

	t.Run("no-op path", func(t *testing.T) {
		mover, err := NewCreatureMove(origin, 50, geo)
		if err != nil {
			t.Fatal(err)
		}
		mover.afterFunc = func(time.Duration, func()) scheduledTimer { return noAllocTimer{} }
		target := TargetSnapshot{ObjectID: 2, Known: true, Position: location.Location{X: 500, Y: 20, Z: 30}}

		allocs := testing.AllocsPerRun(1000, func() {
			if _, moved, err := mover.FollowTick(target, 9.9); err != nil || moved {
				t.Fatalf("FollowTick() = moved %v err %v, want no move", moved, err)
			}
		})
		if allocs != 0 {
			t.Fatalf("FollowTick() no-op path allocs/run = %v, want 0", allocs)
		}
	})

	t.Run("move-triggering path", func(t *testing.T) {
		mover, err := NewCreatureMove(origin, 50, geo)
		if err != nil {
			t.Fatal(err)
		}
		mover.afterFunc = func(time.Duration, func()) scheduledTimer { return noAllocTimer{} }
		mover.StartFriendlyFollow(2, 70)
		target := TargetSnapshot{
			ObjectID:        2,
			Known:           true,
			Position:        location.Location{X: 111, Y: 20, Z: 999},
			CollisionRadius: 10.9,
		}

		// One allocation: the closure scheduling the arrival timer, captured
		// for time.AfterFunc-shaped call through the afterFunc indirection.
		const wantAllocsCeiling = 1
		allocs := testing.AllocsPerRun(1000, func() {
			if _, moved, err := mover.FollowTick(target, 9.9); err != nil || !moved {
				t.Fatalf("FollowTick() = moved %v err %v, want a move", moved, err)
			}
		})
		if allocs != wantAllocsCeiling {
			t.Fatalf("FollowTick() move-triggering path allocs/run = %v, want %v", allocs, wantAllocsCeiling)
		}
	})
}

// ---- from creature_construct_test.go ----
func TestNewCreatureMoveRejectsInvalidDependencies(t *testing.T) {
	tests := []struct {
		name  string
		speed float64
		geo   Geo
	}{
		{name: "nil geodata", speed: 1},
		{name: "negative speed", geo: &recordingGeo{}, speed: -1},
		{name: "not a number speed", geo: &recordingGeo{}, speed: math.NaN()},
		{name: "positive infinite speed", geo: &recordingGeo{}, speed: math.Inf(1)},
		{name: "negative infinite speed", geo: &recordingGeo{}, speed: math.Inf(-1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewCreatureMove(location.Location{}, test.speed, test.geo); err == nil {
				t.Fatal("NewCreatureMove() error = nil")
			}
		})
	}
}

// TestNewCreatureMoveAcceptsZeroSpeed covers an immobile scripted NPC: zero
// speed is a valid stationary state, and MoveToLocation must reject any
// actual movement request rather than the constructor rejecting the actor.
func TestNewCreatureMoveAcceptsZeroSpeed(t *testing.T) {
	geo := &recordingGeo{canMove: true}
	origin := location.Location{X: 10, Y: 20, Z: 30}

	m, err := NewCreatureMove(origin, 0, geo)
	if err != nil {
		t.Fatalf("NewCreatureMove() error = %v, want nil", err)
	}

	if _, err := m.MoveToLocation(location.Location{X: 100, Y: 20, Z: 30}); err == nil {
		t.Fatal("MoveToLocation() error = nil, want error for zero-speed actor")
	}
}

// ---- from creature_fakes_test.go ----
type geoCall struct {
	origin, target location.Location
}

type findPathCall struct {
	origin, target location.Location
}

type validLocationCall struct {
	origin, target location.Location
}

// recordingGeo stands in for the geo/pathfind boundary; per
// docs/agents/test-strategy.md's pure-algorithm/no-boundary exception,
// EngineGeo needs a real geodata engine and pathfinder to construct, which
// is disproportionate for these move-resolution unit tests. Kept as-is.
type recordingGeo struct {
	canMove            bool
	canMoveAt          func(ox, oy, oz, tx, ty, tz int) bool
	height             int16
	findPath           []location.Location
	findPathOK         bool
	validLocation      location.Location
	heightCalls        []location.Location
	moveCalls          []geoCall
	findPathCalls      []findPathCall
	validLocationCalls []validLocationCall
}

func (g *recordingGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool {
	g.moveCalls = append(g.moveCalls, geoCall{
		origin: location.Location{X: ox, Y: oy, Z: oz},
		target: location.Location{X: tx, Y: ty, Z: tz},
	})
	if g.canMoveAt != nil {
		return g.canMoveAt(ox, oy, oz, tx, ty, tz)
	}
	return g.canMove
}

func (g *recordingGeo) Height(x, y, z int) int16 {
	g.heightCalls = append(g.heightCalls, location.Location{X: x, Y: y, Z: z})
	return g.height
}

func (g *recordingGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	g.findPathCalls = append(g.findPathCalls, findPathCall{origin: origin, target: target})
	return g.findPath, g.findPathOK
}

func (g *recordingGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	g.validLocationCalls = append(g.validLocationCalls, validLocationCall{
		origin: location.Location{X: ox, Y: oy, Z: oz},
		target: location.Location{X: tx, Y: ty, Z: tz},
	})
	// Unset means "no progress", mirroring the real engine's same-cell fallback.
	if g.validLocation == (location.Location{}) {
		return location.Location{X: ox, Y: oy, Z: oz}
	}
	return g.validLocation
}

func (g *recordingGeo) Walkable(int, int, int) bool { return true }

// staticGeo is a zero-allocation Geo stub for allocation-ceiling tests:
// recordingGeo's call-log slices grow and occasionally reallocate, which
// would add noise to a per-call allocation measurement.
type staticGeo struct {
	canMove bool
	height  int16
}

func (g staticGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return g.canMove }

func (g staticGeo) Height(x, y, z int) int16 { return g.height }

// staticGeo never reports a found path or partial-progress fall-back: it
// models terrain that has either an open line (canMove=true) or an absolute
// block (canMove=false), whose fallback is a zero-distance arrival.
func (g staticGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }

func (g staticGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func (g staticGeo) Walkable(int, int, int) bool { return true }

// fakeMoveClock/fakeMoveTimer are the sanctioned clock/timer infra-seam
// exception (docs/agents/test-strategy.md): determinism is the point, not
// integration coverage. Kept as-is.
type fakeMoveClock struct {
	timers []*fakeMoveTimer
}

func (c *fakeMoveClock) AfterFunc(delay time.Duration, f func()) scheduledTimer {
	timer := &fakeMoveTimer{delay: delay, f: f}
	c.timers = append(c.timers, timer)
	return timer
}

// fire runs every still-pending timer, latest scheduled first, so a
// superseded earlier timer (already Stop()ped by the newer request) is
// correctly skipped even though both share the same delay.
func (c *fakeMoveClock) fire(delay time.Duration) {
	for i := len(c.timers) - 1; i >= 0; i-- {
		timer := c.timers[i]
		if timer.delay == delay && !timer.stopped {
			timer.stopped = true
			timer.f()
		}
	}
}

type fakeMoveTimer struct {
	delay   time.Duration
	f       func()
	stopped bool
}

func (t *fakeMoveTimer) Stop() bool {
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}

// noAllocTimer is a zero-size scheduledTimer: converting a zero-width value
// to an interface does not allocate, so installing it as afterFunc isolates
// FollowTick's own allocation profile from the real runtime timer's.
type noAllocTimer struct{}

func (noAllocTimer) Stop() bool { return true }

// ---- from creature_follow_test.go ----
func TestCreatureMove_FollowTickUsesCurrentPosition(t *testing.T) {
	spawn := location.Location{X: 0, Y: 0, Z: 0}
	current := location.Location{X: 100, Y: 0, Z: 0}
	target := TargetSnapshot{
		ObjectID:        2,
		Known:           true,
		Position:        location.Location{X: 129, Y: 0, Z: 0},
		CollisionRadius: 5,
	}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(spawn, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetPosition(current)
	mover.StartFriendlyFollow(target.ObjectID, 20)

	event, moved, err := mover.FollowTick(target, 5)
	if err != nil {
		t.Fatal(err)
	}
	if moved {
		t.Fatalf("FollowTick() moved = true with event %+v", event)
	}
	if len(geo.moveCalls) != 0 {
		t.Fatalf("CanMove() calls = %+v, want none", geo.moveCalls)
	}
}

func TestCreatureMove_FriendlyFollowTick(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	target := TargetSnapshot{
		ObjectID:        2,
		Position:        location.Location{X: 111, Y: 20, Z: 999},
		CollisionRadius: 10.9,
		Known:           true,
	}
	geo := &recordingGeo{canMove: true, height: 30}
	mover, err := NewCreatureMove(origin, 50, geo)
	if err != nil {
		t.Fatal(err)
	}

	mover.StartFriendlyFollow(target.ObjectID, 70)
	event, moved, err := mover.FollowTick(target, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick() moved = false, want true")
	}

	want := Event{
		Origin:      origin,
		Destination: location.Location{X: 111, Y: 20, Z: 30},
		Speed:       50,
		Duration:    2100 * time.Millisecond,
	}
	if event != want {
		t.Fatalf("FollowTick() event = %+v, want %+v", event, want)
	}
	if got := mover.Destination(); got != want.Destination {
		t.Fatalf("Destination() = %+v, want %+v", got, want.Destination)
	}
	if !mover.Following() {
		t.Fatal("Following() = false, want true")
	}
	if got := mover.FollowInterval(); got != time.Second {
		t.Fatalf("FollowInterval() = %v, want %v", got, time.Second)
	}
}

func TestCreatureMove_FriendlyFollowTickMovesAtExactRange(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	target := TargetSnapshot{
		ObjectID:        2,
		Known:           true,
		Position:        location.Location{X: 100, Y: 20, Z: 30},
		CollisionRadius: 10.9,
	}
	mover, err := NewCreatureMove(origin, 50, &recordingGeo{canMove: true, height: 30})
	if err != nil {
		t.Fatal(err)
	}
	mover.StartFriendlyFollow(target.ObjectID, 70)

	event, moved, err := mover.FollowTick(target, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick() moved = false at the exact follow range, want true")
	}
	if event.Destination != target.Position {
		t.Fatalf("FollowTick() destination = %+v, want %+v", event.Destination, target.Position)
	}
}

func TestCreatureMove_FollowTickSkipsWhenTargetDoesNotNeedMove(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	tests := []struct {
		name     string
		target   TargetSnapshot
		start    func(*CreatureMove)
		wantMode FollowMode
	}{
		{
			name: "not following",
			target: TargetSnapshot{
				ObjectID: 2,
				Known:    true,
				Position: location.Location{X: 500, Y: 20, Z: 30},
			},
		},
		{
			name: "unknown friendly target",
			target: TargetSnapshot{
				ObjectID: 2,
				Position: location.Location{X: 500, Y: 20, Z: 30},
			},
			start:    func(m *CreatureMove) { m.StartFriendlyFollow(2, 70) },
			wantMode: FollowFriendly,
		},
		{
			name: "different target snapshot",
			target: TargetSnapshot{
				ObjectID: 3,
				Known:    true,
				Position: location.Location{X: 500, Y: 20, Z: 30},
			},
			start:    func(m *CreatureMove) { m.StartFriendlyFollow(2, 70) },
			wantMode: FollowFriendly,
		},
		{
			name: "friendly target in boat",
			target: TargetSnapshot{
				ObjectID: 2,
				Known:    true,
				InBoat:   true,
				Position: location.Location{X: 500, Y: 20, Z: 30},
			},
			start:    func(m *CreatureMove) { m.StartFriendlyFollow(2, 70) },
			wantMode: FollowFriendly,
		},
		{
			name: "inside collision-adjusted range",
			target: TargetSnapshot{
				ObjectID:        2,
				Known:           true,
				Position:        location.Location{X: 99, Y: 20, Z: 30},
				CollisionRadius: 10.9,
			},
			start:    func(m *CreatureMove) { m.StartFriendlyFollow(2, 70) },
			wantMode: FollowFriendly,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			geo := &recordingGeo{canMove: true, height: 30}
			mover, err := NewCreatureMove(origin, 50, geo)
			if err != nil {
				t.Fatal(err)
			}
			if test.start != nil {
				test.start(mover)
			}

			event, moved, err := mover.FollowTick(test.target, 9.9)
			if err != nil {
				t.Fatal(err)
			}
			if moved {
				t.Fatalf("FollowTick() moved = true with event %+v", event)
			}
			if event != (Event{}) {
				t.Fatalf("FollowTick() event = %+v, want zero", event)
			}
			if got := mover.Destination(); got != origin {
				t.Fatalf("Destination() = %+v, want %+v", got, origin)
			}
			if len(geo.moveCalls) != 0 {
				t.Fatalf("CanMove() calls = %+v, want none", geo.moveCalls)
			}
			if got := mover.FollowMode(); got != test.wantMode {
				t.Fatalf("FollowMode() = %v, want %v", got, test.wantMode)
			}
		})
	}
}

func TestCreatureMove_OffensiveFollowTick(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}

	mover.StartOffensiveFollow(9, 40)
	if got := mover.FollowInterval(); got != 500*time.Millisecond {
		t.Fatalf("FollowInterval() = %v, want %v", got, 500*time.Millisecond)
	}

	inRange := TargetSnapshot{ObjectID: 9, Known: true, Position: location.Location{X: 58, Y: 0}, CollisionRadius: 10}
	if event, moved, err := mover.FollowTick(inRange, 9.9); err != nil || moved || event != (Event{}) {
		t.Fatalf("FollowTick(in range) = event %+v moved %v err %v, want no move", event, moved, err)
	}

	outside := TargetSnapshot{ObjectID: 9, Known: true, Position: location.Location{X: 59, Y: 0}, CollisionRadius: 10}
	event, moved, err := mover.FollowTick(outside, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick(outside) moved = false, want true")
	}
	want := Event{
		Origin:       origin,
		Destination:  location.Location{X: 59, Y: 0, Z: 0},
		Speed:        100,
		Duration:     600 * time.Millisecond,
		FollowTarget: 9,
		FollowOffset: 40,
	}
	if event != want {
		t.Fatalf("FollowTick(outside) event = %+v, want %+v", event, want)
	}
}

func TestCreatureMove_CancelFollow(t *testing.T) {
	geo := &recordingGeo{canMove: true}
	mover, err := NewCreatureMove(location.Location{}, 50, geo)
	if err != nil {
		t.Fatal(err)
	}

	mover.StartFriendlyFollow(2, 70)
	mover.CancelFollow()

	if mover.Following() {
		t.Fatal("Following() = true, want false")
	}
	if got := mover.FollowMode(); got != FollowNone {
		t.Fatalf("FollowMode() = %v, want %v", got, FollowNone)
	}
	if got := mover.FollowInterval(); got != 0 {
		t.Fatalf("FollowInterval() = %v, want 0", got)
	}
}

// ---- from creature_move_test.go ----
type movementDynamicObject struct {
	x, y, z int
	data    [][]block.NSWE
}

func (o movementDynamicObject) GeoX() int               { return o.x }
func (o movementDynamicObject) GeoY() int               { return o.y }
func (o movementDynamicObject) GeoZ() int               { return o.z }
func (o movementDynamicObject) Height() int             { return 32 }
func (o movementDynamicObject) GeoData() [][]block.NSWE { return o.data }

type heightGeo struct{ engine *engine.Engine }

func (g heightGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (g heightGeo) Height(x, y, z int) int16          { return g.engine.Height(x, y, z) }
func (heightGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (heightGeo) Walkable(int, int, int) bool { return true }
func (heightGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func TestCreatureMove_MoveToLocationScenarios(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	previous := location.Location{X: 60, Y: 20, Z: 30}
	minInt := -int(^uint(0)>>1) - 1
	maxInt := int(^uint(0) >> 1)
	extremeOrigin := location.Location{X: minInt, Y: minInt, Z: 30}
	extremeTarget := location.Location{X: maxInt, Y: maxInt, Z: 999}
	tests := []struct {
		name              string
		origin            *location.Location
		speed             float64
		canMove           bool
		target            location.Location
		initialTarget     *location.Location
		blockAfterInitial bool
		wantEvent         Event
		wantErr           bool
		wantDestination   location.Location
		wantMoving        bool
	}{
		{
			name:            "normalizes height and uses Java tick duration",
			canMove:         true,
			target:          location.Location{X: 60, Y: 20, Z: 999},
			wantEvent:       Event{Origin: origin, Destination: previous, Speed: 50, Duration: time.Second},
			wantDestination: previous,
			wantMoving:      true,
		},
		{
			name:            "rounds one unit up to one tick",
			canMove:         true,
			target:          location.Location{X: 11, Y: 20, Z: 999},
			wantEvent:       Event{Origin: origin, Destination: location.Location{X: 11, Y: 20, Z: 30}, Speed: 50, Duration: 100 * time.Millisecond},
			wantDestination: location.Location{X: 11, Y: 20, Z: 30},
			wantMoving:      true,
		},
		{
			name:            "rounds fifty-one units up to eleven ticks",
			canMove:         true,
			target:          location.Location{X: 61, Y: 20, Z: 999},
			wantEvent:       Event{Origin: origin, Destination: location.Location{X: 61, Y: 20, Z: 30}, Speed: 50, Duration: 1100 * time.Millisecond},
			wantDestination: location.Location{X: 61, Y: 20, Z: 30},
			wantMoving:      true,
		},
		{
			name:            "accepts blocked route as zero-distance arrival",
			target:          location.Location{X: 60, Y: 20},
			wantEvent:       Event{Origin: origin, Destination: origin, Speed: 50},
			wantDestination: origin,
			wantMoving:      true,
		},
		{
			name:            "same position has zero duration",
			canMove:         true,
			target:          origin,
			wantEvent:       Event{Origin: origin, Destination: origin, Speed: 50},
			wantDestination: origin,
			wantMoving:      true,
		},
		{
			name:            "same position accepts the smallest finite speed",
			speed:           math.SmallestNonzeroFloat64,
			canMove:         true,
			target:          location.Location{X: origin.X, Y: origin.Y, Z: 999},
			wantEvent:       Event{Origin: origin, Destination: origin, Speed: math.SmallestNonzeroFloat64},
			wantDestination: origin,
			wantMoving:      true,
		},
		{
			name:            "rejects extreme coordinates without changing state",
			origin:          &extremeOrigin,
			speed:           0.01,
			canMove:         true,
			target:          extremeTarget,
			wantErr:         true,
			wantDestination: extremeOrigin,
		},
		{
			name:              "blocked follow-up replaces state with zero-distance arrival",
			canMove:           true,
			initialTarget:     &location.Location{X: 60, Y: 20},
			blockAfterInitial: true,
			target:            location.Location{X: 70, Y: 20},
			wantEvent:         Event{Origin: origin, Destination: origin, Speed: 50},
			wantDestination:   origin,
			wantMoving:        true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			moverOrigin := origin
			if test.origin != nil {
				moverOrigin = *test.origin
			}
			speed := 50.0
			if test.speed != 0 {
				speed = test.speed
			}
			geo := &recordingGeo{canMove: test.canMove, height: 30}
			mover, err := NewCreatureMove(moverOrigin, speed, geo)
			if err != nil {
				t.Fatal(err)
			}
			if test.initialTarget != nil {
				if _, err := mover.MoveToLocation(*test.initialTarget); err != nil {
					t.Fatal(err)
				}
			}
			if test.blockAfterInitial {
				geo.canMove = false
			}

			event, err := mover.MoveToLocation(test.target)
			if (err != nil) != test.wantErr {
				t.Fatalf("MoveToLocation() error = %v, want error = %v", err, test.wantErr)
			}
			if !test.wantErr && event != test.wantEvent {
				t.Fatalf("event = %+v, want %+v", event, test.wantEvent)
			}
			if got := mover.Destination(); got != test.wantDestination {
				t.Fatalf("Destination() = %+v, want %+v", got, test.wantDestination)
			}
			if got := mover.Moving(); got != test.wantMoving {
				t.Fatalf("Moving() = %v, want %v", got, test.wantMoving)
			}
		})
	}
}

func TestCreatureMove_MoveToLocationPassesGeodataCoordinates(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	target := location.Location{X: 60, Y: 70, Z: 999}
	geo := &recordingGeo{canMove: true, height: 42}
	mover, err := NewCreatureMove(origin, 50, geo)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := mover.MoveToLocation(target); err != nil {
		t.Fatal(err)
	}

	if len(geo.heightCalls) != 1 || geo.heightCalls[0] != target {
		t.Fatalf("Height() calls = %+v, want [%+v]", geo.heightCalls, target)
	}
	wantMove := geoCall{origin: origin, target: location.Location{X: target.X, Y: target.Y, Z: 42}}
	if len(geo.moveCalls) != 1 || geo.moveCalls[0] != wantMove {
		t.Fatalf("CanMove() calls = %+v, want [%+v]", geo.moveCalls, wantMove)
	}
}

func TestCreatureMove_UpdatePositionStopsWhenObstacleCloses(t *testing.T) {
	geo := &recordingGeo{canMove: true}
	mover, err := NewCreatureMove(location.Location{}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	arrived := 0
	blocked := 0
	mover.SetArrivedHook(func() { arrived++ })
	mover.SetBlockedHook(func() { blocked++ })
	if _, err := mover.MoveToLocation(location.Location{X: 100}); err != nil {
		t.Fatal(err)
	}

	if _, moving := mover.UpdatePosition(PositionUpdateInterval); !moving {
		t.Fatal("first UpdatePosition() stopped move, want moving")
	}
	geo.canMove = false
	if _, moving := mover.UpdatePosition(PositionUpdateInterval); moving {
		t.Fatal("UpdatePosition() moving = true after obstacle closes, want false")
	}

	if got := mover.Position(); got != (location.Location{X: 10}) {
		t.Fatalf("Position() = %+v, want %+v", got, location.Location{X: 10})
	}
	if arrived != 0 {
		t.Fatalf("arrived hook calls = %d, want 0", arrived)
	}
	if blocked != 1 {
		t.Fatalf("blocked hook calls = %d, want 1", blocked)
	}
	want := geoCall{origin: location.Location{X: 10}, target: location.Location{X: 20}}
	if got := geo.moveCalls[len(geo.moveCalls)-1]; got != want {
		t.Fatalf("last CanMove() call = %+v, want %+v", got, want)
	}
}

func TestCreatureMove_UpdatePositionChecksFinalStepForNewObstacle(t *testing.T) {
	geo := &recordingGeo{canMove: true}
	mover, err := NewCreatureMove(location.Location{}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	arrived := 0
	blocked := 0
	mover.SetArrivedHook(func() { arrived++ })
	mover.SetBlockedHook(func() { blocked++ })
	if _, err := mover.MoveToLocation(location.Location{X: 10}); err != nil {
		t.Fatal(err)
	}

	geo.canMove = false
	if _, moving := mover.UpdatePosition(PositionUpdateInterval); moving {
		t.Fatal("UpdatePosition() moving = true after obstacle closes, want false")
	}
	if got := mover.Position(); got != (location.Location{}) {
		t.Fatalf("Position() = %+v, want origin", got)
	}
	if arrived != 0 || blocked != 1 {
		t.Fatalf("arrival callbacks = (%d, %d), want (0, 1)", arrived, blocked)
	}
}

func TestCreatureMove_UpdatePositionStopsWhenDynamicNSWECloses(t *testing.T) {
	e := engine.New()
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewFlat(0)})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetRegion(engine.TileXMin, engine.TileYMin, region); err != nil {
		t.Fatal(err)
	}
	origin := location.Location{X: engine.WorldX(0), Y: engine.WorldY(0)}
	target := location.Location{X: engine.WorldX(2), Y: origin.Y}
	mover, err := NewCreatureMove(origin, 160, NewGeo(e, nil))
	if err != nil {
		t.Fatal(err)
	}
	arrived := 0
	blocked := 0
	mover.SetArrivedHook(func() { arrived++ })
	mover.SetBlockedHook(func() { blocked++ })
	if _, err := mover.MoveToLocation(target); err != nil {
		t.Fatal(err)
	}
	if _, moving := mover.UpdatePosition(PositionUpdateInterval); !moving {
		t.Fatal("first UpdatePosition() stopped move, want moving")
	}

	e.AddObject(movementDynamicObject{x: 1, y: 0, data: [][]block.NSWE{{block.NoDirections}}})
	if _, moving := mover.UpdatePosition(PositionUpdateInterval); moving {
		t.Fatal("UpdatePosition() moving = true after dynamic NSWE closes, want false")
	}
	if got := mover.Position(); got != (location.Location{X: engine.WorldX(1), Y: origin.Y}) {
		t.Fatalf("Position() = %+v, want position before dynamic obstacle", got)
	}
	if arrived != 0 || blocked != 1 {
		t.Fatalf("arrival callbacks = (%d, %d), want (0, 1)", arrived, blocked)
	}
}

func TestCreatureMove_UpdatePositionResamplesDestinationHeight(t *testing.T) {
	e := engine.New()
	region, err := block.NewRegionFromBlocks([]block.Block{block.NewFlat(0)})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetRegion(engine.TileXMin, engine.TileYMin, region); err != nil {
		t.Fatal(err)
	}
	origin := location.Location{X: engine.WorldX(0), Y: engine.WorldY(0)}
	target := location.Location{X: engine.WorldX(2), Y: origin.Y}
	mover, err := NewCreatureMove(origin, 100, heightGeo{engine: e})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mover.MoveToLocation(target); err != nil {
		t.Fatal(err)
	}
	const step = 10 * time.Millisecond
	if _, moving := mover.UpdatePosition(step); !moving {
		t.Fatal("first UpdatePosition() stopped move, want moving")
	}

	e.AddObject(movementDynamicObject{x: 2, y: 0, data: [][]block.NSWE{{block.NoDirections}}})
	for range 40 {
		if _, moving := mover.UpdatePosition(step); !moving {
			break
		}
	}
	if mover.Moving() {
		t.Fatal("UpdatePosition() did not reach destination")
	}
	if got := mover.Position(); got != (location.Location{X: target.X, Y: target.Y, Z: 32}) {
		t.Fatalf("Position() = %+v, want destination at dynamic height", got)
	}
}

func TestCreatureMove_UpdatePositionBiasesGroundHeightAndCapsWorldZ(t *testing.T) {
	origin := location.Location{Z: 100}
	target := location.Location{X: 100}
	geo := &recordingGeo{canMove: true, height: 16420}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mover.MoveToLocation(target); err != nil {
		t.Fatal(err)
	}
	if _, moving := mover.UpdatePosition(100 * time.Millisecond); !moving {
		t.Fatal("UpdatePosition() stopped move, want moving")
	}

	wantHeightCalls := []location.Location{target, {X: target.X, Z: 16420}, {X: 10, Z: 116}}
	if len(geo.heightCalls) != len(wantHeightCalls) || geo.heightCalls[0] != wantHeightCalls[0] || geo.heightCalls[1] != wantHeightCalls[1] || geo.heightCalls[2] != wantHeightCalls[2] {
		t.Fatalf("Height() calls = %+v, want %+v", geo.heightCalls, wantHeightCalls)
	}
	if got := mover.Position(); got != (location.Location{X: 10, Z: 16410}) {
		t.Fatalf("Position() = %+v, want upward-layer height capped at world max", got)
	}
	if _, moving := mover.UpdatePosition(time.Second); moving {
		t.Fatal("UpdatePosition() still moving at destination")
	}
	if got := mover.Position(); got != (location.Location{X: target.X, Z: 16410}) {
		t.Fatalf("Position() = %+v, want destination capped at world max", got)
	}
}

func TestCreatureMove_MoveToLocationUsesCurrentPosition(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	current := location.Location{X: 60, Y: 20, Z: 30}
	geo := &recordingGeo{canMove: true, height: 30}
	mover, err := NewCreatureMove(origin, 50, geo)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := mover.MoveToLocation(current); err != nil {
		t.Fatal(err)
	}
	mover.SetPosition(current)

	event, err := mover.MoveToLocation(location.Location{X: 70, Y: 20, Z: 999})
	if err != nil {
		t.Fatal(err)
	}

	want := Event{
		Origin:      current,
		Destination: location.Location{X: 70, Y: 20, Z: 30},
		Speed:       50,
		Duration:    200 * time.Millisecond,
	}
	if event != want {
		t.Fatalf("MoveToLocation() event = %+v, want %+v", event, want)
	}
	wantMove := geoCall{origin: current, target: want.Destination}
	if got := geo.moveCalls[len(geo.moveCalls)-1]; got != wantMove {
		t.Fatalf("last CanMove() call = %+v, want %+v", got, wantMove)
	}
	if got := mover.Position(); got != current {
		t.Fatalf("Position() = %+v, want %+v", got, current)
	}
}

func TestCreatureMove_MoveToLocationRejectsUnrepresentableDuration(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	geo := &recordingGeo{canMove: true, height: 30}
	mover, err := NewCreatureMove(origin, math.SmallestNonzeroFloat64, geo)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := mover.MoveToLocation(location.Location{X: 11, Y: 20, Z: 999}); err == nil {
		t.Fatal("MoveToLocation() error = nil")
	}
	if got := mover.Destination(); got != origin {
		t.Fatalf("Destination() = %+v, want %+v", got, origin)
	}
	if mover.Moving() {
		t.Fatal("Moving() = true, want false")
	}
}

// ---- from creature_pathfind_test.go ----
// TestCreatureMove_MoveToLocationRoutesThroughPathfoundWaypoints covers the
// tier-2 case: a blocked direct line that the geopath resolves into three
// segments. The accepted request walks each segment in turn, broadcasts a
// per-segment destination, and fires the arrived hook exactly once — at the
// final segment's completion — not once per intermediate waypoint.
func TestCreatureMove_MoveToLocationRoutesThroughPathfindWaypoints(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 30}
	// The pathfinder returns corners plus the final cell, omitting the
	// origin: three cells → three segments to walk sequentially.
	waypoints := []location.Location{
		{X: 50, Y: 0, Z: 30},
		{X: 50, Y: 50, Z: 30},
		{X: 100, Y: 50, Z: 30},
	}
	geo := &recordingGeo{
		canMove:    false, // direct line blocked → tier 2 pathfinding
		height:     30,
		findPath:   waypoints,
		findPathOK: true,
	}
	mover, err := NewCreatureMove(origin, 50, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	event, err := mover.MoveToLocation(location.Location{X: 100, Y: 50, Z: 30})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v, want nil", err)
	}
	// The active destination is the first geopath entry; the remaining two
	// are queued inside CreatureMove.
	wantFirst := location.Location{X: 50, Y: 0, Z: 30}
	if event.Destination != wantFirst {
		t.Fatalf("event.Destination = %+v, want %+v (first waypoint)", event.Destination, wantFirst)
	}
	if got := mover.Destination(); got != wantFirst {
		t.Fatalf("Destination() = %+v, want %+v", got, wantFirst)
	}
	if got := len(geo.findPathCalls); got != 1 {
		t.Fatalf("FindPath() calls = %d, want 1", got)
	}
	if geo.findPathCalls[0].origin != origin || geo.findPathCalls[0].target != (location.Location{X: 100, Y: 50, Z: 30}) {
		t.Fatalf("FindPath() args = %+v -> %+v, want origin -> target", geo.findPathCalls[0].origin, geo.findPathCalls[0].target)
	}
	// The partial-fallback query never runs when pathfinding succeeds.
	if got := len(geo.validLocationCalls); got != 0 {
		t.Fatalf("ValidLocation() calls = %d, want 0", got)
	}

	// Walk each segment by firing its arrival timer. Each fire() runs
	// finishLocked: the first two advance to the next waypoint (returning
	// nil, no arrival), and the third exhausts the queue and fires the
	// arrived hook once. Every segment shares the same duration since the
	// geometry is uniform, so firing event.Duration repeatedly is correct.
	for i := range waypoints {
		if !mover.Moving() {
			t.Fatalf("Moving() = false before firing segment %d", i)
		}
		clock.fire(event.Duration)
	}

	// After the last segment finishes, the arrived hook fires exactly once,
	// and the actor rests at the final waypoint.
	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls = %d, want 1 (final segment only)", arrivedCalls)
	}
	if mover.Moving() {
		t.Fatal("Moving() = true after final segment, want false")
	}
	if got := mover.Position(); got != waypoints[len(waypoints)-1] {
		t.Fatalf("Position() = %+v, want %+v (final waypoint)", got, waypoints[len(waypoints)-1])
	}
}

// TestCreatureMove_MoveToLocationPartialFallbackWalksPartialRoute covers the
// tier-3 case: blocked direct line, no pathfind route, but a partial-progress
// fall-back point exists further along the line. The request succeeds with
// the fall-back as the destination and no waypoint queue.
func TestCreatureMove_MoveToLocationPartialFallbackWalksPartialRoute(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 30}
	fallback := location.Location{X: 25, Y: 0, Z: 30}
	geo := &recordingGeo{
		canMove:       false,
		height:        30,
		findPath:      nil,
		findPathOK:    false,
		validLocation: fallback,
	}
	mover, err := NewCreatureMove(origin, 50, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	event, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 30})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v, want nil (tier 3 fall-back)", err)
	}
	if event.Destination != fallback {
		t.Fatalf("event.Destination = %+v, want fall-back %+v", event.Destination, fallback)
	}
	if got := mover.Destination(); got != fallback {
		t.Fatalf("Destination() = %+v, want %+v", got, fallback)
	}
	if got := len(geo.validLocationCalls); got != 1 {
		t.Fatalf("ValidLocation() calls = %d, want 1", got)
	}

	clock.fire(event.Duration)
	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls = %d, want 1", arrivedCalls)
	}
	if got := mover.Position(); got != fallback {
		t.Fatalf("Position() = %+v, want fall-back %+v", got, fallback)
	}
}

// TestCreatureMove_MoveToLocationNoProgressFallbackStartsZeroDistanceArrival
// covers the tier-3 fully-blocked edge: the straight line is blocked,
// pathfinding finds no route, and the partial fallback resolves to the origin.
func TestCreatureMove_MoveToLocationNoProgressFallbackStartsZeroDistanceArrival(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 30}
	prior := location.Location{X: -42, Y: -42, Z: 30}
	geo := &recordingGeo{
		canMove:    false,
		height:     30,
		findPath:   nil,
		findPathOK: false,
		// ValidLocation left zero → stub returns the call's origin.
	}
	mover, err := NewCreatureMove(origin, 50, geo)
	if err != nil {
		t.Fatal(err)
	}
	// Seed an in-flight destination so the new request must replace it.
	mover.destination = prior
	mover.moving = true

	event, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 30})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v, want nil", err)
	}
	if want := (Event{Origin: origin, Destination: origin, Speed: 50}); event != want {
		t.Fatalf("MoveToLocation() event = %+v, want %+v", event, want)
	}
	if got := mover.Destination(); got != origin {
		t.Fatalf("Destination() = %+v, want origin %+v", got, origin)
	}
	if !mover.Moving() {
		t.Fatal("Moving() = false, want zero-distance arrival pending")
	}
}

// ---- from scatter_test.go ----
func TestRandomNearbyLocationSnapsHeightAndStaysWithinOffset(t *testing.T) {
	geo := staticGeo{canMove: true, height: 42}
	target := location.Location{X: 1000, Y: 1000, Z: 0}

	got := RandomNearbyLocation(geo, target, 20)

	if got.Z != 42 {
		t.Fatalf("Z = %d, want snapped height 42", got.Z)
	}
	if dx := got.X - target.X; dx < -20 || dx > 20 {
		t.Fatalf("X = %d, want within 20 of %d", got.X, target.X)
	}
	if dy := got.Y - target.Y; dy < -20 || dy > 20 {
		t.Fatalf("Y = %d, want within 20 of %d", got.Y, target.Y)
	}
}

func TestRandomNearbyLocationKeepsTargetWhenScatterBlocked(t *testing.T) {
	geo := staticGeo{canMove: false, height: 7}
	target := location.Location{X: 1000, Y: 1000, Z: 0}

	got := RandomNearbyLocation(geo, target, 20)

	if got.X != target.X || got.Y != target.Y {
		t.Fatalf("X,Y = %d,%d, want unchanged target %d,%d (scatter blocked)", got.X, got.Y, target.X, target.Y)
	}
	if got.Z != 7 {
		t.Fatalf("Z = %d, want snapped height 7", got.Z)
	}
}

func TestRandomNearbyLocationSkipsScatterForNonPositiveOffset(t *testing.T) {
	geo := staticGeo{canMove: true, height: 9}
	target := location.Location{X: 1000, Y: 1000, Z: 0}

	got := RandomNearbyLocation(geo, target, 0)

	if got.X != target.X || got.Y != target.Y {
		t.Fatalf("X,Y = %d,%d, want unchanged target %d,%d (offset <= 0)", got.X, got.Y, target.X, target.Y)
	}
}

func TestRandomNearbyLocationNilGeoReturnsTargetUnchanged(t *testing.T) {
	target := location.Location{X: 1000, Y: 1000, Z: 5}

	got := RandomNearbyLocation(nil, target, 20)

	if got != target {
		t.Fatalf("RandomNearbyLocation(nil, ...) = %+v, want unchanged %+v", got, target)
	}
}
