package boat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestForgedFarStepOffRefused pins the reach of a step off: a passenger
// asking to step off far from its boat is refused with ActionFailed and
// stays aboard where it was, whether the boat is tied up or under way; a
// step off beside the boat still goes through.
func TestForgedFarStepOffRefused(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 1)
	board(t, srv, c, b)
	// A ground click on the deck ends the boarding walk, which lets it step
	// off.
	c.Send(encodeMoveBackward(runeShore, runeDock))
	passengerLog(t, srv, c)

	far := location.Location{X: runeDock.X + 50000, Y: runeDock.Y + 50000, Z: -3610}
	c.Send(encodeGetOffVehicle(b, far))
	assertLog(t, "far step off, tied up", passengerLog(t, srv, c), af)
	if x, y, z := srv.PlayerPosition(t, me); (location.Location{X: x, Y: y, Z: z}) != runeDock {
		t.Fatalf("after a far step off the passenger is at %d,%d,%d, want aboard at %v", x, y, z, runeDock)
	}

	// Under way: 500 off the side is still too far once the boarding ratio
	// stretches it to 600 and more.
	sail(srv, 305)
	passengerLog(t, srv, c)
	x, y, z := srv.Boats.Boats()[0].Position()
	c.Send(encodeGetOffVehicle(b, location.Location{X: x, Y: y - 501, Z: z}))
	rest, _ := withoutChecks(passengerLog(t, srv, c))
	assertLog(t, "far step off, under way", rest, af)
	if px, py, _ := srv.PlayerPosition(t, me); px != x || py != y {
		t.Fatalf("after a far step off at sea the passenger is at %d,%d, want aboard at %d,%d", px, py, x, y)
	}

	// The boat sails on carrying it.
	sail(srv, 1)
	if _, checks := withoutChecks(passengerLog(t, srv, c)); len(checks) != 10 {
		t.Fatalf("still aboard: %d checks over a second, want 10", len(checks))
	}

	// Beside the boat, the step off goes through.
	x, y, z = srv.Boats.Boats()[0].Position()
	near := location.Location{X: x, Y: y - 300, Z: z}
	c.Send(encodeGetOffVehicle(b, near))
	log := passengerLog(t, srv, c)
	want := []string{sm(serverpackets.SystemMessageExitPeacefulZone), deckStop(me, b, location.Location{}, 0), getOff(me, b, near)}
	if len(log) < len(want) {
		t.Fatalf("near step off: %q", log)
	}
	assertLog(t, "near step off", log[:len(want)], want...)
	if px, py, _ := srv.PlayerPosition(t, me); px != x || py != y-360 {
		t.Fatalf("stepped off to %d,%d, want %d,%d", px, py, x, y-360)
	}
}

// TestBoardingBoatUnderWayRefused pins boarding a boat that has left its
// dock: even with leave to board, taken from a deck click on a made-up
// boat, the request is refused with ActionFailed and the player stays
// ashore, so no one boards after the fare is taken.
func TestBoardingBoatUnderWayRefused(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 0)
	sail(srv, 304)
	passengerLog(t, srv, c)

	c.Send(encodeMoveInVehicle(b+1000, deckSpot, deckCenter))
	assertLog(t, "deck click on a made-up boat", passengerLog(t, srv, c), af)
	c.Send(encodeGetOnVehicle(b, deckSpot))
	assertLog(t, "boarding under way", passengerLog(t, srv, c), af)
	if x, y, z := srv.PlayerPosition(t, me); (location.Location{X: x, Y: y, Z: z}) != runeBoarding {
		t.Fatalf("player at %d,%d,%d, want still ashore at %v", x, y, z, runeBoarding)
	}

	sail(srv, 1)
	if _, checks := withoutChecks(passengerLog(t, srv, c)); len(checks) != 0 {
		t.Fatalf("carried by a boat it never boarded: %q", checks)
	}
}
