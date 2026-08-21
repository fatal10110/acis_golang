package move

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type movementDynamicObject struct {
	x, y, z int
	data    [][]block.NSWE
}

func (o movementDynamicObject) GeoX() int               { return o.x }
func (o movementDynamicObject) GeoY() int               { return o.y }
func (o movementDynamicObject) GeoZ() int               { return o.z }
func (o movementDynamicObject) Height() int             { return 32 }
func (o movementDynamicObject) GeoData() [][]block.NSWE { return o.data }

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
