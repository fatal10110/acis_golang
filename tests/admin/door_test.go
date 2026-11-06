package admin

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// adminDoorID is the door the //open and //close scenario works on.
const adminDoorID = 24190201

// requireDoorShown fails unless frames hold a DoorStatusUpdate of d showing
// it closed as want says.
func requireDoorShown(t *testing.T, what string, frames [][]byte, d *door.Object, closed bool) {
	t.Helper()
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeDoorStatusUpdate {
			continue
		}
		if got := readDoorFrame(t, f); got.objectID == d.ObjectID() && got.closed == closed {
			return
		}
	}
	t.Fatalf("%s frames = %x, want DoorStatusUpdate closed=%v", what, testsupport.FrameOpcodes(frames), closed)
}

// TestAdminDoorOpenClose pins //open and //close: by id the door changes
// state and observers see it; an id naming no door is an invalid target,
// as is a missing id with no door selected; with a door selected a missing
// or unreadable id acts on it.
func TestAdminDoorOpenClose(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithDoors(reloadDoor(adminDoorID, 100, false)))
	gm := srv.Client
	enterWorld(t, gm)
	d, ok := srv.WorldObjects.Door(adminDoorID)
	if !ok {
		t.Fatal("door missing")
	}
	id := strconv.Itoa(adminDoorID)

	requireDoorShown(t, "//open id", exchange(t, gm, encodeBuildCmd("open "+id)), d, false)
	if !d.Opened() {
		t.Fatal("door closed after //open id")
	}
	requireDoorShown(t, "//close id", exchange(t, gm, encodeBypass("admin_close "+id)), d, true)
	if d.Opened() {
		t.Fatal("door open after //close id")
	}

	for _, cmd := range []string{"open 99999999", "open", "close x"} {
		frames := exchange(t, gm, encodeBuildCmd(cmd))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want INVALID_TARGET alone", cmd, testsupport.FrameOpcodes(frames))
		}
		assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	}

	exchange(t, gm, encodeAction(d.ObjectID()))
	requireDoorShown(t, "//open on the selected door", exchange(t, gm, encodeBuildCmd("open")), d, false)
	requireDoorShown(t, "//close x on the selected door", exchange(t, gm, encodeBuildCmd("close x")), d, true)
	if d.Opened() {
		t.Fatal("selected door open after //close x")
	}
}

// TestAdminManageDoor pins //kill and //heal on a selected door: outside a
// siege the kill takes no HP; a heal fills a damaged door's HP and shows it.
func TestAdminManageDoor(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithDoors(reloadDoor(adminDoorID, 100, false)))
	gm := srv.Client
	enterWorld(t, gm)
	d, ok := srv.WorldObjects.Door(adminDoorID)
	if !ok {
		t.Fatal("door missing")
	}
	exchange(t, gm, encodeAction(d.ObjectID()))

	exchange(t, gm, encodeBuildCmd("kill"))
	if d.Dead() || d.CurrentHP() != float64(d.MaxHP()) {
		t.Fatalf("door after //kill outside a siege: dead %v HP %v/%d, want alive and full", d.Dead(), d.CurrentHP(), d.MaxHP())
	}

	srv.WorldObjects.TakeDoorHP(adminDoorID, 100)
	drain(t, gm)
	requireDoorShown(t, "//heal", exchange(t, gm, encodeBuildCmd("heal")), d, true)
	if d.CurrentHP() != float64(d.MaxHP()) {
		t.Fatalf("door HP after //heal = %v, want %d", d.CurrentHP(), d.MaxHP())
	}
}
