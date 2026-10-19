package npcs

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The first set of batch 3 makers (#214): the Freya garden, Sailren lair
// and Frintezza tomb makers, booted through the batch's catalog file. The
// Benom maker's scenarios are in makers_benom_test.go.

// batch3Makers is the batch's catalog file without the types not ported
// yet, as the composition root joins it, plus extra fixture makers.
func batch3Makers(extra script.MakerCatalog) script.MakerCatalog {
	out := script.MakerCatalog{}
	for typ, ctor := range catalog.Makers()["makers_3"] {
		if ctor != nil {
			out[typ] = ctor
		}
	}
	for typ, ctor := range extra {
		out[typ] = ctor
	}
	return out
}

// heardEvent is one script event an NPC heard.
type heardEvent struct {
	npcID      int32
	decayed    bool
	event      int32
	arg1, arg2 int32
}

func (h heardEvent) String() string {
	return fmt.Sprintf("%d:%d(%d,%d) decayed=%v", h.npcID, h.event, h.arg1, h.arg2, h.decayed)
}

// heardLog records the script events the NPCs of ids hear, in order.
type heardLog struct {
	ids      []int32
	mu       sync.Mutex
	received []heardEvent
}

const heardLogKey = "quest.HeardLog"

func (l *heardLog) script() script.Script {
	return script.Script{
		Bind: script.Bindings{script.EventScriptEvent: l.ids},
		Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				l.mu.Lock()
				l.received = append(l.received, heardEvent{npcID: e.NPC.NpcID(), decayed: e.NPC.Decayed(), event: e.EventID, arg1: e.Arg1, arg2: e.Arg2})
				l.mu.Unlock()
			},
		},
	}
}

func (l *heardLog) take() []heardEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.received
	l.received = nil
	return out
}

// option binds the log to its NPCs, as hostiles.
func (l *heardLog) option() gameservertest.Option {
	kinds := map[int32]script.NPCKind{}
	for _, id := range l.ids {
		kinds[id] = script.KindHostile
	}
	return gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: heardLogKey}}, script.Catalog{heardLogKey: l.script})
}

func (l *heardLog) expect(t *testing.T, step string, want ...heardEvent) {
	t.Helper()
	if got := l.take(); !slices.Equal(got, want) {
		t.Fatalf("%s: script events heard = %v, want %v", step, got, want)
	}
}

// makerEventLog is a fixture maker type recording the maker script
// events its npcmakers run, by npcmaker name.
type makerEventLog struct {
	mu   sync.Mutex
	runs []string
}

func (l *makerEventLog) catalog() script.MakerCatalog {
	return script.MakerCatalog{"maker_event_log": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
				l.mu.Lock()
				l.runs = append(l.runs, fmt.Sprintf("%s:%s(%d,%d)", e.Group.Maker().Name, e.Name, e.Int1, e.Int2))
				l.mu.Unlock()
			},
		}}
	}}
}

func (l *makerEventLog) expect(t *testing.T, step string, want ...string) {
	t.Helper()
	l.mu.Lock()
	got := l.runs
	l.runs = nil
	l.mu.Unlock()
	if !slices.Equal(got, want) {
		t.Fatalf("%s: maker script events run = %q, want %q", step, got, want)
	}
}

