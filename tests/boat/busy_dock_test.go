package boat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

// TestBoatWaitsForHeldDock pins the wait off shore: a boat whose
// destination dock another boat holds checks it every five seconds,
// announcing the wait on the first check and every 90 seconds after, and
// sails its last leg on the first check after the dock is freed. Talking
// Island starts the server held, and only the boat leaving it frees it.
func TestBoatWaitsForHeldDock(t *testing.T) {
	t.Parallel()
	// The boat tied up at Talking Island leaves 240 seconds after its first
	// delay, announcing 3001 to Talking Island alone.
	tiOut := node(tiDock, 300, 800)
	tiOut.Scheduled = []route.ScheduledMessage{{ID: 3001, Delay: 240}}
	tiBoat := route.BoatItinerary{Heading: 0, Routes: []route.BoatRoute{
		{Dock: route.DockTalkingIsland, Nodes: []route.BoatLocation{node(offset(tiDock, -1000, 0), 300, 800), tiOut}},
	}}
	// The Gludin boat leaves on its first delay with no schedule and reaches
	// its last node but one in a second.
	n0 := offset(gludinDock, -400, 0)
	n1 := offset(gludinDock, -400, 1000)
	toTI := node(n1, 250, 900)
	toTI.BusyMessage = 1485
	toGludin := node(gludinDock, 250, 900)
	toGludin.BusyMessage = 1486
	gludinBoat := route.BoatItinerary{Heading: 0, Routes: []route.BoatRoute{
		{Dock: route.DockGludin, Nodes: []route.BoatLocation{node(n0, 400, 800), toTI}},
		{Dock: route.DockTalkingIsland, Nodes: []route.BoatLocation{node(offset(gludinDock, 0, 500), 250, 900), toGludin}},
	}}

	srv, _ := bootAt(t, gludinDock, tiBoat, gludinBoat)
	b := srv.Boats.Boats()[1].ObjectID()
	step := (&clock{srv: srv}).stepper(t, srv.Client)

	// Both boats wait 300 seconds first. Talking Island's 3001 is out of
	// earshot of Gludin.
	step(300)
	step(301, move(b, n0, gludinDock), depart(b, 400, 800, n0), started(b, 1), sound(arrivalDeparture, b, gludinDock))
	// The wait is announced with the line of the route leaving the held
	// dock: 1486, not the 1485 of the route the boat sails.
	step(302, say(1486))
	step(391)
	step(392, say(1486))
	step(481)
	step(482, say(1486))
	// Talking Island's boat leaves, freeing its dock, at 541; the next check
	// is at 542.
	step(541)
	step(542, move(b, n1, n0), depart(b, 250, 900, n1))
}
