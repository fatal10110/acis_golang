package boat

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestPassengerBoardsSailsAndPays follows one passenger from the shore to
// the open sea: a click on the deck from beside the boarding point walks
// the player onto the deck; boarding sets the player at the boat, at peace,
// and shows it aboard; once the boat sails each of its position updates
// carries the passenger along and tells it where the boat is; five seconds
// after the departure one ticket is taken.
func TestPassengerBoardsSailsAndPays(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 2)

	c.Send(encodeMoveInVehicle(b, deckSpot, deckCenter))
	assertLog(t, "deck click", passengerLog(t, srv, c), deckWalk(me, b, deckSpot, deckCenter), af)

	c.Send(encodeGetOnVehicle(b, deckSpot))
	assertLog(t, "boarding", passengerLog(t, srv, c), sm(serverpackets.SystemMessageEnterPeacefulZone), getOn(me, b, deckSpot))
	if x, y, z := srv.PlayerPosition(t, me); (location.Location{X: x, Y: y, Z: z}) != runeDock {
		t.Fatalf("passenger at %d,%d,%d, want the boat's %v", x, y, z, runeDock)
	}

	// Tied up, the boat carries no one anywhere.
	sail(srv, 303)
	rest, checks := withoutChecks(passengerLog(t, srv, c))
	if len(checks) != 0 {
		t.Fatalf("checks while tied up: %q", checks)
	}
	assertLog(t, "tied up", rest)

	// Under way from 303: 20 units west per update.
	sail(srv, 1)
	rest, checks = withoutChecks(passengerLog(t, srv, c))
	if len(checks) != 10 || checks[0] != check(b, runeBoatFirst, 32768) || checks[9] != check(b, offset(runeDock, -200, 0), 32768) {
		t.Fatalf("second 304 checks %q", checks)
	}
	assertLog(t, "second 304", rest)
	if x, y, z := srv.PlayerPosition(t, me); (location.Location{X: x, Y: y, Z: z}) != offset(runeDock, -200, 0) {
		t.Fatalf("passenger at %d,%d,%d, want carried to %v", x, y, z, offset(runeDock, -200, 0))
	}

	// The fare falls due at 308.0, after that second's last update.
	sail(srv, 3)
	rest, _ = withoutChecks(passengerLog(t, srv, c))
	assertLog(t, "seconds 305-307", rest)
	sail(srv, 1)
	rest, _ = withoutChecks(passengerLog(t, srv, c))
	assertLog(t, "second 308", rest, sm(serverpackets.SystemMessageS1Disappeared))
	if got := srv.PlayerInventory(t, me).ItemCount(ticketID, -1, false); got != 1 {
		t.Fatalf("tickets left %d, want 1", got)
	}
}

// TestPassengerWithoutTicketIsPutAshore pins the fare a passenger cannot
// pay: five seconds after the departure the passenger is teleported to the
// shore of the dock the boat left, leaves the boat's peace, and is told its
// ticket is not right. Off the boat, the updates no longer carry it.
func TestPassengerWithoutTicketIsPutAshore(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 0)
	board(t, srv, c, b)

	sail(srv, 307)
	passengerLog(t, srv, c)
	sail(srv, 1)
	rest, _ := withoutChecks(passengerLog(t, srv, c))
	// The teleport aborts whatever the passenger does, as every teleport
	// does, with its ActionFailed acks.
	assertLog(t, "second 308", rest,
		af, af, af, af, teleport(me, runeOust), sm(serverpackets.SystemMessageExitPeacefulZone), sm(serverpackets.SystemMessageNotCorrectBoatTicket))

	sail(srv, 1)
	if _, checks := withoutChecks(passengerLog(t, srv, c)); len(checks) != 0 {
		t.Fatalf("checks after leaving: %q", checks)
	}
}

