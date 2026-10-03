package boat

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBoatShownOnEntry pins what a player entering the world beside a
// docked boat is shown of it: VehicleInfo where the boat stands, facing its
// itinerary's heading, and nothing more while it stands still.
func TestBoatShownOnEntry(t *testing.T) {
	t.Parallel()
	itinerary, _ := runeRoundTrip()
	srv, frames := bootAt(t, runeDock, itinerary)
	b := srv.Boats.Boats()[0].ObjectID()
	assertLog(t, "entry", describeBoatFrames(frames), info(b, runeDock, 40785))
}

// TestSailingBoatShownWithDeparture pins what a player coming to know a
// sailing boat is shown: VehicleInfo where the boat stands now, facing the
// point it heads for, then right after it VehicleDeparture with that point
// and the speeds of the leg.
func TestSailingBoatShownWithDeparture(t *testing.T) {
	t.Parallel()
	itinerary, pts := runeRoundTrip()
	n0 := pts[0]
	srv, _ := bootAt(t, runeDock, itinerary)
	b := srv.Boats.Boats()[0].ObjectID()
	// The boat leaves at 303 and makes 200 units of its first leg, due west,
	// by 304.
	sail(srv, 304)

	frames := enterSecond(t, srv, runeDock)
	assertLog(t, "second player's entry", describeBoatFrames(frames),
		info(b, offset(runeDock, -200, 0), 32768), depart(b, 200, 800, n0))
}

// TestBoatLeavesAndReentersSight pins the known-list side of a boat's
// sailing: a player it sails out of sight of is told to delete it, while
// still hearing its schedule from the dock; when it sails back into sight
// the player is shown it with its leg under way.
func TestBoatLeavesAndReentersSight(t *testing.T) {
	t.Parallel()
	itinerary, pts := runeRoundTrip()
	n0, n1, m0 := pts[0], pts[1], pts[3]
	// One grid region east of the dock's: the boat is in sight at the dock
	// and on its first leg, out of sight once it passes x 32768.
	at := location.Location{X: 35000, Y: runeDock.Y, Z: runeDock.Z}
	srv, _ := bootAt(t, at, itinerary)
	c := srv.Client
	b := srv.Boats.Boats()[0].ObjectID()
	clk := &clock{srv: srv}
	step := clk.stepper(t, c)

	clk.to(t, 302)
	boatLog(t, c) // the departure schedule, heard from the dock
	step(303, say(1992), move(b, n0, runeDock), depart(b, 200, 800, n0), started(b, 1), sound(arrivalDeparture, b, runeDock))
	// Past x 32768 at 311.1, 31 updates into the second leg.
	step(313, move(b, n1, n0), depart(b, 200, 800, n1), deleted(b))
	// Out of sight: the arrival line, heard from the dock, but no stop.
	step(317, say(1988), sound(arrivalDeparture, b, runeDock))
	clk.to(t, 598)
	boatLog(t, c)
	// Leaving Primeval: the line and the sound, heard from the dock, but no
	// move while out of sight; back in sight at 601.2, 22 updates of 18 units
	// from x 32381, heading due east.
	step(599, say(1990), sound(arrivalDeparture, b, runeDock))
	step(602, info(b, location.Location{X: 32777, Y: m0.Y, Z: m0.Z}, 0), depart(b, 180, 800, m0))
}

// enterSecond brings a second character into the world at at and returns
// every frame its client reads until the server is quiet.
func enterSecond(t *testing.T, srv *gameservertest.Server, at location.Location) [][]byte {
	t.Helper()
	ch := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", at.X, at.Y, at.Z, ch.ID); err != nil {
		t.Fatalf("place second character: %v", err)
	}
	c := srv.DialClient(t, "player2", 1)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	return readUntilQuiet(c)
}
