package move

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

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