const (
	deaconID          = 21701
	gardenSculptureID = 21702
	fairySculptureID  = 21703
	freyaSpawnlist    = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="garden_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="garden_deacon" territory="garden_field" maximumNpcs="1">
		<ai type="freya_deacon_keeper_maker"/>
		<npc id="21701" total="1" pos="40;20;30;0" respawn="1min"/>
	</npcmaker>
	<npcmaker name="garden_sculpture" territory="garden_field" maximumNpcs="1">
		<ai type="sculpture_garden_maker"/>
		<npc id="21702" total="1" pos="50;20;30;0" respawn="1min"/>
	</npcmaker>
	<npcmaker name="garden_fairy" territory="garden_field" maximumNpcs="1">
		<ai type="sculpture_ice_fairy_maker"/>
		<npc id="21703" total="1" pos="60;20;30;0" respawn="1min"/>
	</npcmaker>
</list>`
)

// FreyaDeaconKeeperMaker, SculptureGardenMaker and SculptureIceFairyMaker:
// each spawns its NPC at start as the default maker does and passes maker
// script events on to it. The steward hears 10026 for 10025, 10005 for
// 10005 and 11037 for 11037; the garden sculpture hears 11038 for 10005;
// the ice fairy sculpture hears 10001 with its count, which 10005 raises
// by one and 10025 sets back to 0. None answers the default maker's 1000
// or 1001, and none answers another maker's events. An NPC leaving arms
// its respawn as the default maker does.
func TestFreyaGardenMakersPassEventsOn(t *testing.T) {
	t.Parallel()
	log := &heardLog{ids: []int32{deaconID, gardenSculptureID, fairySculptureID}}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(deaconID, gardenSculptureID, fairySculptureID)),
		gameservertest.WithNpcSpawns(batch1Table(t, freyaSpawnlist)),
		gameservertest.WithMakers(batch3Makers(nil)),
		log.option(),
	)
	mw := &makerWorld{srv: w.srv}
	present := func(step string, want ...int) {
		t.Helper()
		var got []int
		for _, id := range []int{deaconID, gardenSculptureID, fairySculptureID} {
			for range mw.inWorld(id) {
				got = append(got, id)
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%s: NPCs in the world = %v, want %v", step, got, want)
		}
	}
	present("boot", deaconID, gardenSculptureID, fairySculptureID)
	drainFrames(t, w.c)
	send := func(maker, name string) {
		t.Helper()
		if !w.srv.NpcSpawns.MakerEvent(maker, name, 7, 9) {
			t.Fatalf("npcmaker %s not found by name", maker)
		}
		w.srv.Settle(t)
	}
	heard := func(id, event, arg1 int32) heardEvent { return heardEvent{npcID: id, event: event, arg1: arg1} }

	send("garden_deacon", "10025")
	send("garden_deacon", "10005")
	send("garden_deacon", "11037")
	send("garden_deacon", "11038")
	log.expect(t, "steward", heard(deaconID, 10026, 0), heard(deaconID, 10005, 0), heard(deaconID, 11037, 0))

	send("garden_sculpture", "10005")
	send("garden_sculpture", "10025")
	log.expect(t, "garden sculpture", heard(gardenSculptureID, 11038, 0))

	send("garden_fairy", "10005")
	send("garden_fairy", "10005")
	send("garden_fairy", "10025")
	send("garden_fairy", "10005")
	send("garden_fairy", "11037")
	log.expect(t, "ice fairy sculpture",
		heard(fairySculptureID, 10001, 1), heard(fairySculptureID, 10001, 2),
		heard(fairySculptureID, 10001, 0), heard(fairySculptureID, 10001, 1))

	for _, maker := range []string{"garden_deacon", "garden_sculpture", "garden_fairy"} {
		send(maker, "1000")
		send(maker, "1001")
	}
	w.srv.Advance(t, 10*time.Second)
	present("1000 and 1001", deaconID, gardenSculptureID, fairySculptureID)
	if frames := drainFrames(t, w.c); len(shown(frames)) != 0 || len(deleted(frames)) != 0 {
		t.Fatalf("1000 and 1001: shown %v and deleted %v, want none", shown(frames), deleted(frames))
	}
	log.expect(t, "1000 and 1001")

	mw.inWorld(fairySculptureID)[0].DeleteMe()
	w.srv.Settle(t)
	if !fireRespawn(w.srv, "garden_fairy#0#0") {
		t.Fatal("the ice fairy sculpture leaving armed no respawn")
	}
	w.srv.Settle(t)
	present("respawn", deaconID, gardenSculptureID, fairySculptureID)
}

const (
	sailrenDummyID   = 21711
	shilenStatueID   = 21712
	sailrenSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="lair_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="lair_dummy" territory="lair_field" maximumNpcs="2">
		<ai type="sailren_dummy_maker"/>
		<npc id="21711" total="1" pos="40;20;30;0" respawn="1min"/>
	</npcmaker>
	<npcmaker name="lair_statue" territory="lair_field" maximumNpcs="1">
		<ai type="statue_of_shilen_maker"/>
		<npc id="21712" total="1" pos="50;20;30;0" respawn="1min"/>
	</npcmaker>
</list>`
)

