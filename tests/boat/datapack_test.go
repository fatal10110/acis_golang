package boat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestShippedInnadrilSchedule runs the shipped boatRoutes.xml fleet and
// follows the Innadril tour boat, the only one heard from its dock. Its
// route lists 983 under the 40-second delay 1000 already has, which ends its
// departure schedule there: after 999 (5 minutes) and 1000 with 983 (1
// minute) it leaves 40 seconds later, and 1001, the second 983 and 1002 are
// never announced.
func TestShippedInnadrilSchedule(t *testing.T) {
	t.Parallel()
	itineraries, err := xml.LoadBoatRoutes(datapack.Path(t, "data", "xml", "boatRoutes.xml"))
	if err != nil {
		t.Fatalf("load boat routes: %v", err)
	}
	if len(itineraries) != 5 {
		t.Fatalf("itineraries = %d, want 5", len(itineraries))
	}
	tour := itineraries[2]
	if tour.Routes[0].Dock != route.DockInnadril || len(tour.Routes) != 1 {
		t.Fatalf("itinerary 2 = %+v, want the one-way Innadril tour", tour.Routes)
	}
	first := location.Location{X: 105448, Y: 226232, Z: -3610}

	srv, frames := bootAt(t, innadrilDock, itineraries...)
	b := srv.Boats.Boats()[2].ObjectID()
	assertLog(t, "entry", describeBoatFrames(frames), info(b, innadrilDock, 32768))

	step := (&clock{srv: srv}).stepper(t, srv.Client)
	step(299)
	step(300, say(999), sound(fiveMinutes, b, innadrilDock))
	step(539)
	step(540, say(1000), say(983), sound(oneMinute, b, innadrilDock))
	step(580)
	step(581, move(b, first, innadrilDock), depart(b, 150, 800, first), started(b, 1), sound(arrivalDeparture, b, innadrilDock))
}