// TestPassengerLeavingCallsOffFare pins the reference's ticket collection
// being called off by any passenger leaving the boat: one passenger
// stepping off before the collection runs spares the other, who holds no
// ticket either and sails on.
func TestPassengerLeavingCallsOffFare(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 0)
	board(t, srv, c, b)
	other, _ := enterSecondClient(t, srv, runeBoarding)
	board(t, srv, other, b)

	sail(srv, 305)
	passengerLog(t, srv, c)
	passengerLog(t, srv, other)
	// A ground click near the deck's middle of a boat under way is walked by
	// the client alone and ends the boarding walk, which lets it step off
	// over the side.
	c.Send(encodeMoveBackward(runeShore, runeDock))
	x, y, z := srv.Boats.Boats()[0].Position()
	c.Send(encodeGetOffVehicle(b, location.Location{X: x, Y: y - 300, Z: z}))
	if log := passengerLog(t, srv, c); !slices.Contains(log, getOff(me, b, location.Location{X: x, Y: y - 300, Z: z})) {
		t.Fatalf("stepping off at sea: %q", log)
	}
	sail(srv, 4)
	rest, checks := withoutChecks(passengerLog(t, srv, other))
	var own []string
	for _, s := range rest {
		if !strings.Contains(s, fmt.Sprint(" ", me, " ")) {
			own = append(own, s)
		}
	}
	assertLog(t, "the other passenger, past 308", own)
	if len(checks) != 40 {
		t.Fatalf("the other passenger got %d checks over 4 seconds, want 40", len(checks))
	}
}

// TestPassengerStepsOff pins leaving a boat tied up: the passenger leaves
// the boat's peace, its deck walk is stopped at a cleared deck position, it
// is shown stepping off toward the point it asked for, and it is set on the
// dock's shore line, 1.2 times the way to the entrance, and walks there.
func TestPassengerStepsOff(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 0)
	board(t, srv, c, b)

	// Straight after boarding the boarding walk still stands, and stepping
	// off is refused; a ground click on the deck ends it.
	c.Send(encodeGetOffVehicle(b, runeShore))
	assertLog(t, "stepping off at once", passengerLog(t, srv, c), af)
	c.Send(encodeMoveBackward(runeShore, runeDock))
	passengerLog(t, srv, c)

	c.Send(encodeGetOffVehicle(b, runeShore))
	log := passengerLog(t, srv, c)
	want := []string{sm(serverpackets.SystemMessageExitPeacefulZone), deckStop(me, b, location.Location{}, 0), getOff(me, b, runeShore)}
	if len(log) < len(want) {
		t.Fatalf("stepping off: %q", log)
	}
	assertLog(t, "stepping off", log[:len(want)], want...)
	if x, y, _ := srv.PlayerPosition(t, me); x != runeGetOffAt.X || y != runeGetOffAt.Y {
		t.Fatalf("stepped off to %d,%d, want %v", x, y, runeGetOffAt)
	}

	// Ashore, a second request is refused.
	c.Send(encodeGetOffVehicle(b, runeShore))
	assertLog(t, "second step off", passengerLog(t, srv, c), af)
}

// TestPassengerDeckReports pins a passenger's deck bookkeeping: a stop the
// client reports is kept and echoed with ActionFailed; a position report
// within 500 of it changes nothing, and one past that is put back with
// GetOnVehicle naming the reported boat.
func TestPassengerDeckReports(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 0)
	board(t, srv, c, b)

	stop := location.Location{X: 50, Y: -150, Z: -40}
	c.Send(encodeCannotMoveInVehicle(b, stop, 16384))
	assertLog(t, "stop", passengerLog(t, srv, c), deckStop(me, b, stop, 16384), af)

	// A stop on another boat is ignored without an answer.
	c.Send(encodeCannotMoveInVehicle(b+1000, stop, 0))
	assertLog(t, "stop on another boat", passengerLog(t, srv, c))

	c.Send(encodeValidatePosition(location.Location{X: 50, Y: 340, Z: -40}, 0, b))
	assertLog(t, "drift 490", passengerLog(t, srv, c))
	c.Send(encodeValidatePosition(location.Location{X: 50, Y: 360, Z: -40}, 0, b))
	assertLog(t, "drift 510", passengerLog(t, srv, c), getOn(me, b, stop))
}

