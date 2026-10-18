package npcs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The makers of batch 1 (#212) as the catalog binds them: each scenario
// boots its npcmakers with the batch's catalog file, so an npcmaker of a
// batch type runs the ported maker and nothing else.

// batch1Makers is the batch's catalog file without the types not ported
// yet, as the composition root joins it.
func batch1Makers() script.MakerCatalog {
	out := script.MakerCatalog{}
	for typ, ctor := range catalog.Makers()["makers_1"] {
		if ctor != nil {
			out[typ] = ctor
		}
	}
	return out
}

// batch1Table loads spawnlist as the spawnlist directory's one file.
func batch1Table(t *testing.T, spawnlist string) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "batch1.xml"), []byte(spawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

func batch1Templates(ids ...int) *npc.Table {
	var templates []*npc.Template
	for _, id := range ids {
		templates = append(templates, spawnMonster(id, fmt.Sprintf("Batch %d", id)))
	}
	return npc.NewTable(templates)
}

const (
	closeDoorGuardID  = 21601
	closeDoorStrayID  = 21602
	closeDoorSpawnlst = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="ice_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="ice_guard" territory="ice_field" maximumNpcs="2">
		<ai type="Close_Door_maker"><set name="DoorName" val="MAKER_GATE"/></ai>
		<npc id="21601" total="3" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="ice_stray" territory="ice_field" maximumNpcs="1">
		<ai type="Close_Door_maker"/>
		<npc id="21602" total="1" pos="60;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
)

// CloseDoorMaker: nothing spawns at start; the npcmaker registers on the
// door its DoorName memo names (ignoring case). The door opening first
// does nothing; closing spawns the group up to the maker's maximum (two of
// the three), and the character is shown them. An NPC leaving arms no
// respawn. The door opening deletes the rest; an NPC the default script
// event 1001 then brings is deleted as it appears. An npcmaker with no
// DoorName never spawns.
func TestCloseDoorMakerFollowsItsDoor(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithDoors(makerGate()),
		gameservertest.WithNPCs(batch1Templates(closeDoorGuardID, closeDoorStrayID)),
		gameservertest.WithNpcSpawns(batch1Table(t, closeDoorSpawnlst)),
		gameservertest.WithMakers(batch1Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	world := func(step string, wantGuards int) {
		t.Helper()
		if got := len(mw.inWorld(closeDoorGuardID)); got != wantGuards {
			t.Fatalf("%s: guards in the world = %d, want %d", step, got, wantGuards)
		}
		if got := len(mw.inWorld(closeDoorStrayID)); got != 0 {
			t.Fatalf("%s: stray NPCs in the world = %d, want 0", step, got)
		}
	}
	world("boot", 0)
	drainFrames(t, w.c)

	w.srv.WorldObjects.SetDoorOpen(makerGateID, true)
	w.srv.Settle(t)
	world("door opened while disabled", 0)
	if got := shown(drainFrames(t, w.c)); len(got) != 0 {
		t.Fatalf("door opened while disabled: NPCs shown = %v, want none", got)
	}

	w.srv.WorldObjects.SetDoorOpen(makerGateID, false)
	w.srv.Settle(t)
	world("door closed", 2)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, repeatID(closeDoorGuardID, 2)) {
		t.Fatalf("door closed: NPCs shown = %v, want two of %d", got, closeDoorGuardID)
	}

	guards := mw.inWorld(closeDoorGuardID)
	guards[0].DeleteMe()
	w.srv.Settle(t)
	for i := range 3 {
		if key := fmt.Sprintf("ice_guard#0#%d", i); w.srv.NpcRespawns.Tracked(key) {
			t.Fatalf("slot %s armed a respawn", key)
		}
	}
	w.srv.Advance(t, 61*time.Second)
	world("a guard left", 1)
	drainFrames(t, w.c)

	w.srv.WorldObjects.SetDoorOpen(makerGateID, true)
	w.srv.Settle(t)
	world("door opened", 0)
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, []int32{guards[1].ObjectID()}) {
		t.Fatalf("door opened: deleted objects = %v, want [%d]", got, guards[1].ObjectID())
	}

	if !w.srv.NpcSpawns.MakerEvent("ice_guard", "1001", 0, 0) {
		t.Fatal("npcmaker ice_guard not found by name")
	}
	w.srv.Advance(t, time.Millisecond)
	world("1001 while the door is open", 0)
	frames := drainFrames(t, w.c)
	if got, gone := shown(frames), deleted(frames); len(got) != len(gone) {
		t.Fatalf("1001 while the door is open: shown %v but deleted %v", got, gone)
	}
}

