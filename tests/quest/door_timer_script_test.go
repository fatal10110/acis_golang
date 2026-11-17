package quest

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// A script that changes a timed door gives it the same auto change an admin
// command does: the door returns to the state it was scheduled for after its
// close time, and a change made in between leaves the timer with nothing to
// do.

func scriptOpensSideGate(log *doorChangeLog) script.Catalog {
	return script.Catalog{
		"script.ai.DoorWatchFirst": func() script.Script {
			return script.Script{ChangedDoors: []int32{watchGateID}, Hooks: script.Hooks{OnDoorChange: log.hook("first", func(sc *script.Script, e script.DoorChange) {
				if e.Door.Opened() {
					sc.Door("side_gate").Open()
				}
			})}}
		},
	}
}

// TestScriptOpenedTimedDoorClosesAfterItsCloseTime: opening the watched gate
// has its script open the side gate, which closes again 10 seconds later and
// shows the close to the player.
func TestScriptOpenedTimedDoorClosesAfterItsCloseTime(t *testing.T) {
	t.Parallel()
	timed := changeDoors()
	timed[2].CloseTime = 10
	srv := bootDoorChangeWith(t, timed, []string{"script.ai.DoorWatchFirst"}, scriptOpensSideGate(&doorChangeLog{}))
	side := doorObjectID(t, srv, sideGateID)
	sideDoor, _ := srv.WorldObjects.Door(sideGateID)

	srv.WorldObjects.SetDoorOpen(watchGateID, true)
	if !sideDoor.Opened() || !srv.DoorTimerPending(t, sideGateID) {
		t.Fatalf("side gate after the script's open: opened %v, timer pending %v; want open with a timer", sideDoor.Opened(), srv.DoorTimerPending(t, sideGateID))
	}
	srv.ReadQueued(t, srv.Client)

	srv.AdvanceDoorTimers(t, 9*time.Second)
	if !sideDoor.Opened() {
		t.Fatal("side gate closed before its close time")
	}
	srv.AdvanceDoorTimers(t, time.Second)
	if sideDoor.Opened() || srv.DoorTimerPending(t, sideGateID) {
		t.Fatalf("side gate at its close time: opened %v, timer pending %v; want closed, no timer", sideDoor.Opened(), srv.DoorTimerPending(t, sideGateID))
	}
	var closes [][2]int32
	for _, u := range doorUpdateOrder(srv.ReadQueued(t, srv.Client)) {
		if u[0] == side {
			closes = append(closes, u)
		}
	}
	if want := [][2]int32{{side, 1}}; !slices.Equal(closes, want) {
		t.Fatalf("side gate DoorStatusUpdate (object, closed) = %v, want one close %v", closes, want)
	}
}

// TestScriptClosedTimedDoorLeavesItsTimerNothingToDo: the script opens the
// side gate and the door is then closed by hand before its timer falls due;
// the timer finds it closed, changes nothing, shows nothing and is gone.
func TestScriptClosedTimedDoorLeavesItsTimerNothingToDo(t *testing.T) {
	t.Parallel()
	timed := changeDoors()
	timed[2].CloseTime = 10
	srv := bootDoorChangeWith(t, timed, []string{"script.ai.DoorWatchFirst"}, scriptOpensSideGate(&doorChangeLog{}))
	side := doorObjectID(t, srv, sideGateID)
	sideDoor, _ := srv.WorldObjects.Door(sideGateID)

	srv.WorldObjects.SetDoorOpen(watchGateID, true)
	srv.AdvanceDoorTimers(t, 3*time.Second)
	srv.WorldObjects.SetDoorOpen(sideGateID, false)
	srv.ReadQueued(t, srv.Client)

	srv.AdvanceDoorTimers(t, 7*time.Second)
	for _, u := range doorUpdateOrder(srv.ReadQueued(t, srv.Client)) {
		if u[0] == side {
			t.Fatalf("the timer of the closed side gate sent %v, want no door change", u)
		}
	}
	if sideDoor.Opened() || srv.DoorTimerPending(t, sideGateID) {
		t.Fatalf("side gate after the timer: opened %v, timer pending %v; want closed, no timer", sideDoor.Opened(), srv.DoorTimerPending(t, sideGateID))
	}
}