// TestPassengerClicksOnDeck pins a passenger's ground click on a boat tied
// up: toward the dock's entrance it walks on the deck to the entrance's deck
// point, then is answered ActionFailed; a click that crosses no line is
// answered ActionFailed alone.
func TestPassengerClicksOnDeck(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 0)
	board(t, srv, c, b)

	c.Send(encodeMoveBackward(runeShore, runeDock))
	assertLog(t, "toward the entrance", passengerLog(t, srv, c), deckWalk(me, b, deckEntrance, deckSpot), af)

	c.Send(encodeMoveBackward(offset(runeDock, 0, 100), runeDock))
	assertLog(t, "toward no line", passengerLog(t, srv, c), af)
}

// TestBoardingNeedsLeave pins the leave to board: a player who never
// clicked on a boat, or whose walk toward the entrance has not arrived, is
// refused; one whose walk to the entrance arrived boards.
func TestBoardingNeedsLeave(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeShore, 0)

	c.Send(encodeGetOnVehicle(b, deckSpot))
	assertLog(t, "no leave", passengerLog(t, srv, c), af)

	// A ground click across the entrance walks there instead.
	c.Send(encodeMoveBackward(runeDock, runeShore))
	log := passengerLog(t, srv, c)
	if len(log) != 1 || log[0] != move(me, runeEntrance, runeShore) {
		t.Fatalf("click across the entrance: %q, want the walk to %v", log, runeEntrance)
	}
	c.Send(encodeGetOnVehicle(b, deckSpot))
	assertLog(t, "walking", passengerLog(t, srv, c), af)

	srv.AdvanceUntil(t, "arrival at the entrance", func() bool {
		x, y, _ := srv.PlayerPosition(t, me)
		return x == runeEntrance.X && y == runeEntrance.Y
	})
	passengerLog(t, srv, c)
	c.Send(encodeGetOnVehicle(b, deckSpot))
	assertLog(t, "arrived", passengerLog(t, srv, c), sm(serverpackets.SystemMessageEnterPeacefulZone), getOn(me, b, deckSpot))
}

// TestDeckClickFromAfarWalksToEntrance pins a click on the deck from the
// shore: the player walks to the boarding point, nine tenths of the way to
// where its walk to the clicked deck point crosses the entrance, and is
// answered ActionFailed.
func TestDeckClickFromAfarWalksToEntrance(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeShore, 0)

	c.Send(encodeMoveInVehicle(b, deckCenter, deckSpot))
	assertLog(t, "deck click", passengerLog(t, srv, c),
		move(me, location.Location{X: runeBoarding.X, Y: runeBoarding.Y, Z: -3624}, runeShore), af)
}

// TestPassengerShownAboard pins how a passenger is shown to a player who
// comes to know it: CharInfo names its boat, and GetOnVehicle follows with
// its deck position.
func TestPassengerShownAboard(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 0)
	board(t, srv, c, b)

	frames := enterSecond(t, srv, runeShore)
	var boatOf int32 = -1
	var log []string
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeCharInfo {
			boatOf = int32(uint32(f[13]) | uint32(f[14])<<8 | uint32(f[15])<<16 | uint32(f[16])<<24)
			log = append(log, "charinfo")
		}
		if s, ok := describePassenger(f); ok && strings.HasPrefix(s, "geton ") {
			log = append(log, s)
		}
	}
	assertLog(t, "second player's entry", log, "charinfo", getOn(me, b, deckSpot))
	if boatOf != b {
		t.Fatalf("CharInfo boat %d, want %d", boatOf, b)
	}
}

// TestPassengerLogsOutAboard pins the save of a passenger leaving the world
// at sea: its position is stored as the shore of the dock its boat serves.
func TestPassengerLogsOutAboard(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 1)
	board(t, srv, c, b)
	sail(srv, 305)
	passengerLog(t, srv, c)

	c.Send(encodeSingle(clientpackets.OpcodeLogout))
	srv.AdvanceUntil(t, "logout save", func() bool {
		return storedPosition(t, srv, me) == runeOust
	})
}
