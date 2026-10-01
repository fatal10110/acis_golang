package move

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// slopeGeo is open geodata whose floor rises one unit per unit east.
type slopeGeo struct{ staticGeo }

func (slopeGeo) Height(x, _, _ int) int16 { return int16(x) }

// A player's zones adding SWIM mid-leg re-time the arrival for the distance
// left under the new move type: a walk timed by its 2D distance becomes a
// swim measured in 3D (CreatureMove.getMoveType reads the SWIM flag the
// water zone sets, and a swimmer travels the 3D line), so a leg with height
// left does not arrive at the stale 2D time.
func TestZoneSwimPlayerSetSwimmingRetimesInFlightArrival(t *testing.T) {
	mover, clock := newTestMover(t, slopeGeo{staticGeo{canMove: true}})
	mover.SetWaterSurface(everywhereWater)
	mover.UseZoneSwim()
	mover.SetSpeeds(100, 100)
	arrived := 0
	mover.setOwner(&hookOwner{onArrived: func() { arrived++ }})

	ev, err := mover.MoveToLocation(location.Location{X: 300, Z: 300})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Duration != 3*time.Second {
		t.Fatalf("walk duration = %v, want 3s for 300 units in 2D at speed 100", ev.Duration)
	}
	clock.in.Advance(time.Second)
	mover.UpdatePosition(time.Second)
	at := mover.Position()
	if at.X != 100 {
		t.Fatalf("after 1s at %+v, want X 100", at)
	}

	mover.SetSwimming(true)
	if got := mover.MoveType(); got != MoveSwim {
		t.Fatalf("MoveType() = %d, want MoveSwim", got)
	}
	clock.in.Advance(2 * time.Second) // the stale 2D arrival time
	if arrived != 0 {
		t.Fatal("swim arrived at the walk's 2D arrival time")
	}
	// The rest is the 3D line from at to the destination, 10 units an update.
	left := math.Hypot(float64(300-at.X), float64(300-at.Z))
	updates := int(math.Ceil(left / 10))
	if updates <= 20 {
		t.Fatalf("%.0f units left in 3D from %+v, want more than the 200 left in 2D", left, at)
	}
	clock.in.Advance(time.Duration(updates)*PositionUpdateInterval - 2*time.Second - PositionUpdateInterval)
	if arrived != 0 {
		t.Fatalf("swim arrived one update before covering %.0f units in 3D", left)
	}
	clock.in.Advance(PositionUpdateInterval)
	if arrived != 1 {
		t.Fatalf("arrived hook calls = %d after %v swimming %.0f units in 3D, want 1", arrived, time.Duration(updates)*PositionUpdateInterval, left)
	}
}
