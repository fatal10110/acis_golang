package move

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

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

	m.SetQueue(newMoveClock().q)

	if _, err := m.MoveToLocation(location.Location{X: 100, Y: 20, Z: 30}); err == nil {
		t.Fatal("MoveToLocation() error = nil, want error for zero-speed actor")
	}
}

// TestCreatureMoveValidLocationWithoutGeoReturnsDestination pins the
// fallback for a movement never initialized with geodata (a summon spawned
// while the server runs without geodata): the destination comes back
// unchanged, as the reference's getValidLocation does over a region with no
// geodata, and the call does not panic.
func TestCreatureMoveValidLocationWithoutGeoReturnsDestination(t *testing.T) {
	var m CreatureMove

	got := m.ValidLocation(1, 2, 3, 10, 20, 30)

	if want := (location.Location{X: 10, Y: 20, Z: 30}); got != want {
		t.Fatalf("ValidLocation() = %+v, want destination %+v", got, want)
	}
}

// TestCreatureMoveValidLocationDelegatesToGeo pins that an initialized
// movement resolves the destination through its own geodata.
func TestCreatureMoveValidLocationDelegatesToGeo(t *testing.T) {
	geo := &recordingGeo{validLocation: location.Location{X: 7, Y: 8, Z: 9}}
	m, err := NewCreatureMove(location.Location{X: 1, Y: 2, Z: 3}, 100, geo)
	if err != nil {
		t.Fatalf("NewCreatureMove() error = %v", err)
	}

	got := m.ValidLocation(1, 2, 3, 10, 20, 30)

	if want := (location.Location{X: 7, Y: 8, Z: 9}); got != want {
		t.Fatalf("ValidLocation() = %+v, want geodata answer %+v", got, want)
	}
	wantCall := validLocationCall{
		origin: location.Location{X: 1, Y: 2, Z: 3},
		target: location.Location{X: 10, Y: 20, Z: 30},
	}
	if len(geo.validLocationCalls) != 1 || geo.validLocationCalls[0] != wantCall {
		t.Fatalf("ValidLocation calls = %+v, want [%+v]", geo.validLocationCalls, wantCall)
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
		wantEvent         event.Move
		wantErr           bool
		wantDestination   location.Location
		wantMoving        bool
	}{
		{
			name:            "normalizes height and uses Java tick duration",
			canMove:         true,
			target:          location.Location{X: 60, Y: 20, Z: 999},
			wantEvent:       event.Move{Origin: origin, Destination: previous, Speed: 50, Duration: time.Second},
			wantDestination: previous,
			wantMoving:      true,
		},
		{
			name:            "rounds one unit up to one tick",
			canMove:         true,
			target:          location.Location{X: 11, Y: 20, Z: 999},
			wantEvent:       event.Move{Origin: origin, Destination: location.Location{X: 11, Y: 20, Z: 30}, Speed: 50, Duration: 100 * time.Millisecond},
			wantDestination: location.Location{X: 11, Y: 20, Z: 30},
			wantMoving:      true,
		},
		{
			name:            "rounds fifty-one units up to eleven ticks",
			canMove:         true,
			target:          location.Location{X: 61, Y: 20, Z: 999},
			wantEvent:       event.Move{Origin: origin, Destination: location.Location{X: 61, Y: 20, Z: 30}, Speed: 50, Duration: 1100 * time.Millisecond},
			wantDestination: location.Location{X: 61, Y: 20, Z: 30},
			wantMoving:      true,
		},
		{
			name:            "accepts blocked route as zero-distance arrival",
			target:          location.Location{X: 60, Y: 20},
			wantEvent:       event.Move{Origin: origin, Destination: origin, Speed: 50},
			wantDestination: origin,
			wantMoving:      true,
		},
		{
			name:            "same position has zero duration",
			canMove:         true,
			target:          origin,
			wantEvent:       event.Move{Origin: origin, Destination: origin, Speed: 50},
			wantDestination: origin,
			wantMoving:      true,
		},
		{
			name:            "same position accepts the smallest finite speed",
			speed:           math.SmallestNonzeroFloat64,
			canMove:         true,
			target:          location.Location{X: origin.X, Y: origin.Y, Z: 999},
			wantEvent:       event.Move{Origin: origin, Destination: origin, Speed: math.SmallestNonzeroFloat64},
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
			wantEvent:         event.Move{Origin: origin, Destination: origin, Speed: 50},
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
			mover.SetQueue(newMoveClock().q)
			if test.initialTarget != nil {
				if _, err := mover.MoveToLocation(*test.initialTarget); err != nil {
					t.Fatal(err)
				}
			}
			if test.blockAfterInitial {
				geo.canMove = false
			}

			ev, err := mover.MoveToLocation(test.target)
			if (err != nil) != test.wantErr {
				t.Fatalf("MoveToLocation() error = %v, want error = %v", err, test.wantErr)
			}
			if !test.wantErr && ev != test.wantEvent {
				t.Fatalf("event = %+v, want %+v", ev, test.wantEvent)
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
	mover.SetQueue(newMoveClock().q)

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
	mover.SetQueue(newMoveClock().q)
	arrived := 0
	blocked := 0
	mover.setOwner(&hookOwner{onArrived: func() { arrived++ }, onBlocked: func() { blocked++ }})
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
	mover.SetQueue(newMoveClock().q)
	arrived := 0
	blocked := 0
	mover.setOwner(&hookOwner{onArrived: func() { arrived++ }, onBlocked: func() { blocked++ }})
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
	mover.SetQueue(newMoveClock().q)
	arrived := 0
	blocked := 0
	mover.setOwner(&hookOwner{onArrived: func() { arrived++ }, onBlocked: func() { blocked++ }})
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

func TestCreatureMove_UpdatePositionAdvancesNextWaypointWhenObstacleClosesMidRoute(t *testing.T) {
	allow := false
	waypoints := []location.Location{
		{X: 100, Y: 0, Z: 30},
		{X: 100, Y: 100, Z: 30},
	}
	geo := &recordingGeo{
		height:     30,
		findPath:   waypoints,
		findPathOK: true,
		canMoveAt:  func(int, int, int, int, int, int) bool { return allow },
	}
	mover, err := NewCreatureMove(location.Location{Z: 30}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	arrived := 0
	blocked := 0
	advanced := 0
	var advancedEvent event.Move
	mover.setOwner(&hookOwner{onArrived: func() { arrived++ }, onBlocked: func() { blocked++ }, onAdvanced: func(ev event.Move) {
		advanced++
		advancedEvent = ev
	}})
	if _, err := mover.MoveToLocation(location.Location{X: 100, Y: 100, Z: 30}); err != nil {
		t.Fatal(err)
	}
	if got := mover.Destination(); got != waypoints[0] {
		t.Fatalf("Destination() = %+v, want first waypoint %+v", got, waypoints[0])
	}

	allow = true
	if _, moving := mover.UpdatePosition(PositionUpdateInterval); !moving {
		t.Fatal("first UpdatePosition() stopped move, want moving")
	}
	if _, moving := mover.UpdatePosition(PositionUpdateInterval); !moving {
		t.Fatal("second UpdatePosition() stopped move, want still on first leg")
	}
	blockedCell := mover.Position()
	if blockedCell == (location.Location{Z: 30}) {
		t.Fatal("Position() still at origin after interpolation ticks")
	}

	allow = false
	ev, moving := mover.UpdatePosition(PositionUpdateInterval)
	if !moving {
		t.Fatal("UpdatePosition() moving = false after mid-route obstacle, want next-leg walk")
	}
	if got := mover.Position(); got != blockedCell {
		t.Fatalf("Position() = %+v, want blocked cell %+v (no snap to closed dest)", got, blockedCell)
	}
	if got := mover.Destination(); got != waypoints[1] {
		t.Fatalf("Destination() = %+v, want remaining waypoint %+v", got, waypoints[1])
	}
	if ev.Destination != waypoints[1] {
		t.Fatalf("event.Destination = %+v, want remaining waypoint %+v", ev.Destination, waypoints[1])
	}
	if arrived != 0 || blocked != 0 {
		t.Fatalf("arrival callbacks = (%d, %d), want (0, 0) while remaining waypoints exist", arrived, blocked)
	}
	if advanced != 1 {
		t.Fatalf("segment-advanced hook calls = %d, want 1", advanced)
	}
	if advancedEvent.Destination != waypoints[1] {
		t.Fatalf("segment-advanced dest = %+v, want %+v", advancedEvent.Destination, waypoints[1])
	}

	allow = true
	for range 40 {
		if _, still := mover.UpdatePosition(PositionUpdateInterval); !still {
			break
		}
	}
	if mover.Moving() {
		t.Fatal("UpdatePosition() still moving after remaining waypoint")
	}
	if got := mover.Position(); got != waypoints[1] {
		t.Fatalf("Position() = %+v, want final waypoint %+v", got, waypoints[1])
	}
	if arrived != 0 || blocked != 1 {
		t.Fatalf("final arrival callbacks = (%d, %d), want (0, 1) after a blocked tick on this route", arrived, blocked)
	}
}

func startBlockedMidRouteMove(t *testing.T) (mover *CreatureMove, allow *bool, arrived, blocked *int) {
	t.Helper()
	on := false
	allow = &on
	waypoints := []location.Location{
		{X: 100, Y: 0, Z: 30},
		{X: 100, Y: 100, Z: 30},
	}
	geo := &recordingGeo{
		height:     30,
		findPath:   waypoints,
		findPathOK: true,
		canMoveAt:  func(int, int, int, int, int, int) bool { return *allow },
	}
	var err error
	mover, err = NewCreatureMove(location.Location{Z: 30}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	mover.SetQueue(newMoveClock().q)
	arrivedN, blockedN := 0, 0
	arrived, blocked = &arrivedN, &blockedN
	mover.setOwner(&hookOwner{onArrived: func() { arrivedN++ }, onBlocked: func() { blockedN++ }})
	if _, err := mover.MoveToLocation(location.Location{X: 100, Y: 100, Z: 30}); err != nil {
		t.Fatal(err)
	}
	*allow = true
	if _, moving := mover.UpdatePosition(PositionUpdateInterval); !moving {
		t.Fatal("first UpdatePosition() stopped move, want moving")
	}
	if _, moving := mover.UpdatePosition(PositionUpdateInterval); !moving {
		t.Fatal("second UpdatePosition() stopped move, want still on first leg")
	}
	*allow = false
	if _, moving := mover.UpdatePosition(PositionUpdateInterval); !moving {
		t.Fatal("UpdatePosition() moving = false after mid-route obstacle, want next-leg walk")
	}
	if arrivedN != 0 || blockedN != 0 {
		t.Fatalf("arrival callbacks = (%d, %d), want (0, 0) while remaining waypoints exist", arrivedN, blockedN)
	}
	return mover, allow, arrived, blocked
}

func TestCreatureMove_RetargetWhileMovingKeepsBlockedArrival(t *testing.T) {
	mover, allow, arrived, blocked := startBlockedMidRouteMove(t)
	*allow = true
	if _, err := mover.MoveToLocation(location.Location{X: 50, Y: 0, Z: 30}); err != nil {
		t.Fatal(err)
	}
	if !mover.Moving() {
		t.Fatal("MoveToLocation() while moving stopped the in-flight walk")
	}
	for range 40 {
		if _, still := mover.UpdatePosition(PositionUpdateInterval); !still {
			break
		}
	}
	if mover.Moving() {
		t.Fatal("UpdatePosition() still moving after retarget")
	}
	if *arrived != 0 || *blocked != 1 {
		t.Fatalf("arrival callbacks = (%d, %d), want (0, 1) after retarget of a previously blocked route", *arrived, *blocked)
	}
}

func TestCreatureMove_RejectedRetargetDoesNotClearBlockedFlag(t *testing.T) {
	mover, allow, arrived, blocked := startBlockedMidRouteMove(t)
	mover.SetSpeed(0)
	if _, err := mover.MoveToLocation(location.Location{X: 200, Y: 0, Z: 30}); err == nil {
		t.Fatal("MoveToLocation() error = nil at zero speed")
	}
	if !mover.Moving() {
		t.Fatal("rejected MoveToLocation() cancelled the in-flight walk")
	}
	mover.SetSpeed(100)
	*allow = true
	for range 40 {
		if _, still := mover.UpdatePosition(PositionUpdateInterval); !still {
			break
		}
	}
	if *arrived != 0 || *blocked != 1 {
		t.Fatalf("arrival callbacks = (%d, %d), want (0, 1) after a rejected retarget on a blocked route", *arrived, *blocked)
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
	mover.SetQueue(newMoveClock().q)
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
	mover.SetQueue(newMoveClock().q)
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
	mover.SetQueue(newMoveClock().q)

	if _, err := mover.MoveToLocation(current); err != nil {
		t.Fatal(err)
	}
	mover.SetPosition(current)

	ev, err := mover.MoveToLocation(location.Location{X: 70, Y: 20, Z: 999})
	if err != nil {
		t.Fatal(err)
	}

	want := event.Move{
		Origin:      current,
		Destination: location.Location{X: 70, Y: 20, Z: 30},
		Speed:       50,
		Duration:    200 * time.Millisecond,
	}
	if ev != want {
		t.Fatalf("MoveToLocation() event = %+v, want %+v", ev, want)
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
	mover.SetQueue(newMoveClock().q)

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
	clock := newMoveClock()
	mover.SetQueue(clock.q)
	arrivedCalls := 0
	mover.setOwner(&hookOwner{onArrived: func() { arrivedCalls++ }})

	ev, err := mover.MoveToLocation(location.Location{X: 100, Y: 50, Z: 30})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v, want nil", err)
	}
	// The active destination is the first geopath entry; the remaining two
	// are queued inside CreatureMove.
	wantFirst := location.Location{X: 50, Y: 0, Z: 30}
	if ev.Destination != wantFirst {
		t.Fatalf("event.Destination = %+v, want %+v (first waypoint)", ev.Destination, wantFirst)
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
		clock.in.Advance(ev.Duration)
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
	clock := newMoveClock()
	mover.SetQueue(clock.q)
	arrivedCalls := 0
	mover.setOwner(&hookOwner{onArrived: func() { arrivedCalls++ }})

	ev, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 30})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v, want nil (tier 3 fall-back)", err)
	}
	if ev.Destination != fallback {
		t.Fatalf("event.Destination = %+v, want fall-back %+v", ev.Destination, fallback)
	}
	if got := mover.Destination(); got != fallback {
		t.Fatalf("Destination() = %+v, want %+v", got, fallback)
	}
	if got := len(geo.validLocationCalls); got != 1 {
		t.Fatalf("ValidLocation() calls = %d, want 1", got)
	}

	clock.in.Advance(ev.Duration)
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
	mover.SetQueue(newMoveClock().q)
	// Seed an in-flight destination so the new request must replace it.
	mover.destination = prior
	mover.moving = true

	ev, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 30})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v, want nil", err)
	}
	if want := (event.Move{Origin: origin, Destination: origin, Speed: 50}); ev != want {
		t.Fatalf("MoveToLocation() event = %+v, want %+v", ev, want)
	}
	if got := mover.Destination(); got != origin {
		t.Fatalf("Destination() = %+v, want origin %+v", got, origin)
	}
	if !mover.Moving() {
		t.Fatal("Moving() = false, want zero-distance arrival pending")
	}
}
