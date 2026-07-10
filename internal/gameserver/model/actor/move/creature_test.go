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

func TestCreatureMove_MoveToLocation(t *testing.T) {
	geo := stubGeo{canMove: true, height: 30}
	mover, err := NewCreatureMove(location.Location{X: 10, Y: 20, Z: 30}, 50, geo)
	if err != nil {
		t.Fatal(err)
	}

	event, err := mover.MoveToLocation(location.Location{X: 60, Y: 20, Z: 999})
	if err != nil {
		t.Fatal(err)
	}
	if event.Origin != (location.Location{X: 10, Y: 20, Z: 30}) || event.Destination != (location.Location{X: 60, Y: 20, Z: 30}) {
		t.Fatalf("event = %+v", event)
	}
	if event.Duration != time.Second || !mover.Moving() {
		t.Fatalf("event = %+v, moving = %v", event, mover.Moving())
	}
}

func TestCreatureMove_MoveToLocationRejectsBlockedRoute(t *testing.T) {
	mover, _ := NewCreatureMove(location.Location{}, 1, stubGeo{canMove: false})
	if _, err := mover.MoveToLocation(location.Location{X: 1}); err == nil {
		t.Fatal("MoveToLocation() error = nil")
	}
}

func TestNewCreatureMoveRejectsInvalidDependencies(t *testing.T) {
	tests := []struct {
		name  string
		speed float64
		geo   Geo
	}{
		{name: "nil geodata", speed: 1},
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

func TestCreatureMove_MoveToLocationSamePosition(t *testing.T) {
	origin := location.Location{X: 10, Y: 20, Z: 30}
	mover, err := NewCreatureMove(origin, 50, stubGeo{canMove: true, height: 30})
	if err != nil {
		t.Fatal(err)
	}

	event, err := mover.MoveToLocation(origin)
	if err != nil {
		t.Fatal(err)
	}
	if event.Duration != 0 || mover.Moving() {
		t.Fatalf("event = %+v, moving = %v", event, mover.Moving())
	}
	if mover.Destination() != origin {
		t.Fatalf("Destination() = %+v, want %+v", mover.Destination(), origin)
	}
}

func TestCreatureMove_MoveToLocationBlockedRoutePreservesState(t *testing.T) {
	geo := &stubGeo{canMove: true, height: 30}
	mover, err := NewCreatureMove(location.Location{X: 10, Y: 20, Z: 30}, 50, geo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mover.MoveToLocation(location.Location{X: 60, Y: 20}); err != nil {
		t.Fatal(err)
	}
	wantDestination := mover.Destination()
	wantMoving := mover.Moving()

	geo.canMove = false
	if _, err := mover.MoveToLocation(location.Location{X: 70, Y: 20}); err == nil {
		t.Fatal("MoveToLocation() error = nil")
	}
	if got := mover.Destination(); got != wantDestination {
		t.Fatalf("Destination() = %+v, want %+v", got, wantDestination)
	}
	if got := mover.Moving(); got != wantMoving {
		t.Fatalf("Moving() = %v, want %v", got, wantMoving)
	}
}
