package admin

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	reloadGateID    = 24190101 // closed at boot, open once reloaded
	reloadOpenID    = 24190102 // open at boot, gone once reloaded
	reloadNewGateID = 24190103 // only in the reloaded doors, closed
)

// reloadDoor is a door template beside the game master at x.
func reloadDoor(id, x int, opened bool) *door.Template {
	return &door.Template{
		ID: id, Name: "reload_gate", Kind: door.KindDoor, Level: 1,
		Position: location.Location{X: x, Y: spawnY, Z: spawnZ},
		Coordinates: []location.Point{
			{X: x - 8, Y: spawnY - 8}, {X: x + 8, Y: spawnY - 8}, {X: x + 8, Y: spawnY + 8}, {X: x - 8, Y: spawnY + 8},
		},
		HP: 500, PDef: 100, MDef: 100, Height: 32, Opened: opened,
	}
}

// doorFrame is a DoorInfo or DoorStatusUpdate frame's object, door id and
// closed flag.
type doorFrame struct {
	opcode   byte
	objectID int32
	doorID   int32
	closed   bool
}

func readDoorFrame(t *testing.T, frame []byte) doorFrame {
	t.Helper()
	r := wire.NewReader(frame[1:])
	f := doorFrame{opcode: frame[0], objectID: r.ReadInt32()}
	switch frame[0] {
	case serverpackets.OpcodeDoorInfo:
		f.doorID = r.ReadInt32()
		r.ReadInt32() // show HP
		r.ReadInt32() // targetable
		f.closed = r.ReadInt32() == 1
	case serverpackets.OpcodeDoorStatusUpdate:
		f.closed = r.ReadInt32() == 1
		r.ReadInt32() // damage stage
		r.ReadInt32() // show HP
		f.doorID = r.ReadInt32()
	case serverpackets.OpcodeDeleteObject:
	default:
		t.Fatalf("opcode %#x is not a door frame", frame[0])
	}
	return f
}

// TestAdminReloadDoor pins //reload door (DoorData.reload): every door
// standing opens, so its geodata blocker goes, and leaves the world; each
// door read anew enters it in the state opposite to its template's, then
// takes that state, and the game master is told the doors reloaded. The
// doors the server resolves by id are the new ones, blocking geodata as
// their state says. A doors.xml that cannot be read leaves the doors as
// they were.
func TestAdminReloadDoor(t *testing.T) {
	t.Parallel()
	reloaded := []*door.Template{reloadDoor(reloadGateID, 100, true), reloadDoor(reloadNewGateID, 200, false)}
	readErr := errors.New("broken doors.xml")
	var failRead atomic.Bool
	reloads := network.DataReloads{Doors: func() (*door.Table, error) {
		if failRead.Load() {
			return nil, readErr
		}
		return door.NewTable(reloaded)
	}}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithDoors(reloadDoor(reloadGateID, 100, false), reloadDoor(reloadOpenID, 300, true)),
		gameservertest.WithDataReloads(reloads),
	)
	gm := srv.Client
	enterWorld(t, gm)
	geo := srv.DoorGeo(t)
	blocks := func(x int) bool { return !geo.CanMove(x-48, spawnY, spawnZ, x+48, spawnY, spawnZ) }

	oldGate, _ := srv.WorldObjects.Door(reloadGateID)
	oldOpen, _ := srv.WorldObjects.Door(reloadOpenID)
	if !blocks(100) || blocks(300) {
		t.Fatalf("fixture: closed gate blocks=%v, open door blocks=%v; want true, false", blocks(100), blocks(300))
	}

	// The read failing: the usage, and the doors stay.
	failRead.Store(true)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload door")), reloadUsage...)
	if d, _ := srv.WorldObjects.Door(reloadGateID); d != oldGate || !blocks(100) {
		t.Fatal("a failed reload replaced the doors")
	}
	failRead.Store(false)

	frames := exchange(t, gm, encodeBuildCmd("reload doors"))
	gate, ok := srv.WorldObjects.Door(reloadGateID)
	if !ok || gate == oldGate {
		t.Fatalf("door %d after the reload = %v, want a new door", reloadGateID, gate)
	}
	newGate, ok := srv.WorldObjects.Door(reloadNewGateID)
	if !ok {
		t.Fatalf("door %d not spawned by the reload", reloadNewGateID)
	}
	if _, ok := srv.WorldObjects.Door(reloadOpenID); ok {
		t.Fatalf("door %d outlived a reload that no longer reads it", reloadOpenID)
	}
	want := []doorFrame{
		// The closed gate opens; the open door is left as it is.
		{serverpackets.OpcodeDoorStatusUpdate, oldGate.ObjectID(), reloadGateID, false},
		{serverpackets.OpcodeDeleteObject, oldGate.ObjectID(), 0, false},
		{serverpackets.OpcodeDeleteObject, oldOpen.ObjectID(), 0, false},
		// Each new door shows the state opposite to its template's, then
		// takes it.
		{serverpackets.OpcodeDoorInfo, gate.ObjectID(), reloadGateID, true},
		{serverpackets.OpcodeDoorStatusUpdate, gate.ObjectID(), reloadGateID, true},
		{serverpackets.OpcodeDoorStatusUpdate, gate.ObjectID(), reloadGateID, false},
		{serverpackets.OpcodeDoorInfo, newGate.ObjectID(), reloadNewGateID, false},
		{serverpackets.OpcodeDoorStatusUpdate, newGate.ObjectID(), reloadNewGateID, false},
		{serverpackets.OpcodeDoorStatusUpdate, newGate.ObjectID(), reloadNewGateID, true},
	}
	if len(frames) != len(want)+1 {
		t.Fatalf("//reload door frames = %x, want %d door frames and the answer", testsupport.FrameOpcodes(frames), len(want))
	}
	for i, w := range want {
		if got := readDoorFrame(t, frames[i]); got != w {
			t.Fatalf("frame %d = %+v, want %+v", i, got, w)
		}
	}
	assertTexts(t, frames[len(want):], "Doors instance has been reloaded.")

	if !gate.Opened() || newGate.Opened() {
		t.Fatalf("reloaded gate opened=%v, new gate opened=%v; want true, false", gate.Opened(), newGate.Opened())
	}
	if blocks(100) || !blocks(200) || blocks(300) {
		t.Fatalf("geodata blocks at the gate=%v, the new gate=%v, the dropped door=%v; want false, true, false", blocks(100), blocks(200), blocks(300))
	}
	for _, old := range []*door.Object{oldGate, oldOpen} {
		if _, ok := srv.State.Object(old.ObjectID()); ok {
			t.Fatalf("replaced door object %d is still in the world", old.ObjectID())
		}
	}

	// A state change by door id reaches the new door.
	if !srv.WorldObjects.SetDoorOpen(reloadNewGateID, true) || !newGate.Opened() || blocks(200) {
		t.Fatal("opening the new gate by id did not open it")
	}
}