const (
	instantOnceID   = 21611
	instantKeptID   = 21612
	instantSpawnlst = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="instant_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="instant_m" territory="instant_field" maximumNpcs="2">
		<ai type="maker_instant_spawn"/>
		<npc id="21611" total="1" pos="40;20;30;0" respawn="no"/>
		<npc id="21612" total="1" pos="60;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
)

// InstantSpawnMaker: an NPC leaving the world respawns nothing on its own.
// The group's last one runs the maker's own script event 1001 with no
// delay, which places a new NPC for a spawn that kept none (the one-time
// NPC's slot is gone) but cannot bring back a kept NPC: a respawn of no
// delay is no respawn. An outside 1001 with a delay arms its respawn.
func TestInstantSpawnMakerRefillsOnTheLastNPC(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(instantOnceID, instantKeptID)),
		gameservertest.WithNpcSpawns(batch1Table(t, instantSpawnlst)),
		gameservertest.WithMakers(batch1Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	count := func(step string, wantOnce, wantKept int) {
		t.Helper()
		if got := len(mw.inWorld(instantOnceID)); got != wantOnce {
			t.Fatalf("%s: one-time NPCs = %d, want %d", step, got, wantOnce)
		}
		if got := len(mw.inWorld(instantKeptID)); got != wantKept {
			t.Fatalf("%s: kept NPCs = %d, want %d", step, got, wantKept)
		}
	}
	count("boot", 1, 1)
	drainFrames(t, w.c)

	once := mw.inWorld(instantOnceID)[0]
	once.DeleteMe()
	w.srv.Advance(t, time.Millisecond)
	count("the one-time NPC left", 0, 1)
	if frames := drainFrames(t, w.c); len(shown(frames)) != 0 {
		t.Fatalf("NPCs shown after the first NPC left = %v, want none", shown(frames))
	}

	kept := mw.inWorld(instantKeptID)[0]
	kept.DeleteMe()
	w.srv.Advance(t, time.Millisecond)
	count("the last NPC left", 1, 0)
	frames := drainFrames(t, w.c)
	if got := deleted(frames); !slices.Contains(got, kept.ObjectID()) {
		t.Fatalf("deleted objects = %v, want %d among them", got, kept.ObjectID())
	}
	if got := shown(frames); !slices.Equal(got, []int{instantOnceID}) {
		t.Fatalf("NPCs shown after the last NPC left = %v, want [%d]", got, instantOnceID)
	}
	if w.srv.NpcRespawns.Tracked("instant_m#1#0") {
		t.Fatal("the kept NPC armed a respawn of its own")
	}
	w.srv.Advance(t, 61*time.Second)
	count("a minute later", 1, 0)

	// The outside 1001 arms the kept NPC's respawn; the fixture's respawn
	// timers never fire on their own, so the test fires it.
	w.srv.NpcSpawns.MakerEvent("instant_m", "1001", 3, 0)
	w.srv.Settle(t)
	if !w.srv.NpcRespawns.Tracked("instant_m#1#0") {
		t.Fatal("the outside 1001 armed no respawn of the kept NPC")
	}
	w.srv.NpcSpawns.Respawn("instant_m#1#0")
	w.srv.Settle(t)
	count("outside 1001", 1, 1)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{instantKeptID}) {
		t.Fatalf("NPCs shown after the outside 1001 = %v, want [%d]", got, instantKeptID)
	}
}

const (
	useDBBossID   = 21621
	useDBGuardID  = 21622
	useDBRowKey   = "usedb_boss"
	useDBSpawnlst = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="usedb_lair" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="usedb_m" territory="usedb_lair" maximumNpcs="1">
		<ai type="default_use_db_maker"><set name="maker_name" val="USEDB_GUARDS"/></ai>
		<npc id="21621" total="1" pos="40;20;30;0" respawn="60sec" dbName="usedb_boss"/>
	</npcmaker>
	<npcmaker name="usedb_guards" territory="usedb_lair" maximumNpcs="2">
		<ai type="no_on_start_maker"/>
		<npc id="21622" total="2" pos="60;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
)

// useDBLog records each boss creation: whether its spawn was loaded from
// a live row.
type useDBLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *useDBLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.lines
	l.lines = nil
	return out
}

func (l *useDBLog) boss() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{useDBBossID}, Hooks: script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			l.mu.Lock()
			l.lines = append(l.lines, fmt.Sprintf("loaded=%v hp=%.0f full=%v", e.NPC.DBLoaded(), e.NPC.HP(), e.NPC.HPRatio() == 1))
			l.mu.Unlock()
		},
	}}
}

