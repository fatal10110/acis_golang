package npcs

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The second set of batch 3 makers (#214): the last two dinosaur makers
// of Sailren's lair, the maker of Frintezza and his followers and the
// maker of Ice Fairy Sirra, booted through the batch's catalog file.

const (
	rhamphorhynchusID  = 21741
	rhamphorhynchusLst = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="raptor_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="rune20_mb2017_02m1" territory="raptor_field" maximumNpcs="1">
		<ai type="rhamphorhynchus_maker"/>
		<npc id="21741" total="1" pos="40;20;30;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="rune20_mb2017_03m1" territory="raptor_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
</list>`
)

// RhamphorhynchusMaker: nothing spawns at start. Velociraptor's 1001
// brings the NPC in after int1 seconds and its timer 1001 sends it 11049
// five seconds later. The NPC leaving with the flag down runs 1001 on
// rune20_mb2017_03m1; once 11052 raises the flag it runs nothing. Its NPC
// never respawns on its own.
func TestRhamphorhynchusMakerWakesTheTyrannosaurus(t *testing.T) {
	t.Parallel()
	log := &heardLog{ids: []int32{rhamphorhynchusID}}
	makers := &makerEventLog{}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(rhamphorhynchusID)),
		gameservertest.WithNpcSpawns(batch1Table(t, rhamphorhynchusLst)),
		gameservertest.WithMakers(batch3Makers(makers.catalog())),
		log.option(),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, want int) {
		t.Helper()
		if got := len(mw.inWorld(rhamphorhynchusID)); got != want {
			t.Fatalf("%s: rhamphorhynchi in the world = %d, want %d", step, got, want)
		}
	}
	send := func(name string, int1 int) {
		t.Helper()
		if !w.srv.NpcSpawns.MakerEvent("rune20_mb2017_02m1", name, int1, 0) {
			t.Fatal("npcmaker rune20_mb2017_02m1 not found by name")
		}
		w.srv.Settle(t)
	}
	count("boot", 0)
	drainFrames(t, w.c)

	send("1001", 1)
	count("1001, before int1 seconds", 0)
	w.srv.Advance(t, time.Second)
	count("1001", 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{rhamphorhynchusID}) {
		t.Fatalf("1001: NPCs shown = %v, want [%d]", got, rhamphorhynchusID)
	}
	w.srv.Advance(t, 4*time.Second)
	log.expect(t, "timer 1001", heardEvent{npcID: rhamphorhynchusID, event: 11049})

	mw.inWorld(rhamphorhynchusID)[0].DeleteMe()
	w.srv.Settle(t)
	makers.expect(t, "the NPC left with the flag down", "rune20_mb2017_03m1:1001(0,0)")
	if w.srv.NpcRespawns.Tracked("rune20_mb2017_02m1#0#0") {
		t.Fatal("the NPC armed a respawn of its own")
	}

	send("11052", 0)
	send("1001", 0)
	w.srv.Advance(t, time.Millisecond)
	count("1001 with the flag up", 1)
	mw.inWorld(rhamphorhynchusID)[0].DeleteMe()
	w.srv.Settle(t)
	makers.expect(t, "the NPC left with the flag up")
}

const (
	tyrannosaurusID  = 21742
	tyrannosaurusLst = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="raptor_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="rune20_mb2017_03m1" territory="raptor_field" maximumNpcs="1">
		<ai type="tyrannosaurus_maker"/>
		<npc id="21742" total="1" pos="40;20;30;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="rune20_mb2017_01m1" territory="raptor_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
	<npcmaker name="rune20_mb2017_02m1" territory="raptor_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
	<npcmaker name="rune20_mb2017_04m1" territory="raptor_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
	<npcmaker name="rune16_npc2017_01m1" territory="raptor_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
</list>`
)

// TyrannosaurusMaker: nothing spawns at start. Velociraptor's 1001 brings
// the NPC in and its timer 1001 sends it 11049 five seconds later. The
// NPC leaving arms timer 1002, which three minutes later runs 11042 on
// rune20_mb2017_04m1. Every other timer is Velociraptor's: timer 78001,
// five hours after timer 1001 with the flag down, runs 11052 and 1000 on
// rune20_mb2017_01m1, 02m1 and 03m1 (the maker itself: its flag goes up)
// and 11050 on rune16_npc2017_01m1. With the flag up the NPC leaving
// still arms timer 1002. Its NPC never respawns on its own.
func TestTyrannosaurusMakerCallsSailren(t *testing.T) {
	t.Parallel()
	log := &heardLog{ids: []int32{tyrannosaurusID}}
	makers := &makerEventLog{}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(tyrannosaurusID)),
		gameservertest.WithNpcSpawns(batch1Table(t, tyrannosaurusLst)),
		gameservertest.WithMakers(batch3Makers(makers.catalog())),
		log.option(),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, want int) {
		t.Helper()
		if got := len(mw.inWorld(tyrannosaurusID)); got != want {
			t.Fatalf("%s: tyrannosauri in the world = %d, want %d", step, got, want)
		}
	}
	send := func(name string) {
		t.Helper()
		if !w.srv.NpcSpawns.MakerEvent("rune20_mb2017_03m1", name, 0, 0) {
			t.Fatal("npcmaker rune20_mb2017_03m1 not found by name")
		}
		w.srv.Settle(t)
	}
	heard11049 := heardEvent{npcID: tyrannosaurusID, event: 11049}
	// bringIn runs 1001 and lets its timer 1001 fire.
	bringIn := func(step string) {
		t.Helper()
		send("1001")
		w.srv.Advance(t, time.Millisecond)
		count(step, 1)
		if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{tyrannosaurusID}) {
			t.Fatalf("%s: NPCs shown = %v, want [%d]", step, got, tyrannosaurusID)
		}
		w.srv.Advance(t, 5000*time.Millisecond-time.Millisecond)
		log.expect(t, step+", timer 1001", heard11049)
	}
	// leave deletes the NPC and waits out timer 1002.
	leave := func(step string) {
		t.Helper()
		mw.inWorld(tyrannosaurusID)[0].DeleteMe()
		w.srv.Settle(t)
		if w.srv.NpcRespawns.Tracked("rune20_mb2017_03m1#0#0") {
			t.Fatalf("%s: the NPC armed a respawn of its own", step)
		}
		w.srv.Advance(t, 180*time.Second-time.Millisecond)
		makers.expect(t, step+", before timer 1002")
		w.srv.Advance(t, time.Millisecond)
		makers.expect(t, step+", timer 1002", "rune20_mb2017_04m1:11042(0,0)")
	}
	count("boot", 0)
	makers.expect(t, "boot")
	drainFrames(t, w.c)

	bringIn("1001")
	leave("the NPC left with the flag down")
	count("the NPC left", 0)

	w.srv.Advance(t, 18000*time.Second-180*time.Second-time.Millisecond)
	makers.expect(t, "before timer 78001")
	w.srv.Advance(t, time.Millisecond)
	makers.expect(t, "timer 78001",
		"rune20_mb2017_01m1:11052(0,0)", "rune20_mb2017_01m1:1000(0,0)",
		"rune20_mb2017_02m1:11052(0,0)", "rune20_mb2017_02m1:1000(0,0)",
		"rune16_npc2017_01m1:11050(0,0)")

	bringIn("1001 with the flag up")
	leave("the NPC left with the flag up")
	w.srv.Advance(t, 18000*time.Second)
	makers.expect(t, "timer 78001 with the flag up")
}

