package move

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// newZoneSwimPlayer is a player's mover: it steps by the player rules and
// swims only while its zones say so (WaterZone.onEnter/onExit add and remove
// MoveType.SWIM; CreatureMove.getMoveType reads the flags).
func newZoneSwimPlayer(t *testing.T, water func(location.Location) (int, bool)) *CreatureMove {
	t.Helper()
	mover, _ := newTestMover(t, staticGeo{canMove: true})
	mover.SetWaterSurface(water)
	mover.UseZoneSwim()
	mover.SetSpeeds(100, 100)
	return mover
}

// A player standing in a water zone its zones have not entered yet does not
// swim, and one its zones hold in water swims wherever it stands: the move
// type comes from the SWIM flag, not from where the player is
// (CreatureMove.java:76-85).
func TestZoneSwimPlayerMoveTypeFollowsSwimFlag(t *testing.T) {
	mover := newZoneSwimPlayer(t, everywhereWater)
	if got := mover.MoveType(); got != MoveGround {
		t.Fatalf("in water before its zones enter it: MoveType() = %d, want MoveGround", got)
	}
	mover.SetSwimming(true)
	if got := mover.MoveType(); got != MoveSwim {
		t.Fatalf("zones hold it in water: MoveType() = %d, want MoveSwim", got)
	}

	dry := newZoneSwimPlayer(t, nil)
	dry.SetFlying(true)
	dry.SetSwimming(true)
	if got := dry.MoveType(); got != MoveSwim {
		t.Fatalf("flying, zones hold it in water: MoveType() = %d, want MoveSwim (SWIM outranks FLY)", got)
	}
	dry.SetSwimming(false)
	if got := dry.MoveType(); got != MoveFly {
		t.Fatalf("flying, zones left the water: MoveType() = %d, want MoveFly", got)
	}
}

// A flying player over a water zone whose floor lies deeper than 20 below
// the surface is capped at the surface while its zones have not added SWIM
// yet (PlayerMove.java:224-257: canBypassZCheck holds for FLY only, and the
// cap reads the water zone at the current cell). Once its zones add SWIM it
// swims, and the surface no longer holds it.
func TestFlyingPlayerCappedAtSurfaceUntilZonesAddSwim(t *testing.T) {
	const surface = 50
	mover := newZoneSwimPlayer(t, func(location.Location) (int, bool) { return surface, true })
	mover.SetFlying(true)
	if got := mover.MoveType(); got != MoveFly {
		t.Fatalf("MoveType() = %d, want MoveFly", got)
	}
	if _, err := mover.MoveToLocation(location.Location{X: 30, Z: 200}); err != nil {
		t.Fatal(err)
	}
	// The 202-unit climb takes 21 updates at 10 units each; a capped flyer
	// never closes it and stays at the surface.
	for range 40 {
		mover.UpdatePosition(PositionUpdateInterval)
	}
	if got := mover.Position(); got.Z != surface || !mover.Moving() {
		t.Fatalf("flyer at %+v (moving %v), want held at height %d", got, mover.Moving(), surface)
	}

	mover.SetSwimming(true)
	ev, moving := mover.UpdatePosition(PositionUpdateInterval)
	if !moving || ev.Origin.Z <= surface {
		t.Fatalf("swimming flyer stepped to %+v (moving %v), want above the surface %d", ev.Origin, moving, surface)
	}
	for range 200 {
		if !mover.Moving() {
			break
		}
		mover.UpdatePosition(PositionUpdateInterval)
	}
	if got, want := mover.Position(), (location.Location{X: 30, Z: 200}); mover.Moving() || got != want {
		t.Fatalf("swimming flyer at %+v (moving %v), want arrived at %+v", got, mover.Moving(), want)
	}
}

// The cap needs a floor deeper than 20 below the surface
// (GeoEngine.getHeight - WaterZone.getWaterZ < -20): over shallow water a
// flyer climbs past it.
func TestFlyingPlayerOverShallowWaterIsNotCapped(t *testing.T) {
	mover, _ := newTestMover(t, staticGeo{canMove: true, height: 40})
	mover.SetWaterSurface(func(location.Location) (int, bool) { return 50, true })
	mover.UseZoneSwim()
	mover.SetSpeeds(100, 100)
	mover.SetFlying(true)
	if _, err := mover.MoveToLocation(location.Location{X: 30, Z: 200}); err != nil {
		t.Fatal(err)
	}
	for range 40 {
		mover.UpdatePosition(PositionUpdateInterval)
	}
	if got, want := mover.Position(), (location.Location{X: 30, Z: 200}); got != want {
		t.Fatalf("flyer over shallow water at %+v, want %+v", got, want)
	}
}