// DefaultUseDBMaker: at start the database-tracked spawn places its NPC
// from the saved live row (loaded, with the row's HP). The NPC leaving
// clears the loaded mark, sends 1001 to the npcmaker the maker_name memo
// names (case-insensitive), which brings that maker's guards in, and arms
// no respawn of the NPC's own: the maker brings the spawn back after its
// respawn delay, unloaded, at full HP. The dead row is saved meanwhile.
func TestDefaultUseDBMakerRestoresAndWakesItsMaker(t *testing.T) {
	store := gamesql.NewSpawnStore(sqltest.SharedDB(t))
	row := &spawn.State{Name: useDBRowKey, Status: spawn.StatusAlive, CurrentHP: 30, CurrentMP: 10, Location: location.Location{X: 100, Y: 100, Z: 30}}
	if err := store.SaveStates(context.Background(), map[string]*spawn.State{useDBRowKey: row}); err != nil {
		t.Fatalf("seed spawn data: %v", err)
	}
	log := &useDBLog{}
	kinds := map[int32]script.NPCKind{useDBBossID: script.KindHostile}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(useDBBossID, useDBGuardID)),
		gameservertest.WithNpcSpawns(batch1Table(t, useDBSpawnlst)),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(batch1Makers()),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.UseDBBoss"}}, script.Catalog{"ai.UseDBBoss": log.boss}),
	)
	mw := &makerWorld{srv: w.srv}
	expectLines(t, "boot", log.take(), []string{"loaded=true hp=30 full=false"})
	if got := len(mw.inWorld(useDBGuardID)); got != 0 {
		t.Fatalf("guards at boot = %d, want 0", got)
	}
	drainFrames(t, w.c)

	boss := mw.inWorld(useDBBossID)[0]
	boss.DeleteMe()
	w.srv.Advance(t, time.Millisecond)
	if got := len(mw.inWorld(useDBGuardID)); got != 2 {
		t.Fatalf("guards after the boss left = %d, want 2", got)
	}
	frames := drainFrames(t, w.c)
	if got := shown(frames); !slices.Equal(got, repeatID(useDBGuardID, 2)) {
		t.Fatalf("NPCs shown after the boss left = %v, want the two guards", got)
	}
	if got := deleted(frames); !slices.Equal(got, []int32{boss.ObjectID()}) {
		t.Fatalf("deleted objects = %v, want [%d]", got, boss.ObjectID())
	}
	if w.srv.NpcRespawns.Tracked("usedb_m#0#0") {
		t.Fatal("the boss armed a respawn of its own")
	}

	w.srv.Advance(t, 59*time.Second)
	if got := len(mw.inWorld(useDBBossID)); got != 0 {
		t.Fatalf("bosses before the respawn delay = %d, want 0", got)
	}
	w.srv.Advance(t, time.Second)
	if got := len(mw.inWorld(useDBBossID)); got != 1 {
		t.Fatalf("bosses after the respawn delay = %d, want 1", got)
	}
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{useDBBossID}) {
		t.Fatalf("NPCs shown after the respawn delay = %v, want [%d]", got, useDBBossID)
	}
	if got := log.take(); len(got) != 1 || !strings.HasPrefix(got[0], "loaded=false ") || !strings.HasSuffix(got[0], " full=true") {
		t.Fatalf("respawn = %q, want one unloaded boss at full HP", got)
	}
}

