package quest

import (
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The door-change hook (#3809): a script lists the doors whose opening or
// closing reaches its hook; each state change of such a door runs the hook
// of every script watching it, in list order, after the door's status
// broadcast and before the door it triggers changes. The registration
// belongs to the standing door: a door reload drops it.

// The fixture doors: the lever triggers the watched gate, which triggers
// the far gate; the side gate is watched too, and opened from a hook.
const (
	leverDoorID = 19210011
	watchGateID = 19210012
	sideGateID  = 19210013
	farGateID   = 19210014
)

func changeDoor(id int, name string, x, triggered int) *door.Template {
	return &door.Template{
		ID: id, Name: name, Kind: door.KindDoor, Level: 1,
		Position:    location.Location{X: x, Y: 0, Z: 0},
		Coordinates: []location.Point{{X: x - 8, Y: -8}, {X: x + 8, Y: -8}, {X: x + 8, Y: 8}, {X: x - 8, Y: 8}},
		HP:          100, PDef: 10, MDef: 10, Height: 32,
		TriggeredID: triggered,
	}
}

func changeDoors() []*door.Template {
	return []*door.Template{
		changeDoor(leverDoorID, "lever_gate", 40, watchGateID),
		changeDoor(watchGateID, "watch_gate", 80, farGateID),
		changeDoor(sideGateID, "side_gate", 120, 0),
		changeDoor(farGateID, "far_gate", 160, 0),
	}
}

// doorChangeSeen is one door-change hook run: the script, the door it was
// handed and that door's open state read inside the hook.
type doorChangeSeen struct {
	script string
	door   string
	opened bool
}

type doorChangeLog struct {
	mu   sync.Mutex
	seen []doorChangeSeen
}

func (l *doorChangeLog) hook(name string, follow func(*script.Script, script.DoorChange)) func(*script.Script, script.DoorChange) {
	return func(sc *script.Script, e script.DoorChange) {
		l.mu.Lock()
		l.seen = append(l.seen, doorChangeSeen{script: name, door: e.Door.Name(), opened: e.Door.Opened()})
		l.mu.Unlock()
		if follow != nil {
			follow(sc, e)
		}
	}
}

func (l *doorChangeLog) take() []doorChangeSeen {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.seen
	l.seen = nil
	return out
}

// bootDoorChange boots the fixture doors with the scripts of catalog, in
// list order, and enters the world beside the doors.
func bootDoorChange(t *testing.T, list []string, catalog script.Catalog) *gameservertest.Server {
	t.Helper()
	var listing []script.Listing
	for _, p := range list {
		listing = append(listing, script.Listing{Path: p})
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithScripts(listing, catalog),
		gameservertest.WithDoors(changeDoors()...),
	)
	enterWorld(t, srv)
	return srv
}

// doorUpdateOrder returns, in order, the object id and closed flag of each
// DoorStatusUpdate among frames.
func doorUpdateOrder(frames [][]byte) [][2]int32 {
	var out [][2]int32
	for _, f := range framesOf(frames, serverpackets.OpcodeDoorStatusUpdate) {
		id := f.r.ReadInt32()
		out = append(out, [2]int32{id, f.r.ReadInt32()})
	}
	return out
}

func doorObjectID(t *testing.T, srv *gameservertest.Server, id int) int32 {
	t.Helper()
	obj, ok := srv.WorldObjects.Door(id)
	if !ok {
		t.Fatalf("door %d not spawned", id)
	}
	return obj.ObjectID()
}

// TestScriptDoorChangeHook pins Quest.addDoorChange and onDoorChange
// (Door.changeState): the hooks of the scripts watching a door run in list
// order on each change of its state, a script listing a door twice once,
// after the door's broadcast and before the door it triggers changes, so a
// door a hook opens is seen between the two. A door no script watches, a
// door id the world lacks and a change to the state a door already has run
// nothing.
func TestScriptDoorChangeHook(t *testing.T) {
	t.Parallel()
	log := &doorChangeLog{}
	catalog := script.Catalog{
		"script.ai.DoorWatchFirst": func() script.Script {
			return script.Script{ChangedDoors: []int32{watchGateID, 19219999, watchGateID}, Hooks: script.Hooks{OnDoorChange: log.hook("first", nil)}}
		},
		"script.ai.DoorWatchSecond": func() script.Script {
			return script.Script{ChangedDoors: []int32{sideGateID, watchGateID}, Hooks: script.Hooks{OnDoorChange: log.hook("second", func(sc *script.Script, e script.DoorChange) {
				if e.Door.Name() != "watch_gate" {
					return
				}
				side := sc.Door("side_gate")
				if e.Door.Opened() {
					side.Open()
				} else {
					side.Close()
				}
			})}}
		},
	}
	srv := bootDoorChange(t, []string{"script.ai.DoorWatchFirst", "script.ai.DoorWatchSecond"}, catalog)
	lever, watch, side, far := doorObjectID(t, srv, leverDoorID), doorObjectID(t, srv, watchGateID), doorObjectID(t, srv, sideGateID), doorObjectID(t, srv, farGateID)
	srv.ReadQueued(t, srv.Client)

	step := func(name string, id int, open bool, wantUpdates [][2]int32, wantSeen []doorChangeSeen) {
		t.Helper()
		srv.WorldObjects.SetDoorOpen(id, open)
		if got := doorUpdateOrder(srv.ReadQueued(t, srv.Client)); !slices.Equal(got, wantUpdates) {
			t.Fatalf("%s: DoorStatusUpdate (object, closed) = %v, want %v", name, got, wantUpdates)
		}
		if got := log.take(); !slices.Equal(got, wantSeen) {
			t.Fatalf("%s: hooks = %+v, want %+v", name, got, wantSeen)
		}
	}
	step("lever opens", leverDoorID, true,
		[][2]int32{{lever, 0}, {watch, 0}, {side, 0}, {far, 0}},
		[]doorChangeSeen{{"first", "watch_gate", true}, {"second", "watch_gate", true}, {"second", "side_gate", true}})
	step("watched gate already open", watchGateID, true, nil, nil)
	step("far gate closes", farGateID, false, [][2]int32{{far, 1}}, nil)
	step("lever closes", leverDoorID, false,
		[][2]int32{{lever, 1}, {watch, 1}, {side, 1}},
		[]doorChangeSeen{{"first", "watch_gate", false}, {"second", "watch_gate", false}, {"second", "side_gate", false}})
	step("side gate opens", sideGateID, true, [][2]int32{{side, 0}}, []doorChangeSeen{{"second", "side_gate", true}})
}

// TestScriptDoorChangeDroppedByDoorReload: a door reload opens the old
// doors first, which the watching script sees once the reload is done; the
// new doors carry no registration, so their changes run no hook.
func TestScriptDoorChangeDroppedByDoorReload(t *testing.T) {
	t.Parallel()
	log := &doorChangeLog{}
	catalog := script.Catalog{
		"script.ai.DoorWatchFirst": func() script.Script {
			return script.Script{ChangedDoors: []int32{watchGateID}, Hooks: script.Hooks{OnDoorChange: log.hook("first", nil)}}
		},
	}
	srv := bootDoorChange(t, []string{"script.ai.DoorWatchFirst"}, catalog)
	srv.ReadQueued(t, srv.Client)

	table, err := door.NewTable(changeDoors())
	if err != nil {
		t.Fatalf("door table: %v", err)
	}
	if err := srv.WorldObjects.ReloadDoors(table); err != nil {
		t.Fatalf("reload doors: %v", err)
	}
	if got, want := log.take(), []doorChangeSeen{{"first", "watch_gate", true}}; !slices.Equal(got, want) {
		t.Fatalf("hooks during the reload = %+v, want %+v", got, want)
	}
	srv.WorldObjects.SetDoorOpen(watchGateID, true)
	srv.WorldObjects.SetDoorOpen(watchGateID, false)
	if got := log.take(); got != nil {
		t.Fatalf("hooks on the reloaded door = %+v, want none", got)
	}
}
