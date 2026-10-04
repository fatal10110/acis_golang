package boat

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// ticketID is the ticket the passenger itinerary's Rune leg charges: a
// stackable item the test catalog carries.
const ticketID = 20

// Rune dock geometry, hand-derived from the reference BoatDock.RUNE lines
// with the reference Line2D/Point2D arithmetic (rounding half up, the
// intersection truncated):
//   - deck point (100, -200) lies at world (34332, -37894);
//   - from (34600, -38100) the walk to deck point (0, -100), at world
//     (34335, -37764), crosses the entrance at (34466, -37930); nine tenths
//     of the way there is (34479, -37947), the boarding point;
//   - from (34479, -37947) the walk to deck point (100, -200) ends within 50
//     of its boarding point (34459, -37940);
//   - from (34600, -38100) the walk to the boat's tie-up point crosses the
//     entrance; nine tenths of the way there is (34506, -37922);
//   - from the tie-up point a walk toward (34600, -38100) crosses the
//     entrance at 1.2 times the way: (34519, -37946), 299.7 away, which is
//     deck point (281, -92).
var (
	runeOust      = location.Location{X: 34513, Y: -38009, Z: -3640}
	runeShore     = location.Location{X: 34600, Y: -38100, Z: -3610}
	runeBoarding  = location.Location{X: 34479, Y: -37947, Z: -3610}
	runeEntrance  = location.Location{X: 34506, Y: -37922, Z: -3624}
	runeGetOffAt  = location.Location{X: 34519, Y: -37946, Z: -3624}
	deckCenter    = location.Location{X: 0, Y: -100, Z: -40}
	deckSpot      = location.Location{X: 100, Y: -200, Z: -40}
	deckEntrance  = location.Location{X: 281, Y: -92, Z: -40}
	runeBoatFirst = location.Location{X: 34361, Y: -37680, Z: -3610}
)

// passengerItinerary is runeRoundTrip charging ticketID on its Rune leg.
func passengerItinerary() route.BoatItinerary {
	it, _ := runeRoundTrip()
	it.Routes[0].ItemID = ticketID
	return it
}

// bootPassenger boots one character standing at at with tickets tickets,
// and the passenger itinerary's boat tied up at Rune, and enters the world.
func bootPassenger(t *testing.T, at location.Location, tickets int32) (*gameservertest.Server, *testsupport.ScriptedClient, int32, int32) {
	t.Helper()
	return bootPassengerWith(t, at, tickets, nil)
}

// bootPassengerWith is bootPassenger with extra boot options, and setup, if
// any, run on the character before it enters the world.
func bootPassengerWith(t *testing.T, at location.Location, tickets int32, setup func(srv *gameservertest.Server, objID int32), opts ...gameservertest.Option) (*gameservertest.Server, *testsupport.ScriptedClient, int32, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Sailor", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithBoats(passengerItinerary()),
	}, opts...)...)
	objID := srv.SoleObjectID(t)
	if setup != nil {
		setup(srv, objID)
	}
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", at.X, at.Y, at.Z, objID); err != nil {
		t.Fatalf("place character: %v", err)
	}
	if tickets > 0 {
		srv.GiveItem(t, objID, ticketID, tickets)
	}
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readUntilQuiet(c)
	return srv, c, objID, srv.Boats.Boats()[0].ObjectID()
}

// board takes the character aboard from the boarding point: a click on the
// deck it stands next to, then the request to board.
func board(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, b int32) {
	t.Helper()
	c.Send(encodeMoveInVehicle(b, deckSpot, deckCenter))
	c.Send(encodeGetOnVehicle(b, deckSpot))
	srv.ReadQueued(t, c)
}

