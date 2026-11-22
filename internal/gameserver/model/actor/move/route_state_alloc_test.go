//go:build !race

package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// A position update of a walk already in flight allocates nothing.
func TestInFlightPositionUpdateAllocatesNothing(t *testing.T) {
	mover, err := NewCreatureMove(location.Location{}, 120, staticGeo{canMove: true}, WithQueue(newMoveClock().q))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mover.MoveToLocation(location.Location{X: 60000}); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(50, func() {
		if _, moving := mover.UpdatePosition(PositionUpdateInterval); !moving {
			panic("walk ended early")
		}
	})
	if allocs != 0 {
		t.Fatalf("position update allocations = %v, want 0", allocs)
	}
}
