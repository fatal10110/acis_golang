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
		{name: "zero speed", geo: &recordingGeo{}, speed: 0},
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