const (
	farmKeeperID  = 21631
	farmAliveID   = 21632
	farmDeadID    = 21633
	farmFreshID   = 21634
	farmSiegeA    = 21641
	farmSiegeB    = 21642
	farmSiegeC    = 21643
	farmSiegeD    = 21644
	farmSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="farm_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="farm_rows" territory="farm_field" maximumNpcs="4">
		<ai type="farm_maker"/>
		<npc id="21631" total="1" pos="40;20;30;0" respawn="1min"/>
		<npc id="21632" total="1" pos="50;20;30;0" respawn="1min" dbName="farm_alive"/>
		<npc id="21633" total="1" pos="60;20;30;0" respawn="1min" dbName="farm_dead"/>
		<npc id="21634" total="1" pos="70;20;30;0" respawn="1min" dbName="farm_fresh"/>
	</npcmaker>
	<npcmaker name="farm_siege" territory="farm_field" maximumNpcs="4">
		<ai type="farm_maker"/>
		<npc id="21641" total="1" pos="80;20;30;0" respawn="1min"/>
		<npc id="21642" total="1" pos="90;20;30;0" respawn="1min"/>
		<npc id="21643" total="1" pos="100;20;30;0" respawn="1min"/>
		<npc id="21644" total="1" pos="110;20;30;0" respawn="1min"/>
	</npcmaker>
</list>`
)

// FarmMaker: at start the first spawn places its NPC and every later one
// runs the npc DB info hook, which brings back only an NPC whose row is
// saved alive: a dead row with its respawn pending stays away, and a row
// never saved comes alive without an NPC (saved alive at shutdown). An NPC
// leaving arms no respawn. onSiegeStart fills the later spawns but the
// last, onFlagWarFinalEvent fills them all, onSiegeEnd deletes them; the
// first spawn's NPC stays through all three.
func TestFarmMakerFollowsTheHallEvents(t *testing.T) {
	store := gamesql.NewSpawnStore(sqltest.SharedDB(t))
	at := location.Location{X: 100, Y: 100, Z: 30}
	if err := store.SaveStates(context.Background(), map[string]*spawn.State{
		"farm_alive": {Name: "farm_alive", Status: spawn.StatusAlive, CurrentHP: 40, CurrentMP: 10, Location: at},
		"farm_dead":  {Name: "farm_dead", Status: spawn.StatusDead, Location: at, RespawnTime: time.Now().Add(time.Hour).UnixMilli()},
	}); err != nil {
		t.Fatalf("seed spawn data: %v", err)
	}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(farmKeeperID, farmAliveID, farmDeadID, farmFreshID, farmSiegeA, farmSiegeB, farmSiegeC, farmSiegeD)),
		gameservertest.WithNpcSpawns(batch1Table(t, farmSpawnlist)),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(batch1Makers()),
	)
	mw := &makerWorld{srv: w.srv}
	present := func(step string, want ...int) {
		t.Helper()
		var got []int
		for _, id := range []int{farmKeeperID, farmAliveID, farmDeadID, farmFreshID, farmSiegeA, farmSiegeB, farmSiegeC, farmSiegeD} {
			for range mw.inWorld(id) {
				got = append(got, id)
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%s: NPCs in the world = %v, want %v", step, got, want)
		}
	}
	present("boot", farmKeeperID, farmAliveID, farmSiegeA)
	if hp := mw.inWorld(farmAliveID)[0].CurrentHP(); hp != 40 {
		t.Fatalf("restored farm NPC HP = %d, want the row's 40", hp)
	}
	drainFrames(t, w.c)

	keeper := mw.inWorld(farmKeeperID)[0]
	keeper.DeleteMe()
	w.srv.Settle(t)
	if w.srv.NpcRespawns.Tracked("farm_rows#0#0") {
		t.Fatal("the farm keeper armed a respawn")
	}
	w.srv.Advance(t, 61*time.Second)
	present("a minute after the keeper left", farmAliveID, farmSiegeA)
	drainFrames(t, w.c)

	npcs := w.srv.NpcSpawns
	npcs.MakerEvent("farm_siege", "onSiegeStart", 0, 0)
	w.srv.Settle(t)
	present("siege start", farmAliveID, farmSiegeA, farmSiegeB, farmSiegeC)
	if got := shown(drainFrames(t, w.c)); !slices.Equal(got, []int{farmSiegeB, farmSiegeC}) {
		t.Fatalf("siege start: NPCs shown = %v, want [%d %d]", got, farmSiegeB, farmSiegeC)
	}

	npcs.MakerEvent("FARM_SIEGE", "ONFLAGWARFINALEVENT", 0, 0)
	w.srv.Settle(t)
	present("flag war final", farmAliveID, farmSiegeA, farmSiegeB, farmSiegeC, farmSiegeD)
	drainFrames(t, w.c)

	later := append(append(mw.inWorld(farmSiegeB), mw.inWorld(farmSiegeC)...), mw.inWorld(farmSiegeD)...)
	npcs.MakerEvent("farm_siege", "onSiegeEnd", 0, 0)
	w.srv.Settle(t)
	present("siege end", farmAliveID, farmSiegeA)
	if got := deleted(drainFrames(t, w.c)); !slices.Equal(got, objectIDs(later)) {
		t.Fatalf("siege end: deleted objects = %v, want %v", got, objectIDs(later))
	}

	npcs.MakerEvent("farm_siege", "onFlagWarFinalEvent", 0, 0)
	w.srv.Settle(t)
	present("flag war final after the end", farmAliveID, farmSiegeA, farmSiegeB, farmSiegeC, farmSiegeD)

	w.srv.Shutdown(t)
	if row := rowState(t, w.srv, "farm_fresh"); row.Status != spawn.StatusAlive {
		t.Fatalf("never-saved farm row status = %d, want alive", row.Status)
	}
	if row := rowState(t, w.srv, "farm_dead"); row.Status != spawn.StatusDead {
		t.Fatalf("dead farm row status = %d, want still dead", row.Status)
	}
}