// SailrenDummyMaker and StatueOfShilenMaker: each spawns its NPC at start
// and sends it the script event a maker script event of its list names,
// with 1 and 0, matched exactly: 11046, 11047 and 11048 for the dummy;
// 11041, 11043, 11045, 11047 and 11050 for the statue. Every maker script
// event then runs the default maker's: 1000 deletes the group, and 1001
// brings it back after int1 seconds.
func TestSailrenLairMakersPassTheirEventsOn(t *testing.T) {
	t.Parallel()
	log := &heardLog{ids: []int32{sailrenDummyID, shilenStatueID}}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(sailrenDummyID, shilenStatueID)),
		gameservertest.WithNpcSpawns(batch1Table(t, sailrenSpawnlist)),
		gameservertest.WithMakers(batch3Makers(nil)),
		log.option(),
	)
	mw := &makerWorld{srv: w.srv}
	if got := [2]int{len(mw.inWorld(sailrenDummyID)), len(mw.inWorld(shilenStatueID))}; got != [2]int{1, 1} {
		t.Fatalf("boot: (dummy, statue) in the world = %v, want [1 1]", got)
	}
	drainFrames(t, w.c)
	heard := func(id, event int32) heardEvent { return heardEvent{npcID: id, event: event, arg1: 1} }
	sendAll := func(maker string, names ...string) {
		t.Helper()
		for _, name := range names {
			if !w.srv.NpcSpawns.MakerEvent(maker, name, 0, 0) {
				t.Fatalf("npcmaker %s not found by name", maker)
			}
		}
		w.srv.Settle(t)
	}

	sendAll("lair_dummy", "11046", "11047", "11048", "11041", "11050", "11049")
	log.expect(t, "dummy", heard(sailrenDummyID, 11046), heard(sailrenDummyID, 11047), heard(sailrenDummyID, 11048))
	sendAll("lair_statue", "11041", "11043", "11045", "11047", "11050", "11046", "11042")
	log.expect(t, "statue",
		heard(shilenStatueID, 11041), heard(shilenStatueID, 11043), heard(shilenStatueID, 11045),
		heard(shilenStatueID, 11047), heard(shilenStatueID, 11050))

	statue := mw.inWorld(shilenStatueID)[0]
	sendAll("lair_statue", "1000")
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, []int32{statue.ObjectID()}) {
		t.Fatalf("statue 1000: deleted objects = %v, want [%d]", got, statue.ObjectID())
	}
	sendAll("lair_statue", "1001")
	w.srv.Advance(t, time.Millisecond)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{shilenStatueID}) {
		t.Fatalf("statue 1001: NPCs shown = %v, want [%d]", got, shilenStatueID)
	}

	dummy := mw.inWorld(sailrenDummyID)[0]
	sendAll("lair_dummy", "1000")
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, []int32{dummy.ObjectID()}) {
		t.Fatalf("dummy 1000: deleted objects = %v, want [%d]", got, dummy.ObjectID())
	}
	sendAll("lair_dummy", "11046")
	log.expect(t, "dummy deleted")
}

