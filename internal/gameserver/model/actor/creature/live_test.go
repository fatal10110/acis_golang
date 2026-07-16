package creature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type liveGeo struct {
	canMove bool
	height  int16
}

func (g liveGeo) CanMove(_, _, _, _, _, _ int) bool { return g.canMove }
func (g liveGeo) Height(_, _, _ int) int16          { return g.height }

func TestLiveOwnsOneMovementState(t *testing.T) {
	live, err := NewLive(location.Location{X: 10, Y: 20, Z: 30}, 50, liveGeo{canMove: true, height: 30})
	if err != nil {
		t.Fatal(err)
	}

	first := live.Move()
	if first != &live.movement {
		t.Fatal("Move() does not return the embedded movement state")
	}

	if _, err := first.MoveToLocation(location.Location{X: 60, Y: 20, Z: 999}); err != nil {
		t.Fatal(err)
	}
	second := live.Move()
	if second != first {
		t.Fatal("Move() returned a different movement state")
	}
	if got := second.Destination(); got != (location.Location{X: 60, Y: 20, Z: 30}) {
		t.Fatalf("Destination() = %+v, want the accepted target", got)
	}

	if _, err := second.MoveToLocation(location.Location{X: 70, Y: 20, Z: 999}); err != nil {
		t.Fatal(err)
	}
	if live.Move() != first {
		t.Fatal("repeated movement replaced the embedded movement state")
	}
	if got := first.Destination(); got != (location.Location{X: 70, Y: 20, Z: 30}) {
		t.Fatalf("Destination() = %+v, want the second accepted target", got)
	}
}

func TestLiveMovementStateIsPerCreature(t *testing.T) {
	geo := liveGeo{canMove: true, height: 30}
	first, err := NewLive(location.Location{X: 0, Y: 0, Z: 30}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewLive(location.Location{X: 100, Y: 0, Z: 30}, 100, geo)
	if err != nil {
		t.Fatal(err)
	}

	if first.Move() == second.Move() {
		t.Fatal("two live creatures share movement state")
	}
	if _, err := first.Move().MoveToLocation(location.Location{X: 50, Y: 0, Z: 999}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Move().MoveToLocation(location.Location{X: 150, Y: 0, Z: 999}); err != nil {
		t.Fatal(err)
	}

	if got := first.Move().Destination(); got != (location.Location{X: 50, Y: 0, Z: 30}) {
		t.Fatalf("first Destination() = %+v, want its own target", got)
	}
	if got := second.Move().Destination(); got != (location.Location{X: 150, Y: 0, Z: 30}) {
		t.Fatalf("second Destination() = %+v, want its own target", got)
	}
}
