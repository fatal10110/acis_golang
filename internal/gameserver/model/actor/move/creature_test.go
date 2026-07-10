package move

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type stubGeo struct {
	canMove bool
	height  int16
}

func (g stubGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool {
	return g.canMove
}

func (g stubGeo) Height(x, y, z int) int16 {
	return g.height
}

type nilMapGeo map[int]struct{}

func (nilMapGeo) CanMove(int, int, int, int, int, int) bool { return false }

func (nilMapGeo) Height(int, int, int) int16 { return 0 }

func TestNewCreatureMoveRejectsInvalidDependencies(t *testing.T) {
	var typedNil *stubGeo
	var typedNilMap nilMapGeo
	tests := []struct {
		name  string
		speed float64
		geo   Geo
	}{
		{name: "nil geodata", speed: 1},
		{name: "typed-nil geodata", speed: 1, geo: typedNil},
		{name: "typed-nil map geodata", speed: 1, geo: typedNilMap},
		{name: "zero speed", geo: stubGeo{}, speed: 0},
		{name: "negative speed", geo: stubGeo{}, speed: -1},
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
	tests := []struct {
		name              string
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
			geo := &stubGeo{canMove: test.canMove, height: 30}
			mover, err := NewCreatureMove(origin, 50, geo)
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