const (
	evilSpiritAID    = 21721
	evilSpiritBID    = 21722
	evilateSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="tomb_hall" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="tomb_evilate" territory="tomb_hall" maximumNpcs="1">
		<ai type="frintessa_evilate_maker"><set name="on_start_spawn" val="0"/></ai>
		<npc id="21721" total="1" pos="40;20;30;0" respawn="0sec"/>
		<npc id="21722" total="1" pos="50;20;30;0" respawn="1min"/>
	</npcmaker>
</list>`
	evilateSpiritSlotA = "tomb_evilate#0#0"
	evilateSpiritSlotB = "tomb_evilate#1#0"
)

// FrintezzaEvilateMaker: an npcmaker that does not start on its own
// places nothing at boot. 1001 fills every spawn to its own total after
// int1 seconds, past the maximum of one. A spirit with no respawn delay
// leaving is gone from its spawn; one with a delay arms its respawn as the
// default maker does. A later 1001 brings a new spirit for the first
// after int1 seconds and arms the second's respawn, adding none for its
// spawn. 1000 deletes the group.
func TestFrintezzaEvilateMakerFillsPastItsMaximum(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(evilSpiritAID, evilSpiritBID)),
		gameservertest.WithNpcSpawns(batch1Table(t, evilateSpawnlist)),
		gameservertest.WithMakers(batch3Makers(nil)),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, a, b int) {
		t.Helper()
		if got := [2]int{len(mw.inWorld(evilSpiritAID)), len(mw.inWorld(evilSpiritBID))}; got != [2]int{a, b} {
			t.Fatalf("%s: evil spirits (a, b) in the world = %v, want [%d %d]", step, got, a, b)
		}
	}
	count("boot", 0, 0)
	drainFrames(t, w.c)

	if !w.srv.NpcSpawns.MakerEvent("tomb_evilate", "1001", 2, 0) {
		t.Fatal("npcmaker tomb_evilate not found by name")
	}
	w.srv.Advance(t, 2*time.Second-time.Millisecond)
	count("before int1 seconds", 0, 0)
	w.srv.Advance(t, time.Millisecond)
	count("1001", 1, 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{evilSpiritAID, evilSpiritBID}) {
		t.Fatalf("1001: NPCs shown = %v", got)
	}

	mw.inWorld(evilSpiritAID)[0].DeleteMe()
	mw.inWorld(evilSpiritBID)[0].DeleteMe()
	w.srv.Settle(t)
	count("both spirits left", 0, 0)
	if w.srv.NpcRespawns.Tracked(evilateSpiritSlotA) {
		t.Fatal("the spirit with no respawn delay armed a respawn")
	}
	if !w.srv.NpcRespawns.Tracked(evilateSpiritSlotB) {
		t.Fatal("the spirit with a respawn delay armed no respawn")
	}
	drainFrames(t, w.c)

	// The fixture's respawn timers never fire on their own: the second
	// spirit comes back when its rearmed respawn is fired.
	w.srv.NpcSpawns.MakerEvent("tomb_evilate", "1001", 3, 0)
	w.srv.Advance(t, 3*time.Second-time.Millisecond)
	count("1001 again, before int1 seconds", 0, 0)
	w.srv.Advance(t, time.Millisecond)
	count("1001 again", 1, 0)
	if !fireRespawn(w.srv, evilateSpiritSlotB) {
		t.Fatal("1001 again left the second spirit no respawn")
	}
	w.srv.Settle(t)
	count("the second spirit's respawn", 1, 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{evilSpiritAID, evilSpiritBID}) {
		t.Fatalf("1001 again: NPCs shown = %v", got)
	}

	w.srv.NpcSpawns.MakerEvent("tomb_evilate", "1000", 0, 0)
	w.srv.Settle(t)
	count("1000", 0, 0)
	if got := deleted(drainFrames(t, w.c)); len(got) != 2 {
		t.Fatalf("1000: deleted objects = %v, want two", got)
	}
}

const (
	velociraptorID       = 21731
	velociraptorSpawnlst = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="raptor_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="rune20_mb2017_01m1" territory="raptor_field" maximumNpcs="2">
		<ai type="velociraptor_maker"/>
		<npc id="21731" total="1" pos="40;20;30;0" respawn="0sec"/>
		<npc id="21731" total="2" pos="50;20;30;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="rune20_mb2017_02m1" territory="raptor_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
	<npcmaker name="rune20_mb2017_03m1" territory="raptor_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
	<npcmaker name="rune16_npc2017_01m1" territory="raptor_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
</list>`
)