// passengerLog renders the passenger frames c has been sent so far,
// leaving out the boat's own moves.
func passengerLog(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) []string {
	t.Helper()
	boats := make(map[string]bool)
	for _, b := range srv.Boats.Boats() {
		boats[fmt.Sprintf("move %d ", b.ObjectID())] = true
	}
	var out []string
	for _, f := range srv.ReadQueued(t, c) {
		s, ok := describePassenger(f)
		if !ok || (len(s) > 5 && boats[s[:strings.Index(s[5:], " ")+6]]) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// describePassenger renders the frames a passenger flow sends: the boat
// passenger packets, moves, teleports, system messages and ActionFailed.
func describePassenger(frame []byte) (string, bool) {
	r := wire.NewReader(frame[1:])
	loc := func() location.Location {
		return location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	}
	switch frame[0] {
	case serverpackets.OpcodeGetOnVehicle:
		id, boatID := r.ReadInt32(), r.ReadInt32()
		return fmt.Sprintf("geton %d %d %v", id, boatID, loc()), true
	case serverpackets.OpcodeGetOffVehicle:
		id, boatID := r.ReadInt32(), r.ReadInt32()
		return fmt.Sprintf("getoff %d %d %v", id, boatID, loc()), true
	case serverpackets.OpcodeMoveToLocationInVehicle:
		id, boatID := r.ReadInt32(), r.ReadInt32()
		to := loc()
		return fmt.Sprintf("deckwalk %d %d %v from %v", id, boatID, to, loc()), true
	case serverpackets.OpcodeStopMoveInVehicle:
		id, boatID := r.ReadInt32(), r.ReadInt32()
		at := loc()
		return fmt.Sprintf("deckstop %d %d %v h%d", id, boatID, at, r.ReadInt32()), true
	case serverpackets.OpcodeOnVehicleCheckLocation:
		boatID := r.ReadInt32()
		at := loc()
		return fmt.Sprintf("check %d %v h%d", boatID, at, r.ReadInt32()), true
	case serverpackets.OpcodeMoveToLocation:
		id := r.ReadInt32()
		dest := loc()
		return fmt.Sprintf("move %d %v from %v", id, dest, loc()), true
	case serverpackets.OpcodeTeleportToLocation:
		id := r.ReadInt32()
		return fmt.Sprintf("teleport %d %v", id, loc()), true
	case serverpackets.OpcodeSystemMessage:
		return fmt.Sprintf("sm %d", r.ReadInt32()), true
	case serverpackets.OpcodeActionFailed:
		return "af", true
	}
	return "", false
}

// withoutChecks drops the OnVehicleCheckLocation lines of log, returning
// them apart.
func withoutChecks(log []string) (rest, checks []string) {
	for _, s := range log {
		if len(s) > 6 && s[:6] == "check " {
			checks = append(checks, s)
			continue
		}
		rest = append(rest, s)
	}
	return rest, checks
}

func encodeMoveInVehicle(boatID int32, target, origin location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMoveInVehicle)
	w.WriteInt32(boatID)
	for _, v := range []int{target.X, target.Y, target.Z, origin.X, origin.Y, origin.Z} {
		w.WriteInt32(int32(v))
	}
	return w.Bytes()
}

func encodeGetOnVehicle(boatID int32, at location.Location) []byte {
	return encodeBoatPoint(clientpackets.OpcodeRequestGetOnVehicle, boatID, at)
}

func encodeGetOffVehicle(boatID int32, at location.Location) []byte {
	return encodeBoatPoint(clientpackets.OpcodeRequestGetOffVehicle, boatID, at)
}

func encodeBoatPoint(opcode byte, boatID int32, at location.Location) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(boatID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	return w.Bytes()
}

func encodeCannotMoveInVehicle(boatID int32, at location.Location, heading int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeCannotMoveInVehicle)
	w.WriteInt32(boatID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(heading)
	return w.Bytes()
}

func encodeMoveBackward(target, origin location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	for _, v := range []int{target.X, target.Y, target.Z, origin.X, origin.Y, origin.Z} {
		w.WriteInt32(int32(v))
	}
	w.WriteInt32(1) // mouse click
	return w.Bytes()
}

func encodeValidatePosition(at location.Location, heading, boatID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeValidatePosition)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(heading)
	w.WriteInt32(boatID)
	return w.Bytes()
}

// Expected renderings of describePassenger.

func getOn(id, boatID int32, at location.Location) string {
	return fmt.Sprintf("geton %d %d %v", id, boatID, at)
}

func getOff(id, boatID int32, at location.Location) string {
	return fmt.Sprintf("getoff %d %d %v", id, boatID, at)
}

func deckWalk(id, boatID int32, to, from location.Location) string {
	return fmt.Sprintf("deckwalk %d %d %v from %v", id, boatID, to, from)
}

func deckStop(id, boatID int32, at location.Location, heading int) string {
	return fmt.Sprintf("deckstop %d %d %v h%d", id, boatID, at, heading)
}

func check(boatID int32, at location.Location, heading int) string {
	return fmt.Sprintf("check %d %v h%d", boatID, at, heading)
}

func teleport(id int32, at location.Location) string { return fmt.Sprintf("teleport %d %v", id, at) }

func sm(id int) string { return fmt.Sprintf("sm %d", id) }

const af = "af"

func encodeSingle(opcode byte) []byte { return wire.NewPacketWriter(opcode).Bytes() }

// storedPosition reads objID's stored position.
func storedPosition(t *testing.T, srv *gameservertest.Server, objID int32) location.Location {
	t.Helper()
	var at location.Location
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT x, y, z FROM characters WHERE obj_Id = ?", objID).Scan(&at.X, &at.Y, &at.Z); err != nil {
		t.Fatalf("read position: %v", err)
	}
	return at
}

// enterSecondClient brings a second character into the world at at and
// returns its client, quiet, and its object id.
func enterSecondClient(t *testing.T, srv *gameservertest.Server, at location.Location) (*testsupport.ScriptedClient, int32) {
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
	readUntilQuiet(c)
	return c, ch.ID
}
