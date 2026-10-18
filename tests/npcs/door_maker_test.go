package npcs

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The maker door event (#3809): a maker finds a door by name and registers
// its npcmaker on it; each later opening or closing of the door runs the
// door event hook of every registered maker, in registration order, after
// the hooks of the scripts watching the door. Registering an npcmaker
// again moves it last.

const (
	makerGateID     = 19210021
	doorWatchNpcID  = 21511
	doorWatchScript = "script.ai.MakerGateWatch"
)

const doorWatchSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="gate_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="gate_first" territory="gate_field" maximumNpcs="1">
		<ai type="door_watch_maker"><set name="DoorName" val="MAKER_GATE"/></ai>
		<npc id="21511" total="1" pos="40;20;30;100" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="gate_second" territory="gate_field" maximumNpcs="1">
		<ai type="door_watch_maker"><set name="DoorName" val="maker_gate"/></ai>
		<npc id="21511" total="1" pos="80;20;30;100" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="gate_missing" territory="gate_field" maximumNpcs="1">
		<ai type="door_watch_maker"><set name="DoorName" val="no_such_gate"/></ai>
		<npc id="21511" total="1" pos="120;20;30;100" respawn="60sec"/>
	</npcmaker>
</list>`

// doorWatch is what the fixture makers and the script saw: each npcmaker's
// maker and group as it started, whether the door lookup found the door,
// and each door hook in the order the hooks ran.
type doorWatch struct {
	mu     sync.Mutex
	makers map[string]*script.Maker
	groups map[string]spawn.Group
	found  map[string]bool
	events []doorWatchEvent
}

// doorWatchEvent is one hook run: who ran it ("script" or the npcmaker's
// name), the door it was handed and the door's open state read in it.
type doorWatchEvent struct {
	who    string
	door   string
	opened bool
}

func (w *doorWatch) record(ev doorWatchEvent) {
	w.mu.Lock()
	w.events = append(w.events, ev)
	w.mu.Unlock()
}

func (w *doorWatch) take() []doorWatchEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := w.events
	w.events = nil
	return out
}

func (w *doorWatch) catalog() script.MakerCatalog {
	return script.MakerCatalog{"door_watch_maker": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(m *script.Maker, e script.MakerStart) {
				mk := e.Group.Maker()
				d := m.Door(mk.AIParams["DoorName"])
				w.mu.Lock()
				w.makers[mk.Name], w.groups[mk.Name], w.found[mk.Name] = m, e.Group, d != nil
				w.mu.Unlock()
				if d != nil {
					d.AddMakerEvent(m, e.Group)
				}
			},
			OnDoorEvent: func(_ *script.Maker, e script.MakerDoor) {
				w.record(doorWatchEvent{who: e.Group.Maker().Name, door: e.Door.Name(), opened: e.Door.Opened()})
			},
		}}
	}}
}

func (w *doorWatch) scripts() gameservertest.Option {
	return gameservertest.WithScripts([]script.Listing{{Path: doorWatchScript}}, script.Catalog{doorWatchScript: func() script.Script {
		return script.Script{ChangedDoors: []int32{makerGateID}, Hooks: script.Hooks{OnDoorChange: func(_ *script.Script, e script.DoorChange) {
			w.record(doorWatchEvent{who: "script", door: e.Door.Name(), opened: e.Door.Opened()})
		}}}
	}})
}

func makerGate() *door.Template {
	const x = 200
	return &door.Template{
		ID: makerGateID, Name: "maker_gate", Kind: door.KindDoor, Level: 1,
		Position:    location.Location{X: x, Y: 0, Z: 0},
		Coordinates: []location.Point{{X: x - 8, Y: -8}, {X: x + 8, Y: -8}, {X: x + 8, Y: 8}, {X: x - 8, Y: 8}},
		HP:          100, PDef: 10, MDef: 10, Height: 32,
	}
}

func TestMakerDoorEventFollowsTheDoor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gate.xml"), []byte(doorWatchSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	watch := &doorWatch{makers: map[string]*script.Maker{}, groups: map[string]spawn.Group{}, found: map[string]bool{}}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithDoors(makerGate()),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{spawnMonster(doorWatchNpcID, "Gate Watcher")})),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithMakers(watch.catalog()),
		watch.scripts(),
	)

	watch.mu.Lock()
	found := maps.Clone(watch.found)
	watch.mu.Unlock()
	if want := map[string]bool{"gate_first": true, "gate_second": true, "gate_missing": false}; !maps.Equal(found, want) {
		t.Fatalf("door found by name at start = %v, want %v", found, want)
	}

	check := func(step string, want []doorWatchEvent) {
		t.Helper()
		if got := watch.take(); !slices.Equal(got, want) {
			t.Fatalf("%s: door hooks = %+v, want %+v", step, got, want)
		}
	}
	all := func(opened bool, makers ...string) []doorWatchEvent {
		out := []doorWatchEvent{{who: "script", door: "maker_gate", opened: opened}}
		for _, m := range makers {
			out = append(out, doorWatchEvent{who: m, door: "maker_gate", opened: opened})
		}
		return out
	}

	w.srv.WorldObjects.SetDoorOpen(makerGateID, true)
	check("gate opened", all(true, "gate_first", "gate_second"))
	w.srv.WorldObjects.SetDoorOpen(makerGateID, true)
	check("gate already open", nil)

	// The first npcmaker registers again: it moves behind the second.
	watch.mu.Lock()
	m, g := watch.makers["gate_first"], watch.groups["gate_first"]
	watch.mu.Unlock()
	m.Door("maker_gate").AddMakerEvent(m, g)
	w.srv.WorldObjects.SetDoorOpen(makerGateID, false)
	check("gate closed", all(false, "gate_second", "gate_first"))
}