const (
	frintezzaID       = 21751
	frintezzaFollower = 21752
	frintezzaCubeID   = 21753
	frintezzaRowKey   = "frintessa_fixture"
	frintezzaLst      = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="frintessa_lair" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="frintessa_maker" territory="frintessa_lair" maximumNpcs="9">
		<ai type="default_use_db_maker_for_frintessa"><set name="maker_name" val="frintessa_teleportcuve"/></ai>
		<npc id="21751" total="1" pos="40;20;30;0" respawn="60sec" dbName="frintessa_fixture"/>
		<npc id="21752" total="1" pos="60;20;30;0" respawn="60sec" dbName="follower_fixture"/>
	</npcmaker>
	<npcmaker name="frintessa_teleportcuve" territory="frintessa_lair" maximumNpcs="1">
		<ai type="no_on_start_maker"/>
		<npc id="21753" total="1" pos="80;20;30;0" respawn="0sec"/>
	</npcmaker>
</list>`
)

// DefaultUseDBMakerForFrintezza: at start each database-tracked spawn
// places its NPC, Frintezza from his saved live row (loaded, with the
// row's HP) and the follower whose row was never saved fresh. The last of
// them leaving sends 1001 to the npcmaker the maker_name memo names, which
// brings the teleport cube in; each comes back after its respawn delay,
// unloaded, at full HP.
func TestDefaultUseDBMakerForFrintezzaPlacesItsRows(t *testing.T) {
	store := gamesql.NewSpawnStore(sqltest.SharedDB(t))
	row := &spawn.State{Name: frintezzaRowKey, Status: spawn.StatusAlive, CurrentHP: 30, CurrentMP: 10, Location: location.Location{X: 100, Y: 100, Z: 30}}
	if err := store.SaveStates(context.Background(), map[string]*spawn.State{frintezzaRowKey: row}); err != nil {
		t.Fatalf("seed spawn data: %v", err)
	}
	log := &useDBLog{}
	kinds := map[int32]script.NPCKind{frintezzaID: script.KindHostile, frintezzaFollower: script.KindHostile}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(frintezzaID, frintezzaFollower, frintezzaCubeID)),
		gameservertest.WithNpcSpawns(batch1Table(t, frintezzaLst)),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(batch3Makers(nil)),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.FrintezzaRows"}}, script.Catalog{"ai.FrintezzaRows": func() script.Script {
			s := log.boss()
			s.NPCs = []int32{frintezzaID, frintezzaFollower}
			return s
		}}),
	)
	mw := &makerWorld{srv: w.srv}
	boot := log.take()
	if len(boot) != 2 || boot[0] != "loaded=true hp=30 full=false" || !strings.HasPrefix(boot[1], "loaded=false ") || !strings.HasSuffix(boot[1], " full=true") {
		t.Fatalf("boot = %q, want Frintezza loaded from his row and a fresh follower", boot)
	}
	if got := len(mw.inWorld(frintezzaCubeID)); got != 0 {
		t.Fatalf("teleport cubes at boot = %d, want 0", got)
	}
	drainFrames(t, w.c)

	frintezza := mw.inWorld(frintezzaID)[0]
	frintezza.DeleteMe()
	w.srv.Advance(t, time.Millisecond)
	if got := len(mw.inWorld(frintezzaCubeID)); got != 0 {
		t.Fatalf("teleport cubes with the follower alive = %d, want 0", got)
	}
	follower := mw.inWorld(frintezzaFollower)[0]
	follower.DeleteMe()
	w.srv.Advance(t, time.Millisecond)
	if got := len(mw.inWorld(frintezzaCubeID)); got != 1 {
		t.Fatalf("teleport cubes after the last left = %d, want 1", got)
	}
	frames := drainFrames(t, w.c)
	if got := shown(frames); !slices.Equal(got, []int{frintezzaCubeID}) {
		t.Fatalf("NPCs shown after the last left = %v, want [%d]", got, frintezzaCubeID)
	}
	if got := deleted(frames); !slices.Equal(got, []int32{frintezza.ObjectID(), follower.ObjectID()}) {
		t.Fatalf("deleted objects = %v, want [%d %d]", got, frintezza.ObjectID(), follower.ObjectID())
	}
	for _, slot := range []string{"frintessa_maker#0#0", "frintessa_maker#1#0"} {
		if w.srv.NpcRespawns.Tracked(slot) {
			t.Fatalf("slot %s armed a respawn of its own", slot)
		}
	}

	w.srv.Advance(t, 60*time.Second-3*time.Millisecond)
	if got := len(mw.inWorld(frintezzaID)) + len(mw.inWorld(frintezzaFollower)); got != 0 {
		t.Fatalf("NPCs back before the respawn delay = %d, want 0", got)
	}
	w.srv.Advance(t, 2*time.Millisecond)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{frintezzaID, frintezzaFollower}) {
		t.Fatalf("NPCs shown after the respawn delay = %v, want [%d %d]", got, frintezzaID, frintezzaFollower)
	}
	got := log.take()
	if len(got) != 2 {
		t.Fatalf("respawns = %q, want two", got)
	}
	for _, line := range got {
		if !strings.HasPrefix(line, "loaded=false ") || !strings.HasSuffix(line, " full=true") {
			t.Fatalf("respawns = %q, want unloaded NPCs at full HP", got)
		}
	}
}

const (
	iceFairySirrID  = 21761
	iceFairySirrLst = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="sirr_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="schuttgart13_mb2314_05m1" territory="sirr_field" maximumNpcs="8">
		<ai type="ice_fairy_sirr_maker"><set name="DoorName" val="MAKER_GATE"/></ai>
		<npc id="21761" total="1" pos="40;20;30;0" respawn="no"/>
	</npcmaker>
	<npcmaker name="schuttgart13_npc2314_1m1" territory="sirr_field" maximumNpcs="1">
		<ai type="maker_event_log"/>
	</npcmaker>
</list>`
)

