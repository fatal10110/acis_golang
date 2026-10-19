package npcs

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Imperial Tomb hall makers of batch 2 (#213 PR 2): the first hall's
// warriors, whose loss closes the grave pathways and opens the wall doors,
// and the band masters behind those wall doors, whose loss opens the next
// pathways and clears the band npcmakers. Both are open-door makers,
// booted through the batch's catalog file with the doors they name.

// The tomb doors of the shipped datapack, by name, beside the player so
// the client sees their status.
const (
	gravePathway1 = 25150042
	gravePathway2 = 25150043
	gravePathway3 = 25150045
	gravePathway4 = 25150046
	wallDoorL1    = 25150066
	wallDoorL2    = 25150067
	wallDoorR5    = 25150061
)

func tombDoors() gameservertest.Option {
	gate := func(id int, name string, x int) *door.Template {
		return &door.Template{
			ID: id, Name: name, Kind: door.KindDoor, Level: 1,
			Position: location.Location{X: x, Y: 100, Z: gameservertest.SpawnZ},
			Coordinates: []location.Point{
				{X: x - 8, Y: 92}, {X: x + 8, Y: 92}, {X: x + 8, Y: 108}, {X: x - 8, Y: 108},
			},
			HP: 100, PDef: 10, MDef: 10, Height: 32,
		}
	}
	return gameservertest.WithDoors(
		gate(gravePathway1, "grave_pathway_1", -120),
		gate(gravePathway2, "grave_pathway_2", -80),
		gate(gravePathway3, "grave_pathway_3", -40),
		gate(gravePathway4, "grave_pathway_4", 0),
		gate(wallDoorL1, "wall_door_b_left_1", 40),
		gate(wallDoorL2, "wall_door_b_left_2", 80),
		gate(wallDoorR5, "wall_door_b_right_5", 120),
	)
}

// tombDoorStates reads whether each named door stands open, in order.
func tombDoorStates(t *testing.T, srv *gameservertest.Server, names ...string) []bool {
	t.Helper()
	out := make([]bool, len(names))
	for i, name := range names {
		d, ok := srv.WorldObjects.DoorByName(name)
		if !ok {
			t.Fatalf("door %s not in the world", name)
		}
		out[i] = d.Opened()
	}
	return out
}

const (
	weaknessWarriorID = 21701
	weaknessSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="tomb_hall" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="godard32_2515_16m1" territory="tomb_hall" maximumNpcs="4">
		<ai type="warrior_passive_weakness_maker">
			<set name="DoorName" val="grave_pathway_1"/>
			<set name="TDoorName3" val="GRAVE_PATHWAY_3"/>
			<set name="WallDoorNameB1" val="wall_door_b_left_1"/>
			<set name="WallDoorNameB10" val="wall_door_b_right_5"/>
		</ai>
		<npc id="21701" total="4" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
	weaknessTimerDelay = 500 * time.Millisecond
)

// WarriorPassiveWeaknessMaker: the open-door maker's group (nothing at
// start, the four warriors once its grave_pathway_1 opens). An NPC leaving
// arms no respawn. Half a second after the last warrior leaves, timer 1000
// closes grave_pathway_1 and grave_pathway_2 (TDoorName1 and TDoorName2
// undeclared) and the declared TDoorName3, leaves grave_pathway_4
// (TDoorName4 undeclared) as it is, and opens the two declared wall doors,
// in that order; the client sees each door change. Its own door closing
// resets the group: the door opening again brings the warriors back.
func TestWarriorPassiveWeaknessMakerOpensTheWallDoors(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		tombDoors(),
		gameservertest.WithNPCs(batch1Templates(weaknessWarriorID)),
		gameservertest.WithNpcSpawns(batch1Table(t, weaknessSpawnlist)),
		gameservertest.WithMakers(batch2Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, want int) {
		t.Helper()
		if got := len(mw.inWorld(weaknessWarriorID)); got != want {
			t.Fatalf("%s: warriors in the world = %d, want %d", step, got, want)
		}
	}
	doors := []string{"grave_pathway_1", "grave_pathway_2", "grave_pathway_3", "grave_pathway_4", "wall_door_b_left_1", "wall_door_b_left_2", "wall_door_b_right_5"}
	doorsAre := func(step string, want ...bool) {
		t.Helper()
		if got := tombDoorStates(t, w.srv, doors...); !slices.Equal(got, want) {
			t.Fatalf("%s: doors %v open = %v, want %v", step, doors, got, want)
		}
	}
	count("boot", 0)
	w.srv.Advance(t, time.Second)
	count("a second after boot", 0)
	drainFrames(t, w.c)

	w.srv.WorldObjects.SetDoorOpen(gravePathway1, true)
	w.srv.WorldObjects.SetDoorOpen(gravePathway2, true)
	w.srv.WorldObjects.SetDoorOpen(gravePathway3, true)
	w.srv.WorldObjects.SetDoorOpen(gravePathway4, true)
	w.srv.Settle(t)
	count("grave pathways opened", 4)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, repeatID(weaknessWarriorID, 4)) {
		t.Fatalf("grave pathways opened: NPCs shown = %v", got)
	}

	warriors := mw.inWorld(weaknessWarriorID)
	for i, h := range warriors[:3] {
		h.DeleteMe()
		w.srv.Settle(t)
		count(fmt.Sprintf("warrior %d left", i+1), 3-i)
	}
	w.srv.Advance(t, weaknessTimerDelay)
	doorsAre("three warriors left", true, true, true, true, false, false, false)
	for i := range 4 {
		if slot := fmt.Sprintf("godard32_2515_16m1#0#%d", i); w.srv.NpcRespawns.Tracked(slot) {
			t.Fatalf("three warriors left: slot %s armed a respawn", slot)
		}
	}
	drainFrames(t, w.c)

	warriors[3].DeleteMe()
	w.srv.Settle(t)
	count("the last warrior left", 0)
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, []int32{warriors[3].ObjectID()}) {
		t.Fatalf("the last warrior left: deleted objects = %v", got)
	}
	w.srv.Advance(t, weaknessTimerDelay-time.Millisecond)
	doorsAre("before timer 1000", true, true, true, true, false, false, false)
	w.srv.Advance(t, time.Millisecond)
	doorsAre("timer 1000", false, false, false, true, true, false, true)
	ids, open, _ := gateStates(drainFrames(t, w.c))
	if want := []int32{gravePathway1, gravePathway2, gravePathway3, wallDoorL1, wallDoorR5}; !slices.Equal(ids, want) {
		t.Fatalf("timer 1000: door updates = %v, want %v", ids, want)
	}
	if want := []bool{false, false, false, true, true}; !slices.Equal(open, want) {
		t.Fatalf("timer 1000: door updates open = %v, want %v", open, want)
	}

	w.srv.WorldObjects.SetDoorOpen(gravePathway1, true)
	w.srv.Settle(t)
	count("grave_pathway_1 opened again", 4)
}

