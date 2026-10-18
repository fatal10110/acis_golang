package npcs

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The maker spawn operations (#3800): a maker sends a script event to every
// NPC one of its spawns keeps, reads the spawn's first NPC, deletes one
// spawn's NPCs, draws a spawn's point, finds a spawn of any npcmaker by
// template alias, and checks a database-tracked spawn's row; a script finds
// a spawn by NPC id and starts or stops the npcmakers of a spawn time.
const (
	mopsGuardID  = 21501 // two guards at fixed points, kept by spawn 0
	mopsKeeperID = 21502 // one keeper, spawn 1 of the same maker
	mopsBossID   = 21503 // a database-tracked boss under a maker that never spawns on its own
	mopsSiegeID  = 21504 // a hall defender of spawn time agit_defend_warfare_start(64)
	mopsLooseID  = 21505 // a monster no npcmaker declares
)

const mopsSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="mops_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="mops_guards" territory="mops_field" maximumNpcs="3">
		<ai type="mops_maker"/>
		<npc id="21501" total="2" pos="40;20;30;100" respawn="60sec"/>
		<npc id="21502" total="1" pos="80;20;30;200" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="mops_lair" territory="mops_field" maximumNpcs="1">
		<ai type="mops_idle_maker"/>
		<npc id="21503" total="1" pos="120;20;30;300" respawn="60sec" dbName="mops_boss"/>
	</npcmaker>
	<npcmaker name="mops_hall" territory="mops_field" maximumNpcs="1" spawnTime="agit_defend_warfare_start(64)">
		<ai type="mops_maker"/>
		<npc id="21504" total="1" pos="160;20;30;400" respawn="60sec"/>
	</npcmaker>
</list>`

func mopsTemplates() *npc.Table {
	guard := spawnMonster(mopsGuardID, "Guard")
	keeper := spawnMonster(mopsKeeperID, "Keeper")
	keeper.Alias = "mops_keeper_alias"
	boss := spawnMonster(mopsBossID, "Boss")
	boss.HPMax, boss.MPMax = 5000, 700
	return npc.NewTable([]*npc.Template{guard, keeper, boss, spawnMonster(mopsSiegeID, "Defender"), spawnMonster(mopsLooseID, "Loose")})
}

// mopsGroups holds the group of each npcmaker as its maker's start hook
// was handed it.
type mopsGroups struct {
	mu     sync.Mutex
	groups map[string]spawn.Group
	// deleted counts the maker hooks run for an NPC leaving the world.
	deleted atomic.Int32
}

func (m *mopsGroups) group(t *testing.T, name string) spawn.Group {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[name]
	if !ok {
		t.Fatalf("npcmaker %s was never started", name)
	}
	return g
}

// makers are the fixture's makers: mops_maker spawns every spawn up to its
// total whenever its group starts; mops_idle_maker never spawns on its own.
// Both keep the group they start with, and count their NPCs leaving.
func (m *mopsGroups) makers() script.MakerCatalog {
	keep := func(g spawn.Group) {
		m.mu.Lock()
		m.groups[g.Maker().Name] = g
		m.mu.Unlock()
	}
	deleted := func(*script.Maker, script.MakerNPC) { m.deleted.Add(1) }
	return script.MakerCatalog{
		"mops_maker": func() script.Maker {
			return script.Maker{MakerHooks: maker.Default().MakerHooks.With(script.MakerHooks{
				OnStart: func(_ *script.Maker, e script.MakerStart) {
					keep(e.Group)
					for _, ms := range e.Group.Spawns() {
						for range ms.Total() - ms.Spawned() {
							ms.Spawn()
						}
					}
				},
				OnNPCCreated: func(*script.Maker, script.MakerNPC) {},
				OnNPCDeleted: deleted,
			})}
		},
		"mops_idle_maker": func() script.Maker {
			return script.Maker{MakerHooks: script.MakerHooks{
				OnStart:      func(_ *script.Maker, e script.MakerStart) { keep(e.Group) },
				OnNPCDeleted: deleted,
			}}
		},
	}
}

// mopsEvents records the script events the guards and the keeper receive,
// and the object id of each life created, in creation order.
type mopsEvents struct {
	mu       sync.Mutex
	created  []int32
	received []mopsReceipt
}

type mopsReceipt struct {
	id               int32
	decayed          bool
	event, arg1, arg int32
}

func (l *mopsEvents) script() script.Script {
	ids := []int32{mopsGuardID, mopsKeeperID}
	return script.Script{
		Bind: script.Bindings{script.EventCreated: ids, script.EventScriptEvent: ids},
		Hooks: script.Hooks{
			OnCreated: func(_ *script.Script, e script.Created) {
				l.mu.Lock()
				l.created = append(l.created, e.NPC.ObjectID())
				l.mu.Unlock()
			},
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				l.mu.Lock()
				l.received = append(l.received, mopsReceipt{id: e.NPC.ObjectID(), decayed: e.NPC.Decayed(), event: e.EventID, arg1: e.Arg1, arg: e.Arg2})
				l.mu.Unlock()
			},
		},
	}
}

func (l *mopsEvents) takeReceived() []mopsReceipt {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.received
	l.received = nil
	return out
}

func (l *mopsEvents) createdIDs() []int32 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.created)
}

type mopsWorld struct {
	srv       *gameservertest.Server
	templates *npc.Table
	groups    *mopsGroups
	events    *mopsEvents
	spawner   *script.Spawner
}

func bootMops(t *testing.T, opts ...gameservertest.Option) *mopsWorld {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mops.xml"), []byte(mopsSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	w := &mopsWorld{templates: mopsTemplates(), groups: &mopsGroups{groups: map[string]spawn.Group{}}, events: &mopsEvents{}}
	kinds := map[int32]script.NPCKind{mopsGuardID: script.KindHostile, mopsKeeperID: script.KindHostile}
	w.srv = gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithNPCs(w.templates),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithMakers(w.groups.makers()),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "quest.MopsReceiver"}}, script.Catalog{"quest.MopsReceiver": w.events.script}),
	}, opts...)...)
	w.srv.Settle(t)
	w.spawner = script.NewSpawner(w.srv.NpcSpawns, zerolog.Nop())
	return w
}

// liveCount counts the NPCs of template id in the world.
func (w *mopsWorld) liveCount(id int) int {
	count := 0
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == id {
			count++
		}
	}
	return count
}

// A maker's script event to a spawn reaches every NPC the spawn keeps, in
// creation order, the one deleted out of the world included, as its last
// life; the other spawn's NPC hears nothing. The spawn's first NPC is the
// first it created, and a script event sent to it reaches only that NPC.
func TestMakerSpawnSendScriptEventReachesDecayedNPCs(t *testing.T) {
	t.Parallel()
	w := bootMops(t)
	guards := w.groups.group(t, "mops_guards").Spawns()[0]
	created := w.events.createdIDs()
	if len(created) != 3 {
		t.Fatalf("created lives = %v, want the two guards and the keeper", created)
	}
	first, second := created[0], created[1]

	npcs := guards.NPCs()
	if len(npcs) != 2 {
		t.Fatalf("guard spawn keeps %d NPCs, want 2", len(npcs))
	}
	npcs[1].Delete()
	w.srv.Settle(t)
	if !npcs[1].Decayed() || guards.Decayed() != 1 {
		t.Fatalf("deleted guard decayed %v, spawn decayed count %d; want the second guard out of the world", npcs[1].Decayed(), guards.Decayed())
	}

	guards.SendScriptEvent(11042, 1, 0)
	want := []mopsReceipt{{id: first, event: 11042, arg1: 1}, {id: second, decayed: true, event: 11042, arg1: 1}}
	if got := w.events.takeReceived(); !slices.Equal(got, want) {
		t.Fatalf("spawn script event reached %+v, want %+v", got, want)
	}

	head := guards.FirstNPC()
	if head == nil || head.Decayed() {
		t.Fatal("the guard spawn's first NPC is missing or out of the world, want the first guard")
	}
	head.SendScriptEvent(10100, 1, 0)
	if got, want := w.events.takeReceived(), []mopsReceipt{{id: first, event: 10100, arg1: 1}}; !slices.Equal(got, want) {
		t.Fatalf("script event to the first NPC reached %+v, want %+v", got, want)
	}

	guards.DeleteNPCs()
	if guards.FirstNPC() != nil {
		t.Fatal("a spawn that keeps no NPC still has a first one")
	}
	guards.SendScriptEvent(11042, 1, 0)
	if got := w.events.takeReceived(); len(got) != 0 {
		t.Fatalf("script event to a spawn that keeps no NPC reached %+v", got)
	}
}

// Deleting one spawn's NPCs takes its live NPC out of the world and forgets
// the deleted one whose respawn is pending: nothing comes back, no maker
// hook runs, and the maker's other spawn keeps its NPC. The spawn spawns
// again afterwards as a new one.
func TestMakerSpawnDeleteNPCs(t *testing.T) {
	t.Parallel()
	w := bootMops(t)
	g := w.groups.group(t, "mops_guards")
	guards, keeper := g.Spawns()[0], g.Spawns()[1]
	guards.NPCs()[0].Delete()
	w.srv.Settle(t)
	deletedBefore := w.groups.deleted.Load()

	guards.DeleteNPCs()
	w.srv.Settle(t)
	if n := w.liveCount(mopsGuardID); n != 0 {
		t.Fatalf("%d guards in the world after DeleteNPCs, want none", n)
	}
	if guards.Spawned() != 0 || guards.Decayed() != 0 || len(guards.NPCs()) != 0 {
		t.Fatalf("guard spawn: spawned %d, decayed %d, npcs %d; want it to keep none", guards.Spawned(), guards.Decayed(), len(guards.NPCs()))
	}
	if got := w.groups.deleted.Load(); got != deletedBefore {
		t.Fatalf("maker deleted hooks ran %d times for DeleteNPCs, want none", got-deletedBefore)
	}
	if keeper.Spawned() != 1 || w.liveCount(mopsKeeperID) != 1 || g.Alive() != 1 {
		t.Fatalf("keeper spawn: spawned %d, live %d, group alive %d; want the keeper untouched", keeper.Spawned(), w.liveCount(mopsKeeperID), g.Alive())
	}
	w.srv.Advance(t, 10*time.Minute)
	if n := w.liveCount(mopsGuardID); n != 0 {
		t.Fatalf("%d guards came back after DeleteNPCs, want none", n)
	}

	guards.Spawn()
	w.srv.Settle(t)
	if guards.Spawned() != 1 || w.liveCount(mopsGuardID) != 1 {
		t.Fatalf("after a new spawn: spawned %d, live %d; want one guard", guards.Spawned(), w.liveCount(mopsGuardID))
	}
}

// A maker finds the first spawn whose template has an alias, ignoring case,
// across every npcmaker, and draws a spawn's declared point; an alias no
// template has finds nothing.
func TestMakerFindSpawnAndDrawLocation(t *testing.T) {
	t.Parallel()
	w := bootMops(t)
	lair := w.groups.group(t, "mops_lair")
	found := lair.FindSpawn("MOPS_KEEPER_ALIAS")
	if found == nil || found.Entry().NPCID != mopsKeeperID || found.Spawned() != 1 {
		t.Fatalf("FindSpawn by the keeper's alias = %v, want the keeper's spawn of mops_guards", found)
	}
	if got := lair.FindSpawn("no_such_alias"); got != nil {
		t.Fatalf("FindSpawn by an unknown alias = %v, want nil", got)
	}
	at, ok := found.DrawSpawnLocation()
	if want := (spawn.Position{Location: location.Location{X: 80, Y: 20, Z: 30}, Heading: 200}); !ok || at.Location != want.Location || at.Heading != want.Heading {
		t.Fatalf("keeper spawn's point = %+v (%v), want %+v", at, ok, want)
	}
}

// A script finds a spawn by NPC id: an npcmaker's first, else a standalone
// one; it draws the spawn's point, and finds nothing for an NPC no spawn
// places.
func TestScriptFindSpawn(t *testing.T) {
	t.Parallel()
	w := bootMops(t)
	sp := w.spawner.FindSpawn(mopsGuardID)
	if sp == nil {
		t.Fatal("FindSpawn of the guards found nothing")
	}
	if got, want := sp.DrawSpawnLocation(), (script.Loc{X: 40, Y: 20, Z: 30, Heading: 100}); got != want {
		t.Fatalf("guard spawn's point = %+v, want %+v", got, want)
	}
	if got := w.spawner.FindSpawn(mopsLooseID); got != nil {
		t.Fatal("FindSpawn of an NPC no spawn places found one")
	}
	loose, ok := w.templates.Get(mopsLooseID)
	if !ok {
		t.Fatal("no loose template")
	}
	if err := w.srv.NpcSpawns.SpawnFixed(loose, 300, 300, 30, 500, 0); err != nil {
		t.Fatalf("spawn the loose monster: %v", err)
	}
	sp = w.spawner.FindSpawn(mopsLooseID)
	if sp == nil {
		t.Fatal("FindSpawn of a standalone spawn found nothing")
	}
	if got := sp.DrawSpawnLocation(); got.X != 300 || got.Y != 300 || got.Heading != 500 {
		t.Fatalf("standalone spawn's point = %+v, want (300, 300) facing 500", got)
	}
}

// A script starts the npcmakers of a spawn time whose parameters match,
// ignoring case; a parameter that differs, or none, starts nothing. The
// stop deletes their NPCs and reports how many.
func TestScriptStartStopSpawnTime(t *testing.T) {
	t.Parallel()
	w := bootMops(t)
	if n := w.liveCount(mopsSiegeID); n != 0 {
		t.Fatalf("%d defenders at boot, want none: a spawn-time maker does not start at boot", n)
	}
	if got := w.spawner.StartSpawnTime("agit_defend_warfare_start", "65"); got != 0 || w.liveCount(mopsSiegeID) != 0 {
		t.Fatalf("start with another hall started %d", got)
	}
	if got := w.spawner.StartSpawnTime("agit_defend_warfare_start"); got != 0 || w.liveCount(mopsSiegeID) != 0 {
		t.Fatalf("start with no parameter started %d", got)
	}
	if got := w.spawner.StartSpawnTime("AGIT_DEFEND_WARFARE_START", "64"); got != 1 {
		t.Fatalf("start of hall 64 reports %d NPCs, want 1", got)
	}
	w.srv.Settle(t)
	if n := w.liveCount(mopsSiegeID); n != 1 {
		t.Fatalf("%d defenders after the start, want 1", n)
	}
	if got := w.spawner.StopSpawnTime("agit_defend_warfare_start", "64"); got != 1 {
		t.Fatalf("stop of hall 64 reports %d NPCs, want 1", got)
	}
	w.srv.Settle(t)
	if n := w.liveCount(mopsSiegeID); n != 0 {
		t.Fatalf("%d defenders after the stop, want none", n)
	}
}

// A database-tracked spawn's row: never initialized, it is not dead, and
// the alive check makes it alive at the point given with the template's
// base max HP and MP, reporting false; asked again it reports true, and the
// spawn's NPC is then placed at the row's point. Once the NPC is killed and
// its corpse decays, the row is dead with its respawn ahead. A spawn that
// keeps no row panics.
func TestMakerSpawnRowChecks(t *testing.T) {
	w := bootMops(t, gameservertest.WithSpawnData())
	boss := w.groups.group(t, "mops_lair").Spawns()[0]
	if boss.CheckDead() {
		t.Fatal("a row never initialized reads dead")
	}
	at := spawn.Position{Location: location.Location{X: 150, Y: 60, Z: 30}, Heading: 1234}
	if boss.CheckAlive(at) {
		t.Fatal("the alive check of a row never initialized reports true, want false as it becomes alive")
	}
	row, ok := w.srv.NpcSpawns.Spawns().State("mops_boss")
	if !ok || row.Status != spawn.StatusAlive || row.Location != at.Location || row.Heading != at.Heading || row.CurrentHP != 5000 || row.CurrentMP != 700 || row.RespawnTime != 0 {
		t.Fatalf("row after the alive check = %+v, want alive at %+v with 5000 HP and 700 MP", row, at)
	}
	if !boss.CheckAlive(spawn.Position{Location: location.Location{X: 1, Y: 1, Z: 1}}) {
		t.Fatal("the alive check of a row made alive reports false")
	}
	if row, _ := w.srv.NpcSpawns.Spawns().State("mops_boss"); row.Location != at.Location {
		t.Fatalf("a second alive check moved the row to %+v", row.Location)
	}
	if n := w.liveCount(mopsBossID); n != 0 {
		t.Fatalf("%d bosses in the world after the checks, want none: a check spawns nothing", n)
	}

	boss.Spawn()
	w.srv.Settle(t)
	live := liveOf(t, w.srv, mopsBossID).(*npc.Hostile)
	if x, y, _ := live.Position(); x != 150 || y != 60 || live.Heading() != 1234 {
		t.Fatalf("boss placed at (%d, %d) facing %d, want the row's (150, 60) facing 1234", x, y, live.Heading())
	}

	onQueueOf(t, live, func() {
		if !live.Die(nil, nil) {
			t.Error("the boss did not die")
		}
	})
	onQueueOf(t, live, func() { live.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(live.ObjectID())) })
	w.srv.Settle(t)
	if !boss.CheckDead() {
		t.Fatal("the row of a decayed boss with its respawn ahead does not read dead")
	}

	nonDB := w.groups.group(t, "mops_guards").Spawns()[0]
	panicked := func() (p bool) {
		defer func() { p = recover() != nil }()
		nonDB.CheckDead()
		return false
	}()
	if !panicked {
		t.Fatal("CheckDead of a spawn that keeps no row did not panic")
	}
}