// IceFairySirrMaker: nothing spawns at start and the door opening first
// does nothing. The door closing spawns Sirra and runs 11037 on
// schuttgart13_npc2314_1m1. Maker script event 10005 adds one to the
// count and sends her 10001 with it, 11040 sends her 11040 with int1; the
// default maker's 1000 and 1001 do nothing. Sirra leaving sets the count
// back to 0 and arms no respawn. The door opening deletes the group and
// closing again brings her back.
func TestIceFairySirrMakerFollowsItsDoor(t *testing.T) {
	t.Parallel()
	log := &heardLog{ids: []int32{iceFairySirrID}}
	makers := &makerEventLog{}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithDoors(makerGate()),
		gameservertest.WithNPCs(batch1Templates(iceFairySirrID)),
		gameservertest.WithNpcSpawns(batch1Table(t, iceFairySirrLst)),
		gameservertest.WithMakers(batch3Makers(makers.catalog())),
		log.option(),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, want int) {
		t.Helper()
		if got := len(mw.inWorld(iceFairySirrID)); got != want {
			t.Fatalf("%s: Sirras in the world = %d, want %d", step, got, want)
		}
	}
	send := func(name string, int1 int) {
		t.Helper()
		if !w.srv.NpcSpawns.MakerEvent("schuttgart13_mb2314_05m1", name, int1, 0) {
			t.Fatal("npcmaker schuttgart13_mb2314_05m1 not found by name")
		}
		w.srv.Advance(t, time.Millisecond)
	}
	door := func(open bool) {
		t.Helper()
		w.srv.WorldObjects.SetDoorOpen(makerGateID, open)
		w.srv.Settle(t)
	}
	count("boot", 0)
	makers.expect(t, "boot")
	drainFrames(t, w.c)

	door(true)
	count("door opened while disabled", 0)
	makers.expect(t, "door opened while disabled")

	door(false)
	count("door closed", 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{iceFairySirrID}) {
		t.Fatalf("door closed: NPCs shown = %v, want [%d]", got, iceFairySirrID)
	}
	makers.expect(t, "door closed", "schuttgart13_npc2314_1m1:11037(0,0)")

	send("10005", 0)
	send("10005", 0)
	send("11040", 7)
	log.expect(t, "10005 twice and 11040",
		heardEvent{npcID: iceFairySirrID, event: 10001, arg1: 1},
		heardEvent{npcID: iceFairySirrID, event: 10001, arg1: 2},
		heardEvent{npcID: iceFairySirrID, event: 11040, arg1: 7})
	send("1000", 0)
	send("1001", 0)
	count("1000 and 1001", 1)
	log.expect(t, "1000 and 1001")
	if frames := drainFrames(t, w.c); len(shown(frames)) != 0 || len(deleted(frames)) != 0 {
		t.Fatalf("1000 and 1001: shown %v and deleted %v, want none", shown(frames), deleted(frames))
	}

	sirra := mw.inWorld(iceFairySirrID)[0]
	sirra.DeleteMe()
	w.srv.Settle(t)
	if w.srv.NpcRespawns.Tracked("schuttgart13_mb2314_05m1#0#0") {
		t.Fatal("Sirra armed a respawn of her own")
	}
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, []int32{sirra.ObjectID()}) {
		t.Fatalf("Sirra left: deleted objects = %v, want [%d]", got, sirra.ObjectID())
	}

	door(true)
	door(false)
	count("door opened and closed", 1)
	makers.expect(t, "door opened and closed", "schuttgart13_npc2314_1m1:11037(0,0)")
	log.take()
	send("10005", 0)
	log.expect(t, "10005 after Sirra left", heardEvent{npcID: iceFairySirrID, event: 10001, arg1: 1})

	sirra = mw.inWorld(iceFairySirrID)[0]
	drainFrames(t, w.c)
	door(true)
	count("door opened", 0)
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, []int32{sirra.ObjectID()}) {
		t.Fatalf("door opened: deleted objects = %v, want [%d]", got, sirra.ObjectID())
	}
	makers.expect(t, "door opened")
}