// VelociraptorMaker: nothing spawns at start. 1001 brings the group up to
// its maximum after int1 seconds, and each NPC brought arms timer 1001
// five seconds out, which sends every NPC of the group 11049 and arms
// timer 78001 five hours out. With the flag down, the first 78001 runs
// 11052 and 1000 on rune20_mb2017_01m1 (the maker itself: its flag goes
// up and its group is deleted), 02m1 and 03m1, then 11050 on
// rune16_npc2017_01m1; the second finds the flag up and does nothing.
// 11053 lowers the flag. The group's last NPC leaving with the flag down
// runs 1001 on rune20_mb2017_02m1; with the flag up it runs nothing. Its
// NPCs never respawn on their own.
func TestVelociraptorMakerWakesTheLair(t *testing.T) {
	t.Parallel()
	log := &heardLog{ids: []int32{velociraptorID}}
	makers := &makerEventLog{}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(velociraptorID)),
		gameservertest.WithNpcSpawns(batch1Table(t, velociraptorSpawnlst)),
		gameservertest.WithMakers(batch3Makers(makers.catalog())),
		log.option(),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, want int) {
		t.Helper()
		if got := len(mw.inWorld(velociraptorID)); got != want {
			t.Fatalf("%s: velociraptors in the world = %d, want %d", step, got, want)
		}
	}
	send := func(name string, int1 int) {
		t.Helper()
		if !w.srv.NpcSpawns.MakerEvent("rune20_mb2017_01m1", name, int1, 0) {
			t.Fatal("npcmaker rune20_mb2017_01m1 not found by name")
		}
		w.srv.Settle(t)
	}
	heard11049 := heardEvent{npcID: velociraptorID, event: 11049}
	count("boot", 0)
	makers.expect(t, "boot")
	drainFrames(t, w.c)

	send("1001", 1)
	count("1001, before int1 seconds", 0)
	w.srv.Advance(t, time.Second)
	count("1001", 2)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, repeatID(velociraptorID, 2)) {
		t.Fatalf("1001: NPCs shown = %v", got)
	}
	send("1001", 0)
	w.srv.Advance(t, time.Millisecond)
	count("1001 at the maximum", 2)
	log.expect(t, "before timer 1001")
	w.srv.Advance(t, 4*time.Second-time.Millisecond)
	log.expect(t, "timer 1001", heard11049, heard11049, heard11049, heard11049)
	makers.expect(t, "timer 1001")

	w.srv.Advance(t, 18000*time.Second-time.Millisecond)
	makers.expect(t, "before timer 78001")
	count("before timer 78001", 2)
	w.srv.Advance(t, time.Millisecond)
	makers.expect(t, "timer 78001",
		"rune20_mb2017_02m1:11052(0,0)", "rune20_mb2017_02m1:1000(0,0)",
		"rune20_mb2017_03m1:11052(0,0)", "rune20_mb2017_03m1:1000(0,0)",
		"rune16_npc2017_01m1:11050(0,0)")
	count("timer 78001", 0)
	if got := deleted(drainFrames(t, w.c)); len(got) != 2 {
		t.Fatalf("timer 78001: deleted objects = %v, want two", got)
	}

	send("1001", 0)
	w.srv.Advance(t, time.Millisecond)
	count("1001 with the flag up", 2)
	raptors := mw.inWorld(velociraptorID)
	raptors[0].DeleteMe()
	raptors[1].DeleteMe()
	w.srv.Settle(t)
	count("the group left with the flag up", 0)
	makers.expect(t, "the group left with the flag up")

	send("11053", 0)
	send("1001", 0)
	w.srv.Advance(t, time.Millisecond)
	count("1001 with the flag down", 2)
	raptors = mw.inWorld(velociraptorID)
	raptors[0].DeleteMe()
	w.srv.Settle(t)
	makers.expect(t, "one velociraptor left")
	raptors[1].DeleteMe()
	w.srv.Settle(t)
	makers.expect(t, "the group left with the flag down", "rune20_mb2017_02m1:1001(0,0)")
	for _, slot := range []string{"rune20_mb2017_01m1#0#0", "rune20_mb2017_01m1#1#0", "rune20_mb2017_01m1#1#1"} {
		if w.srv.NpcRespawns.Tracked(slot) {
			t.Fatalf("slot %s armed a respawn", slot)
		}
	}
}
