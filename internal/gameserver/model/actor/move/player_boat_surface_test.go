package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// A passenger's own server-side steps are held at the surface of a water
// zone whose floor lies deeper than 20 below it, swimming or not, as a
// flyer's are (PlayerMove.java:225-257: canBypassZCheck holds for a player
// whose BoatInfo.getBoat() is set). Ashore, the same swimmer climbs past the
// surface.
func TestPassengerCappedAtSurface(t *testing.T) {
	const surface = 50
	mover := newZoneSwimPlayer(t, func(location.Location) (int, bool) { return surface, true })
	mover.SetSwimming(true)
	mover.SetInBoat(true)
	if _, err := mover.MoveToLocation(location.Location{X: 30, Z: 200}); err != nil {
		t.Fatal(err)
	}
	for range 40 {
		mover.UpdatePosition(PositionUpdateInterval)
	}
	if got := mover.Position(); got.Z != surface || !mover.Moving() {
		t.Fatalf("passenger at %+v (moving %v), want held at height %d", got, mover.Moving(), surface)
	}

	mover.SetInBoat(false)
	for range 200 {
		if !mover.Moving() {
			break
		}
		mover.UpdatePosition(PositionUpdateInterval)
	}
	if got, want := mover.Position(), (location.Location{X: 30, Z: 200}); mover.Moving() || got != want {
		t.Fatalf("swimmer ashore at %+v (moving %v), want arrived at %+v", got, mover.Moving(), want)
	}
}
