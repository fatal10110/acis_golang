package npcs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The saved X of a database-tracked spawn's row, read from a maker's npc
// DB info hook (#4028).

const (
	savedXBossID  = 21311
	savedXGuardID = 21312
	savedXRow     = "saved_x_boss"
)

const savedXSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="lair" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="saved_x_lair" territory="lair" maximumNpcs="1">
		<ai type="saved_x_maker"/>
		<npc id="21311" total="1" pos="40;20;30;0" respawn="60sec" dbName="saved_x_boss"/>
	</npcmaker>
	<npcmaker name="saved_x_guards" territory="lair" maximumNpcs="1">
		<ai type="saved_x_maker"/>
		<npc id="21312" total="1" pos="80;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// savedXLog records what the maker's npc DB info hook reads of its
// spawn's row, and keeps the first spawn of each group the maker starts,
// by maker name.
type savedXLog struct {
	mu     sync.Mutex
	lines  []string
	spawns map[string]spawn.GroupSpawn
}

func (l *savedXLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.lines
	l.lines = nil
	return out
}

func (l *savedXLog) spawn(makerName string) spawn.GroupSpawn {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.spawns[makerName]
}

// maker is the default maker, except that its start keeps the group's
// first spawn and its npc DB info hook first records the row's dead check
// and saved X, as SailrenMaker reads them.
func (l *savedXLog) maker() script.Maker {
	base := maker.Default().MakerHooks
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			l.mu.Lock()
			if l.spawns == nil {
				l.spawns = map[string]spawn.GroupSpawn{}
			}
			l.spawns[e.Group.Maker().Name] = e.Group.Spawns()[0]
			l.mu.Unlock()
			base.Start(m, e)
		},
		OnNPCDBInfo: func(m *script.Maker, e script.MakerSpawn) {
			line := fmt.Sprintf("dead=%v x=%d", e.Spawn.CheckDead(), e.Spawn.SavedX())
			l.mu.Lock()
			l.lines = append(l.lines, line)
			l.mu.Unlock()
			base.NPCDBInfo(m, e)
		},
	})}
}

func bootSavedX(t *testing.T, log *savedXLog) *gameservertest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "saved_x.xml"), []byte(savedXSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{spawnMonster(savedXBossID, "Boss"), spawnMonster(savedXGuardID, "Guard")})),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(script.MakerCatalog{"saved_x_maker": log.maker}),
	)
	srv.Settle(t)
	return srv
}

// A maker reads its spawn's saved X without changing the row: 0 for a row
// never initialized; the X of the row it made alive, saved with the row at
// shutdown and read back from the database after the restart; 0 for a row
// saved dead. A spawn that keeps no row panics.
func TestMakerReadsSavedX(t *testing.T) {
	log := &savedXLog{}
	srv := bootSavedX(t, log)
	expectLines(t, "first boot", log.take(), []string{"dead=false x=0"})
	boss := liveOf(t, srv, savedXBossID).(*npc.Hostile)
	x, _, _ := boss.Position()
	if got := log.spawn("saved_x_lair").SavedX(); got != x || x != 40 {
		t.Fatalf("saved X of the live row = %d, want the boss's X %d at its point 40", got, x)
	}
	srv.Shutdown(t)
	if row := rowState(t, srv, savedXRow); row.Status != spawn.StatusAlive || row.Location.X != 40 {
		t.Fatalf("saved row = status %d at X %d, want alive at 40", row.Status, row.Location.X)
	}

	srv = bootSavedX(t, log)
	expectLines(t, "restart", log.take(), []string{"dead=false x=40"})
	boss = liveOf(t, srv, savedXBossID).(*npc.Hostile)
	onQueueOf(t, boss, func() {
		if !boss.Die(nil, nil) {
			t.Error("the boss did not die")
		}
	})
	onQueueOf(t, boss, func() { boss.Decay(srv.State, srv.NpcSpawns.RespawnHook(boss.ObjectID())) })
	srv.Settle(t)
	before, _ := srv.NpcSpawns.Spawns().State(savedXRow)
	if got := log.spawn("saved_x_lair").SavedX(); got != 0 {
		t.Fatalf("saved X of the dead row = %d, want 0", got)
	}
	if after, _ := srv.NpcSpawns.Spawns().State(savedXRow); after != before || after.Status != spawn.StatusDead {
		t.Fatalf("reading the saved X changed the row from %+v to %+v", before, after)
	}
	srv.Shutdown(t)

	srv = bootSavedX(t, log)
	expectLines(t, "restart with the row dead", log.take(), []string{"dead=true x=0"})

	guards := log.spawn("saved_x_guards")
	if guards == nil {
		t.Fatal("the guard group never started")
	}
	panicked := func() (p bool) {
		defer func() { p = recover() != nil }()
		guards.SavedX()
		return false
	}()
	if !panicked {
		t.Fatal("SavedX of a spawn that keeps no row did not panic")
	}
}
