package boat

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// clock tracks how many seconds a test has sailed its fleet.
type clock struct {
	srv *gameservertest.Server
	now int
}

// stepper returns a step that sails up to second s and asserts the boat
// frames c received meanwhile are want.
func (c *clock) stepper(t *testing.T, client *testsupport.ScriptedClient) func(s int, want ...string) {
	return func(s int, want ...string) {
		t.Helper()
		c.to(t, s)
		assertLog(t, fmt.Sprint("second ", s), boatLog(t, client), want...)
	}
}

// to sails the fleet up to second s.
func (c *clock) to(t *testing.T, s int) {
	t.Helper()
	if s < c.now {
		t.Fatalf("sail back to %d from %d", s, c.now)
	}
	sail(c.srv, s-c.now)
	c.now = s
}

// runeRoundTrip is a short Rune-Primeval round trip sailed within sight of
// the Rune dock. Leaving Rune it announces 1001 (5 minutes), 1002 (1
// minute), 1003 (20 seconds) and 1004; leaving Primeval it lists 2003
// under 2002's delay, which ends its schedule before 2004.
func runeRoundTrip() (route.BoatItinerary, []location.Location) {
	n0 := offset(runeDock, -1000, 0)
	n1 := offset(runeDock, -2000, 0)
	n2 := offset(runeDock, -2000, 1000)
	m0 := offset(runeDock, -1000, 1000)
	out0 := node(n0, 200, 800)
	out0.DepartureMessages = []int{1992}
	out2 := node(n2, 250, 1000)
	out2.BusyMessage = 1994
	out2.ArrivalMessages = []int{1988}
	out2.Scheduled = []route.ScheduledMessage{{ID: 1001, Delay: 240}, {ID: 1002, Delay: 40}, {ID: 1003, Delay: 20}, {ID: 1004, Delay: 0}}
	back0 := node(m0, 180, 800)
	back0.DepartureMessages = []int{1990}
	back1 := node(runeDock, 220, 800)
	back1.BusyMessage = 1993
	back1.ArrivalMessages = []int{1620}
	back1.Scheduled = []route.ScheduledMessage{{ID: 2001, Delay: 240}, {ID: 2002, Delay: 40}, {ID: 2003, Delay: 40}, {ID: 2004, Delay: 20}}
	return route.BoatItinerary{Heading: 40785, Routes: []route.BoatRoute{
		{Dock: route.DockRune, ItemID: 8925, Nodes: []route.BoatLocation{out0, node(n1, 200, 800), out2}},
		{Dock: route.DockPrimeval, ItemID: 8924, Nodes: []route.BoatLocation{back0, back1}},
	}}, []location.Location{n0, n1, n2, m0}
}

// TestBoatRoundTrip follows one boat through a full round trip, as a player
// standing on its first dock sees and hears it: the departure schedule, one
// step per scheduled delay with its sound; the departure; each leg of the
// route; the wait off shore and the last leg to the dock; the stop; the
// tie-up; then the schedule of the way back, cut short where it repeats a
// delay, the way back, and the schedule of the first dock again.
func TestBoatRoundTrip(t *testing.T) {
	t.Parallel()
	itinerary, pts := runeRoundTrip()
	n0, n1, n2, m0 := pts[0], pts[1], pts[2], pts[3]
	srv, _ := bootAt(t, runeDock, itinerary)
	c := srv.Client
	b := srv.Boats.Boats()[0].ObjectID()
	step := (&clock{srv: srv}).stepper(t, c)

	// The Rune-Primeval boat waits no first delay.
	step(1, say(1001), sound(fiveMinutes, b, runeDock))
	step(240)
	step(241, say(1002), sound(oneMinute, b, runeDock))
	step(281, say(1003), sound(oneMinute, b, runeDock))
	step(301, say(1004))
	step(302)
	step(303, say(1992), move(b, n0, runeDock), depart(b, 200, 800, n0), started(b, 1), sound(arrivalDeparture, b, runeDock))
	// 1000 units at 200 a second.
	step(307)
	step(308, move(b, n1, n0), depart(b, 200, 800, n1))
	// The last node but one: the boat waits for its destination dock,
	// which Primeval, holding any number of boats, never makes it do.
	step(313, move(b, n2, n1), depart(b, 250, 1000, n2))
	step(316)
	step(317, say(1988), started(b, 0), info(b, n2, 16384), sound(arrivalDeparture, b, runeDock), info(b, n2, 16384))
	step(318, say(2001), sound(fiveMinutes, b, runeDock))
	step(558, say(2002), say(2003), sound(oneMinute, b, runeDock))
	// No 2004: the schedule ended with 2003.
	step(598)
	step(599, say(1990), move(b, m0, n2), depart(b, 180, 800, m0), started(b, 1), sound(arrivalDeparture, b, runeDock))
	// 1000 units at 180 a second end at 604.6; Rune, freed when the boat
	// left it, takes the boat at the next schedule step.
	step(605, move(b, runeDock, m0), depart(b, 220, 800, runeDock))
	// 1414 units at 220 a second.
	step(611)
	step(612, say(1620), started(b, 0), info(b, runeDock, 57344), sound(arrivalDeparture, b, runeDock), info(b, runeDock, 57344))
	step(613, say(1001), sound(fiveMinutes, b, runeDock))
}
