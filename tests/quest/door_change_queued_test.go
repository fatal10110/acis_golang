package quest

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The door-change hook around the door operations that change several
// doors under one lock (#3809): a siege's door respawn and a door reload
// run the hooks once they are done; a door a hook's reload replaced gets
// no auto timer from the change that ran the hook.

// TestScriptDoorChangeReloadFromHookLeavesNoStaleTimer: the watched gate
// closes again after a minute; its hook reloads the doors with a gate that
// never closes by itself. Once the opening returns, the gate has no auto
// timer: the replaced gate's opening schedules none.
func TestScriptDoorChangeReloadFromHookLeavesNoStaleTimer(t *testing.T) {
	t.Parallel()
	log := &doorChangeLog{}
	timed := changeDoors()
	timed[1].CloseTime = 60
	plain, err := door.NewTable(changeDoors())
	if err != nil {
		t.Fatalf("door table: %v", err)
	}
	var (
		srv       *gameservertest.Server
		reloaded  bool
		reloadErr error
	)
	catalog := script.Catalog{
		"script.ai.DoorWatchFirst": func() script.Script {
			return script.Script{ChangedDoors: []int32{watchGateID}, Hooks: script.Hooks{OnDoorChange: log.hook("first", func(*script.Script, script.DoorChange) {
				if !reloaded {
					reloaded = true
					reloadErr = srv.WorldObjects.ReloadDoors(plain)
				}
			})}}
		},
	}
	srv = bootDoorChangeWith(t, timed, []string{"script.ai.DoorWatchFirst"}, catalog)
	srv.ReadQueued(t, srv.Client)

	srv.WorldObjects.SetDoorOpen(watchGateID, true)
	if reloadErr != nil {
		t.Fatalf("reload from the hook: %v", reloadErr)
	}
	if got, want := log.take(), []doorChangeSeen{{"first", "watch_gate", true}}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %+v, want %+v", got, want)
	}
	gate, ok := srv.WorldObjects.Door(watchGateID)
	if !ok || gate.Template.CloseTime != 0 || gate.Opened() {
		t.Fatalf("gate after the reload: found %v, close time %d, opened %v; want the reloaded closed gate", ok, gate.Template.CloseTime, gate.Opened())
	}
	if srv.DoorTimerPending(t, watchGateID) {
		t.Fatal("the reloaded gate has an auto timer, want none")
	}
}

// TestScriptDoorChangeAfterRespawn: a siege's respawn of the open watched
// gate closes it and stands its HP again; the hook runs once after both,
// sees the gate closed, and opens the side gate from there.
func TestScriptDoorChangeAfterRespawn(t *testing.T) {
	t.Parallel()
	log := &doorChangeLog{}
	catalog := script.Catalog{
		"script.ai.DoorWatchFirst": func() script.Script {
			return script.Script{ChangedDoors: []int32{watchGateID}, Hooks: script.Hooks{OnDoorChange: log.hook("first", func(sc *script.Script, e script.DoorChange) {
				if !e.Door.Opened() {
					sc.Door("side_gate").Open()
				}
			})}}
		},
	}
	srv := bootDoorChange(t, []string{"script.ai.DoorWatchFirst"}, catalog)
	watch, side := doorObjectID(t, srv, watchGateID), doorObjectID(t, srv, sideGateID)
	srv.WorldObjects.SetDoorOpen(watchGateID, true)
	log.take()
	srv.ReadQueued(t, srv.Client)

	srv.WorldObjects.RespawnDoor(watchGateID, 1, false)
	if got, want := log.take(), []doorChangeSeen{{"first", "watch_gate", false}}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %+v, want %+v", got, want)
	}
	var got [][2]int32
	for _, u := range doorUpdateOrder(srv.ReadQueued(t, srv.Client)) {
		if u[0] == watch || u[0] == side {
			got = append(got, u)
		}
	}
	if want := [][2]int32{{watch, 1}, {watch, 1}, {side, 0}}; !slices.Equal(got, want) {
		t.Fatalf("DoorStatusUpdate (object, closed) = %v, want the close, the HP, then the side gate %v", got, want)
	}
}
