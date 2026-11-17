package admin

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The linked pair of doors.xml (19160010 and 19160011 trigger each other
// and close 10 seconds after an open).
const (
	timerPairFirst  = 19160010
	timerPairSecond = 19160011
)

// timedDoor is a closed door that closes again closeTime seconds after an
// open, triggering triggeredID's door when it is not zero.
func timedDoor(id, x, closeTime, triggeredID int) *door.Template {
	tmpl := reloadDoor(id, x, false)
	tmpl.CloseTime, tmpl.TriggeredID = closeTime, triggeredID
	return tmpl
}

// requireNoDoorChange fails when the frames queued for c since the last read
// hold any DoorStatusUpdate.
func requireNoDoorChange(t *testing.T, what string, c *testsupport.ScriptedClient) {
	t.Helper()
	for _, f := range c.ReadQueued() {
		if f[0] == serverpackets.OpcodeDoorStatusUpdate {
			t.Fatalf("%s: door frame %+v, want no door change", what, readDoorFrame(t, f))
		}
	}
}

func requireDoorStates(t *testing.T, what string, want bool, doors ...*door.Object) {
	t.Helper()
	for _, d := range doors {
		if d.Opened() != want {
			t.Fatalf("%s: door %d opened = %v, want %v", what, d.DoorID(), d.Opened(), want)
		}
	}
}

// TestDoorTimerOfCascadeClosedDoorLeavesItClosed pins a door's auto change
// to the state it was scheduled for: //open on one door of a linked pair
// opens both and schedules the first's auto-close; //close on the second
// closes both. When the auto-close falls due the first door is already
// closed, so it stays closed, nobody sees a change, and no timer is left.
func TestDoorTimerOfCascadeClosedDoorLeavesItClosed(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithDoors(
		timedDoor(timerPairFirst, 100, 10, timerPairSecond),
		timedDoor(timerPairSecond, 140, 10, timerPairFirst),
	))
	gm := srv.Client
	enterWorld(t, gm)
	first, _ := srv.WorldObjects.Door(timerPairFirst)
	second, _ := srv.WorldObjects.Door(timerPairSecond)

	exchange(t, gm, encodeBuildCmd("open "+strconv.Itoa(timerPairFirst)))
	requireDoorStates(t, "after //open of the first door", true, first, second)
	exchange(t, gm, encodeBuildCmd("close "+strconv.Itoa(timerPairSecond)))
	requireDoorStates(t, "after //close of the second door", false, first, second)
	drain(t, gm)

	srv.AdvanceDoorTimers(t, 10*time.Second)
	requireNoDoorChange(t, "first door's auto-close", gm)
	requireDoorStates(t, "after the first door's auto-close", false, first, second)
	for _, id := range []int{timerPairFirst, timerPairSecond} {
		if srv.DoorTimerPending(t, id) {
			t.Fatalf("door %d still has a timer pending", id)
		}
	}
}

// TestDoorTimerFromFirstOpenStillCloses pins that a later change drops no
// auto change already pending: a door opened at t0, closed at t2 and opened
// again at t3 closes at t0+closeTime, and the auto-close the reopen
// scheduled finds it closed and does nothing.
func TestDoorTimerFromFirstOpenStillCloses(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithDoors(timedDoor(adminDoorID, 100, 10, 0)))
	gm := srv.Client
	enterWorld(t, gm)
	d, _ := srv.WorldObjects.Door(adminDoorID)
	id := strconv.Itoa(adminDoorID)

	exchange(t, gm, encodeBuildCmd("open "+id))
	srv.AdvanceDoorTimers(t, 2*time.Second)
	exchange(t, gm, encodeBuildCmd("close "+id))
	srv.AdvanceDoorTimers(t, time.Second)
	exchange(t, gm, encodeBuildCmd("open "+id))
	requireDoorStates(t, "after the reopen", true, d)
	drain(t, gm)

	srv.AdvanceDoorTimers(t, 6*time.Second)
	requireNoDoorChange(t, "before t0+closeTime", gm)
	requireDoorStates(t, "before t0+closeTime", true, d)

	srv.AdvanceDoorTimers(t, time.Second)
	requireDoorShown(t, "auto-close at t0+closeTime", gm.ReadQueued(), d, true)
	requireDoorStates(t, "at t0+closeTime", false, d)
	if !srv.DoorTimerPending(t, adminDoorID) {
		t.Fatal("the reopen's auto-close was dropped")
	}

	srv.AdvanceDoorTimers(t, 3*time.Second)
	requireNoDoorChange(t, "the reopen's auto-close", gm)
	requireDoorStates(t, "after the reopen's auto-close", false, d)
	if srv.DoorTimerPending(t, adminDoorID) {
		t.Fatal("door still has a timer pending")
	}
}
