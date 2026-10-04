package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// seabedGeo is permissiveGeo with its floor at height 0 everywhere.
type seabedGeo struct{ permissiveGeo }

func (seabedGeo) Height(int, int, int) int16 { return 0 }

type testVessel struct{}

func (testVessel) ObjectID() int32                 { return 7 }
func (testVessel) OustLocation() location.Location { return location.Location{} }

// Boarding tells the live movement simulation the player rides a boat: its
// own server-side steps are then held at the surface of deep water, swimming
// or not (PlayerMove.updatePosition, canBypassZCheck), until it leaves the
// boat.
func TestBoardHoldsServerStepsAtWaterSurface(t *testing.T) {
	const surface = 50
	c := liveCharacter(1, combatTemplate(), combatItems())
	live, err := creature.NewLive(location.Location{}, 100, seabedGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	c.Live = live
	mover := c.Move()
	mover.SetWaterSurface(func(location.Location) (int, bool) { return surface, true })
	mover.UseZoneSwim()
	mover.SetSwimming(true)
	mover.SetSpeeds(100, 100)

	c.Board(testVessel{})
	if _, err := mover.MoveToLocation(location.Location{X: 30, Z: 200}); err != nil {
		t.Fatal(err)
	}
	for range 40 {
		mover.UpdatePosition(move.PositionUpdateInterval)
	}
	if got := mover.Position(); got.Z != surface {
		t.Fatalf("passenger at %+v, want held at height %d", got, surface)
	}

	c.Board(nil)
	for range 200 {
		if !mover.Moving() {
			break
		}
		mover.UpdatePosition(move.PositionUpdateInterval)
	}
	if got, want := mover.Position(), (location.Location{X: 30, Z: 200}); got != want {
		t.Fatalf("after leaving the boat at %+v, want arrived at %+v", got, want)
	}
}