const (
	bandmasterFirstID = 21710 // the eight spawns carry 21710 to 21717
	bandRecorderID    = 21720
	bandListenerKey   = "quest.BandmasterListener"
)

// bandmasterSpawnlist is the band master npcmaker with eight one-NPC
// spawns, and the seven band npcmakers it wakes, in an order of their own.
var bandmasterSpawnlist = func() string {
	s := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="tomb_hall" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="godard32_2515_17m1" territory="tomb_hall" maximumNpcs="8">
		<ai type="undead_bandmaster_maker">
			<set name="DoorName" val="wall_door_b_left_1"/>
			<set name="TDoorName3" val="grave_pathway_3"/>
			<set name="TDoorName4" val="GRAVE_PATHWAY_4"/>
		</ai>
`
	for i := range 8 {
		s += fmt.Sprintf("\t\t<npc id=\"%d\" total=\"1\" pos=\"%d;20;30;0\" respawn=\"60sec\"/>\n", bandmasterFirstID+i, 40+10*i)
	}
	s += "\t</npcmaker>\n"
	for _, n := range []int{10, 11, 12, 13, 14, 15, 18} {
		s += fmt.Sprintf("\t<npcmaker name=\"godard32_2515_%dm1\" territory=\"tomb_hall\" maximumNpcs=\"1\">\n\t\t<ai type=\"band_recorder_maker\"/>\n\t\t<npc id=\"%d\" total=\"1\" pos=\"200;20;30;0\" respawn=\"no\"/>\n\t</npcmaker>\n", n, bandRecorderID)
	}
	return s + "</list>"
}()

// bandEvent is one maker script event a band npcmaker ran, with whether
// grave_pathway_3 and grave_pathway_4 stood open when it ran.
type bandEvent struct {
	maker      string
	name       string
	int1, int2 int
	pathways   [2]bool
}

// bandLog records the maker script events of the band npcmakers and the
// script events the band masters hear, in order.
type bandLog struct {
	mu     sync.Mutex
	events []bandEvent
	heard  []alarmReceipt
}

func (l *bandLog) catalog() script.MakerCatalog {
	out := batch2Makers()
	out["band_recorder_maker"] = func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
				ev := bandEvent{maker: e.Group.Maker().Name, name: e.Name, int1: e.Int1, int2: e.Int2}
				ev.pathways = [2]bool{m.Door("grave_pathway_3").Opened(), m.Door("grave_pathway_4").Opened()}
				l.mu.Lock()
				l.events = append(l.events, ev)
				l.mu.Unlock()
			},
		}}
	}
	return out
}

func (l *bandLog) script() script.Script {
	var ids []int32
	for i := range 8 {
		ids = append(ids, int32(bandmasterFirstID+i))
	}
	return script.Script{
		Bind: script.Bindings{script.EventScriptEvent: ids},
		Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				l.mu.Lock()
				l.heard = append(l.heard, alarmReceipt{npcID: e.NPC.NpcID(), decayed: e.NPC.Decayed(), event: e.EventID, arg1: e.Arg1})
				l.mu.Unlock()
			},
		},
	}
}

func (l *bandLog) take() ([]bandEvent, []alarmReceipt) {
	l.mu.Lock()
	defer l.mu.Unlock()
	events, heard := l.events, l.heard
	l.events, l.heard = nil, nil
	slices.SortFunc(heard, func(a, b alarmReceipt) int { return int(a.npcID - b.npcID) })
	return events, heard
}

// UndeadBandmasterMaker: nothing spawns at start. The wall door opening a
// second later brings the eight band masters. Timer 1000 sends 10032 with
// the spawn's index to the NPCs of spawns 0, 2, 4 and 6: once three
// seconds after start, then once for each band master three seconds after
// it came. The door closing deletes them; opening it again brings them back.
// An NPC leaving arms no respawn, and nothing happens until the last one
// leaves: then grave_pathway_3 and grave_pathway_4 open (the client sees
// both) and, with both open, the seven band npcmakers run maker script
// event 1000 with int2 1, in the order 14, 11, 10, 12, 13, 15, 18.
func TestUndeadBandmasterMakerOpensTheNextPathways(t *testing.T) {
	t.Parallel()
	log := &bandLog{}
	ids := make([]int, 8)
	kinds := map[int32]script.NPCKind{}
	for i := range ids {
		ids[i] = bandmasterFirstID + i
		kinds[int32(ids[i])] = script.KindHostile
	}
	w := bootFolkWorld(t, map[string]string{},
		tombDoors(),
		gameservertest.WithNPCs(batch1Templates(append(slices.Clone(ids), bandRecorderID)...)),
		gameservertest.WithNpcSpawns(batch1Table(t, bandmasterSpawnlist)),
		gameservertest.WithMakers(log.catalog()),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: bandListenerKey}}, script.Catalog{bandListenerKey: log.script}),
	)
	mw := &makerWorld{srv: w.srv}
	inWorld := func() []int {
		var out []int
		for _, id := range ids {
			out = append(out, repeatID(id, len(mw.inWorld(id)))...)
		}
		return out
	}
	expect := func(step string, wantEvents []bandEvent, wantHeard []alarmReceipt) {
		t.Helper()
		events, heard := log.take()
		if !slices.Equal(events, wantEvents) {
			t.Fatalf("%s: band maker events = %+v, want %+v", step, events, wantEvents)
		}
		if !slices.Equal(heard, wantHeard) {
			t.Fatalf("%s: band masters heard %v, want %v", step, heard, wantHeard)
		}
	}
	if got := inWorld(); len(got) != 0 {
		t.Fatalf("boot: band masters in the world = %v", got)
	}
	w.srv.Advance(t, time.Second)
	expect("a second after boot", nil, nil)
	drainFrames(t, w.c)

	w.srv.WorldObjects.SetDoorOpen(wallDoorL1, true)
	w.srv.Settle(t)
	if got := inWorld(); !slices.Equal(got, ids) {
		t.Fatalf("wall door opened: band masters in the world = %v, want %v", got, ids)
	}
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, ids) {
		t.Fatalf("wall door opened: NPCs shown = %v, want %v", got, ids)
	}
	timerHeard := func(times int) []alarmReceipt {
		var out []alarmReceipt
		for _, i := range []int{0, 2, 4, 6} {
			for range times {
				out = append(out, alarmReceipt{npcID: int32(bandmasterFirstID + i), event: 10032, arg1: int32(i)})
			}
		}
		return out
	}
	w.srv.Advance(t, 2*time.Second-time.Millisecond)
	expect("before the start timer", nil, nil)
	w.srv.Advance(t, time.Millisecond)
	expect("the start timer", nil, timerHeard(1))
	w.srv.Advance(t, time.Second-time.Millisecond)
	expect("before the spawn timers", nil, nil)
	w.srv.Advance(t, time.Millisecond)
	expect("the spawn timers", nil, timerHeard(8))

	masters := mw.inWorld(bandmasterFirstID)
	w.srv.WorldObjects.SetDoorOpen(wallDoorL1, false)
	w.srv.Settle(t)
	if got := inWorld(); len(got) != 0 {
		t.Fatalf("wall door closed: band masters in the world = %v", got)
	}
	if got := deleted(drainFrames(t, w.c)); len(got) != 8 || !slices.Contains(got, masters[0].ObjectID()) {
		t.Fatalf("wall door closed: deleted objects = %v, want the eight band masters", got)
	}
	expect("wall door closed", nil, nil)

	w.srv.WorldObjects.SetDoorOpen(wallDoorL1, true)
	w.srv.Settle(t)
	if got := inWorld(); !slices.Equal(got, ids) {
		t.Fatalf("wall door opened again: band masters in the world = %v, want %v", got, ids)
	}
	w.srv.Advance(t, 3*time.Second)
	log.take()
	drainFrames(t, w.c)

	var all []int32
	for _, id := range ids {
		for _, h := range mw.inWorld(id) {
			all = append(all, h.ObjectID())
			h.DeleteMe()
			w.srv.Settle(t)
			if len(all) < 8 {
				expect(fmt.Sprintf("%d band masters left", len(all)), nil, nil)
				if got := tombDoorStates(t, w.srv, "grave_pathway_3", "grave_pathway_4"); slices.Contains(got, true) {
					t.Fatalf("%d band masters left: grave pathways open = %v", len(all), got)
				}
			}
		}
	}
	for i := range 8 {
		if slot := fmt.Sprintf("godard32_2515_17m1#%d#0", i); w.srv.NpcRespawns.Tracked(slot) {
			t.Fatalf("slot %s armed a respawn", slot)
		}
	}
	var want []bandEvent
	for _, n := range []int{14, 11, 10, 12, 13, 15, 18} {
		want = append(want, bandEvent{maker: fmt.Sprintf("godard32_2515_%dm1", n), name: "1000", int2: 1, pathways: [2]bool{true, true}})
	}
	expect("the last band master left", want, nil)
	frames := drainFrames(t, w.c)
	if got := deleted(frames); len(got) != 8 {
		t.Fatalf("band masters left: deleted objects = %v, want eight", got)
	}
	doorIDs, open, _ := gateStates(frames)
	if want := []int32{gravePathway3, gravePathway4}; !slices.Equal(doorIDs, want) || !slices.Equal(open, []bool{true, true}) {
		t.Fatalf("the last band master left: door updates = %v open %v, want %v both open", doorIDs, open, want)
	}
}
