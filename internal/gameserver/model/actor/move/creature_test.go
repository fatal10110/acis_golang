package move

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type geoCall struct {
	origin, target location.Location
}

type recordingGeo struct {
	canMove     bool
	height      int16
	heightCalls []location.Location
	moveCalls   []geoCall
}

func (g *recordingGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool {
	g.moveCalls = append(g.moveCalls, geoCall{
		origin: location.Location{X: ox, Y: oy, Z: oz},
		target: location.Location{X: tx, Y: ty, Z: tz},
	})
	return g.canMove
}

func (g *recordingGeo) Height(x, y, z int) int16 {
	g.heightCalls = append(g.heightCalls, location.Location{X: x, Y: y, Z: z})
	return g.height
}

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
			name:            "rejects blocked route",
			target:          location.Location{X: 60, Y: 20},
			wantErr:         true,
			wantDestination: origin,
		},
		{
			name:            "same position has zero duration",
			canMove:         true,
			target:          origin,
			wantEvent:       Event{Origin: origin, Destination: origin, Speed: 50},
			wantDestination: origin,
		},
		{
			name:            "same position accepts the smallest finite speed",
			speed:           math.SmallestNonzeroFloat64,
			canMove:         true,
			target:          location.Location{X: origin.X, Y: origin.Y, Z: 999},
			wantEvent:       Event{Origin: origin, Destination: origin, Speed: math.SmallestNonzeroFloat64},
			wantDestination: origin,
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
			name:              "blocked follow-up preserves state",
			canMove:           true,
			initialTarget:     &location.Location{X: 60, Y: 20},
			blockAfterInitial: true,
			target:            location.Location{X: 70, Y: 20},
			wantErr:           true,
			wantDestination:   previous,
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

func TestCreatureMove_FollowTickUsesCurrentPosition(t *testing.T) {
	spawn := location.Location{X: 0, Y: 0, Z: 0}
	current := location.Location{X: 100, Y: 0, Z: 0}
	target := TargetSnapshot{
		ObjectID:        2,
		Known:           true,
		Position:        location.Location{X: 130, Y: 0, Z: 0},
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
				Position:        location.Location{X: 100, Y: 20, Z: 30},
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

	inRange := TargetSnapshot{ObjectID: 9, Known: true, Position: location.Location{X: 59, Y: 0}, CollisionRadius: 10}
	if event, moved, err := mover.FollowTick(inRange, 9.9); err != nil || moved || event != (Event{}) {
		t.Fatalf("FollowTick(in range) = event %+v moved %v err %v, want no move", event, moved, err)
	}

	outside := TargetSnapshot{ObjectID: 9, Known: true, Position: location.Location{X: 60, Y: 0}, CollisionRadius: 10}
	event, moved, err := mover.FollowTick(outside, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick(outside) moved = false, want true")
	}
	want := Event{
		Origin:       origin,
		Destination:  location.Location{X: 60, Y: 0, Z: 0},
		Speed:        100,
		Duration:     600 * time.Millisecond,
		FollowTarget: 9,
		FollowOffset: 40,
	}
	if event != want {
		t.Fatalf("FollowTick(outside) event = %+v, want %+v", event, want)
	}
}

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

func TestCreatureMove_MoveToLocationFiresArrivedOnceDurationElapses(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	target := location.Location{X: 100, Y: 0, Z: 0}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	event, err := mover.MoveToLocation(target)
	if err != nil {
		t.Fatal(err)
	}
	if !mover.Moving() {
		t.Fatal("Moving() = false immediately after an accepted move, want true")
	}
	if got := mover.Position(); got != origin {
		t.Fatalf("Position() = %+v before arrival, want %+v", got, origin)
	}

	clock.fire(event.Duration)

	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls = %d, want 1", arrivedCalls)
	}
	if got := mover.Position(); got != target {
		t.Fatalf("Position() = %+v after arrival, want %+v", got, target)
	}
	if mover.Moving() {
		t.Fatal("Moving() = true after arrival, want false")
	}
}

func TestCreatureMove_MoveToLocationSupersedesPendingArrival(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	first, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 0})
	if err != nil {
		t.Fatal(err)
	}
	second, err := mover.MoveToLocation(location.Location{X: 200, Y: 0, Z: 0})
	if err != nil {
		t.Fatal(err)
	}

	// The superseded first timer must not move the actor once the second
	// request has changed the destination.
	clock.fire(first.Duration)
	if arrivedCalls != 0 {
		t.Fatalf("arrived hook calls after stale timer = %d, want 0", arrivedCalls)
	}
	if got := mover.Position(); got != origin {
		t.Fatalf("Position() = %+v after stale timer, want %+v", got, origin)
	}

	clock.fire(second.Duration)
	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls after current timer = %d, want 1", arrivedCalls)
	}
	if got := mover.Position(); got != (location.Location{X: 200, Y: 0, Z: 0}) {
		t.Fatalf("Position() = %+v, want %+v", got, location.Location{X: 200, Y: 0, Z: 0})
	}
}

func TestCreatureMove_CancelMoveStopsArrival(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	event, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 0})
	if err != nil {
		t.Fatal(err)
	}
	mover.CancelMove()
	clock.fire(event.Duration)

	if arrivedCalls != 0 {
		t.Fatalf("arrived hook calls after CancelMove = %d, want 0", arrivedCalls)
	}
	if mover.Moving() {
		t.Fatal("Moving() = true after CancelMove, want false")
	}
	if got := mover.Position(); got != origin {
		t.Fatalf("Position() = %+v after CancelMove, want %+v", got, origin)
	}
}

func TestCreatureMove_OffensiveFollowTickSchedulesArrival(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	mover.StartOffensiveFollow(9, 40)
	outside := TargetSnapshot{ObjectID: 9, Known: true, Position: location.Location{X: 200, Y: 0}, CollisionRadius: 10}
	event, moved, err := mover.FollowTick(outside, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick() moved = false, want true")
	}

	clock.fire(event.Duration)

	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls = %d, want 1", arrivedCalls)
	}
	if got := mover.Position(); got != (location.Location{X: 200, Y: 0, Z: 0}) {
		t.Fatalf("Position() = %+v, want %+v", got, location.Location{X: 200, Y: 0, Z: 0})
	}
}

// staticGeo is a zero-allocation Geo stub for allocation-ceiling tests:
// recordingGeo's call-log slices grow and occasionally reallocate, which
// would add noise to a per-call allocation measurement.
type staticGeo struct {
	canMove bool
	height  int16
}

func (g staticGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return g.canMove }

func (g staticGeo) Height(x, y, z int) int16 { return g.height }

// noAllocTimer is a zero-size scheduledTimer: converting a zero-width value
// to an interface does not allocate, so installing it as afterFunc isolates
// FollowTick's own allocation profile from the real runtime timer's.
type noAllocTimer struct{}

func (noAllocTimer) Stop() bool { return true }

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
