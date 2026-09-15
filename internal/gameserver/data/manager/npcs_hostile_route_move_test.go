package manager

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type recordingMoveController struct {
	ai.MoveController
	moved []location.Location
}

func (m *recordingMoveController) MoveToLocation(dest location.Location) (bool, error) {
	m.moved = append(m.moved, dest)
	return true, nil
}

// A random-walk or escort move must not inherit the route flag left by the
// last walker leg, or its arrival would advance the patrol route.
func TestRouteAwareMoveControllerMoveToLocationClearsRouteMove(t *testing.T) {
	var routeMove atomic.Bool
	routeMove.Store(true)
	inner := &recordingMoveController{}
	controller := routeAwareMoveController{MoveController: inner, routeMove: &routeMove}

	dest := location.Location{X: 10, Y: 20, Z: 30}
	if ok, err := controller.MoveToLocation(dest); !ok || err != nil {
		t.Fatalf("MoveToLocation() = %v, %v; want true, nil", ok, err)
	}
	if routeMove.Load() {
		t.Fatal("routeMove = true after MoveToLocation, want false")
	}
	if len(inner.moved) != 1 || inner.moved[0] != dest {
		t.Fatalf("inner moves = %v, want [%v]", inner.moved, dest)
	}
}
