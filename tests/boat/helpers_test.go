package boat

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/dbtest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

// Dock sites boats tie up at.
var (
	runeDock     = location.Location{X: 34381, Y: -37680, Z: -3610}
	gludinDock   = location.Location{X: -95686, Y: 150514, Z: -3610}
	tiDock       = location.Location{X: -96622, Y: 261660, Z: -3610}
	innadrilDock = location.Location{X: 111384, Y: 226232, Z: -3610}
)

// offset returns l moved by dx, dy.
func offset(l location.Location, dx, dy int) location.Location {
	return location.Location{X: l.X + dx, Y: l.Y + dy, Z: l.Z}
}

// node is a route node at l with the given speeds and messages.
func node(l location.Location, speed, rotation int) route.BoatLocation {
	return route.BoatLocation{Location: l, Speed: speed, Rotation: rotation}
}

// bootAt boots one character standing at at, with boats sailing
// itineraries, and enters the world. It returns the server and every frame
// of the entry.
func bootAt(t *testing.T, at location.Location, itineraries ...route.BoatItinerary) (*gameservertest.Server, [][]byte) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Sailor", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithBoats(itineraries...),
	)
	objID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", at.X, at.Y, at.Z, objID); err != nil {
		t.Fatalf("place character: %v", err)
	}
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	return srv, readUntilQuiet(c)
}

// sail drives the fleet seconds seconds forward.
func sail(srv *gameservertest.Server, seconds int) {
	for range seconds * 10 {
		srv.Boats.Tick()
	}
}

// boatLog returns, rendered by describe, every boat frame c has received
// since the last call: everything sent before a manor-list barrier.
func boatLog(t *testing.T, c *testsupport.ScriptedClient) []string {
	t.Helper()
	frames := testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeRequestManorList()) }, serverpackets.OpcodeExtended)
	return describeBoatFrames(frames)
}

func describeBoatFrames(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		if s, ok := describe(f); ok {
			out = append(out, s)
		}
	}
	return out
}

// describe renders a boat frame: a vehicle packet, a boat schedule line, a
// boat sound, a creature move.
func describe(frame []byte) (string, bool) {
	r := wire.NewReader(frame[1:])
	loc := func() location.Location {
		return location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	}
	switch frame[0] {
	case serverpackets.OpcodeVehicleInfo:
		id := r.ReadInt32()
		at := loc()
		return fmt.Sprintf("info %d %v h%d", id, at, r.ReadInt32()), true
	case serverpackets.OpcodeVehicleDeparture:
		id, speed, rotation := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		return fmt.Sprintf("depart %d %d/%d %v", id, speed, rotation, loc()), true
	case serverpackets.OpcodeVehicleStarted:
		id := r.ReadInt32()
		return fmt.Sprintf("started %d %d", id, r.ReadInt32()), true
	case serverpackets.OpcodeMoveToLocation:
		id := r.ReadInt32()
		dest := loc()
		return fmt.Sprintf("move %d %v from %v", id, dest, loc()), true
	case serverpackets.OpcodeCreatureSay:
		speaker, channel := r.ReadInt32(), r.ReadInt32()
		if speaker != 0 || channel != 11 {
			return "", false
		}
		sysString := r.ReadInt32()
		return fmt.Sprintf("say %d/%d", sysString, r.ReadInt32()), true
	case serverpackets.OpcodePlaySound:
		typ := r.ReadInt32()
		file := r.ReadString()
		bound, id := r.ReadInt32(), r.ReadInt32()
		at := loc()
		return fmt.Sprintf("sound %d %s bound=%d %d %v delay=%d", typ, file, bound, id, at, r.ReadInt32()), true
	case serverpackets.OpcodeDeleteObject:
		return fmt.Sprintf("delete %d", r.ReadInt32()), true
	}
	return "", false
}

func assertLog(t *testing.T, when string, got []string, want ...string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("%s: boat frames\n got %q\nwant %q", when, got, want)
	}
}

func readUntilQuiet(c *testsupport.ScriptedClient) [][]byte {
	var frames [][]byte
	for f := c.ReadWithTimeout(300 * time.Millisecond); f != nil; f = c.ReadWithTimeout(300 * time.Millisecond) {
		frames = append(frames, f)
	}
	return frames
}

func encodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeEnterWorld() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes()
}

func encodeRequestManorList() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestManorList)
	return w.Bytes()
}

func encodeAction(objectID int32, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func encodeAttackRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// Expected renderings of describe.

func say(id int) string { return fmt.Sprintf("say 801/%d", id) }

func sound(file string, boatID int32, at location.Location) string {
	return fmt.Sprintf("sound 0 %s bound=1 %d %v delay=0", file, boatID, at)
}

func move(boatID int32, dest, from location.Location) string {
	return fmt.Sprintf("move %d %v from %v", boatID, dest, from)
}

func depart(boatID int32, speed, rotation int, dest location.Location) string {
	return fmt.Sprintf("depart %d %d/%d %v", boatID, speed, rotation, dest)
}

func deleted(objectID int32) string { return fmt.Sprintf("delete %d", objectID) }

func started(boatID int32, state int) string { return fmt.Sprintf("started %d %d", boatID, state) }

func info(boatID int32, at location.Location, heading int) string {
	return fmt.Sprintf("info %d %v h%d", boatID, at, heading)
}

const (
	fiveMinutes      = "itemsound.ship_5min"
	oneMinute        = "itemsound.ship_1min"
	arrivalDeparture = "itemsound.ship_arrival_departure"
)
