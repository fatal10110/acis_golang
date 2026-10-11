package npcs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The spawn-row fixture's two database-tracked bosses: one under the
// default maker, one under a maker that clears its spawn's loaded mark
// when its NPC leaves, as the database maker does.
const (
	rowBossID   = 21301
	rowDBBossID = 21302
	rowBossKey  = "row_boss"
	rowDBKey    = "row_db_boss"
)

const rowSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="lair" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="row_a_lair" territory="lair" maximumNpcs="1">
		<npc id="21301" total="1" pos="40;20;30;0" respawn="60sec" dbName="row_boss"/>
	</npcmaker>
	<npcmaker name="row_b_db_lair" territory="lair" maximumNpcs="1">
		<ai type="row_use_db_maker"/>
		<npc id="21302" total="1" pos="60;20;30;0" respawn="60sec" dbName="row_db_boss"/>
	</npcmaker>
</list>`

func rowTable(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "row.xml"), []byte(rowSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// rowUseDBMaker is the default maker, except that its NPC leaving the
// world first clears its spawn's loaded mark.
func rowUseDBMaker() script.Maker {
	base := maker.Default().MakerHooks
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			e.Spawn.ClearDBLoaded()
			base.NPCDeleted(m, e)
		},
	})}
}

// rowLog records what the bosses' created hooks find, one line each.
type rowLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *rowLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.lines
	l.lines = nil
	return out
}

// boss is the behavior of both bosses. Its created hook records the
// loaded mark, the spawn's db value and what the global memo key "5"
// resolves to, then stores the db value plus 3 and its own object id under
// "5", as a grand boss claims its lair.
func (l *rowLog) boss() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{rowBossID, rowDBBossID}, Hooks: script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			memo := "unset"
			if s.GlobalMemo().Int("5", -1) != -1 {
				memo = "gone"
				if c := s.GlobalMemo().Creature("5"); c != nil {
					if n, ok := c.(*script.NPC); ok {
						memo = fmt.Sprintf("npc %d", n.NpcID())
					}
				}
			}
			line := fmt.Sprintf("%d loaded=%v value=%d memo=%s", e.NPC.NpcID(), e.NPC.DBLoaded(), e.NPC.DBValue(), memo)
			e.NPC.SetDBValue(e.NPC.DBValue() + 3)
			s.GlobalMemo().Set("5", int(e.NPC.ObjectID()))
			l.mu.Lock()
			l.lines = append(l.lines, line)
			l.mu.Unlock()
		},
	}}
}

// bootRow boots the fixture backed by the spawn_data table, with no
// character in the world.
func bootRow(t *testing.T, log *rowLog) *gameservertest.Server {
	t.Helper()
	kinds := map[int32]script.NPCKind{rowBossID: script.KindHostile, rowDBBossID: script.KindHostile}
	return gameservertest.Boot(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{spawnMonster(rowBossID, "Boss"), spawnMonster(rowDBBossID, "DB Boss")})),
		gameservertest.WithNpcSpawns(rowTable(t)),
		gameservertest.WithSpawnData(),
		gameservertest.WithMakers(script.MakerCatalog{"row_use_db_maker": rowUseDBMaker}),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.RowBoss"}}, script.Catalog{"ai.RowBoss": log.boss}),
	)
}

// rowState returns the saved spawn_data row name.
func rowState(t *testing.T, srv *gameservertest.Server, name string) *spawn.State {
	t.Helper()
	states, err := gamesql.NewSpawnStore(srv.DB).LoadStates(context.Background())
	if err != nil {
		t.Fatalf("load spawn data: %v", err)
	}
	state, ok := states[name]
	if !ok {
		t.Fatalf("no spawn_data row %q", name)
	}
	return state
}

func expectLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %q, want %q", what, got, want)
	}
}

// A db value a script writes through the NPC handle is saved with the
// spawn_data row at shutdown and read back after the restart, where the
// restored live row marks the spawn loaded; a fresh spawn is not. The
// global memo holds what the first boss stored for the second, and is
// empty again after the restart: it is kept in memory only.
func TestSpawnRowValueSurvivesRestart(t *testing.T) {
	log := &rowLog{}
	srv := bootRow(t, log)
	expectLines(t, "first boot", log.take(), []string{
		fmt.Sprintf("%d loaded=false value=0 memo=unset", rowBossID),
		fmt.Sprintf("%d loaded=false value=0 memo=npc %d", rowDBBossID, rowBossID),
	})
	srv.Shutdown(t)
	if row := rowState(t, srv, rowBossKey); row.Status != spawn.StatusAlive || row.DBValue != 3 {
		t.Fatalf("saved row = status %d value %d, want alive with 3", row.Status, row.DBValue)
	}

	srv = bootRow(t, log)
	expectLines(t, "restart", log.take(), []string{
		fmt.Sprintf("%d loaded=true value=3 memo=unset", rowBossID),
		fmt.Sprintf("%d loaded=true value=3 memo=npc %d", rowDBBossID, rowBossID),
	})
	srv.Shutdown(t)
	if row := rowState(t, srv, rowBossKey); row.DBValue != 6 {
		t.Fatalf("saved row value after the restart = %d, want 6", row.DBValue)
	}
}

// A row saved dead with its respawn still pending is not restored: the
// spawn is not marked loaded, and the NPC it brings back later reads the
// db value the dead row kept. The memo key the dead boss's earlier life
// stored resolves to nothing once that NPC is gone.
func TestSpawnRowDeadRowIsNotLoaded(t *testing.T) {
	log := &rowLog{}
	srv := bootRow(t, log)
	log.take()
	boss := liveOf(t, srv, rowDBBossID).(*npc.Hostile)
	onQueueOf(t, boss, func() {
		if !boss.Die(nil, nil) {
			t.Error("the boss did not die")
		}
	})
	onQueueOf(t, boss, func() { boss.Decay(srv.State, srv.NpcSpawns.RespawnHook(boss.ObjectID())) })
	srv.Settle(t)
	srv.Shutdown(t)
	if row := rowState(t, srv, rowDBKey); row.Status != spawn.StatusDead || row.DBValue != 3 {
		t.Fatalf("saved row = status %d value %d, want dead with 3", row.Status, row.DBValue)
	}

	srv = bootRow(t, log)
	expectLines(t, "restart", log.take(), []string{fmt.Sprintf("%d loaded=true value=3 memo=unset", rowBossID)})
	srv.NpcSpawns.Respawn(rowDBKey)
	srv.Settle(t)
	expectLines(t, "respawn", log.take(), []string{fmt.Sprintf("%d loaded=false value=3 memo=npc %d", rowDBBossID, rowBossID)})
	other := liveOf(t, srv, rowBossID).(*npc.Hostile)
	other.DeleteMe()
	srv.Settle(t)
	srv.NpcSpawns.Respawn(rowBossKey)
	srv.Settle(t)
	expectLines(t, "respawn after the memo's NPC left", log.take(), []string{fmt.Sprintf("%d loaded=true value=6 memo=npc %d", rowBossID, rowDBBossID)})
}

// A spawn restored from a live row stays marked loaded across a respawn
// under the default maker; the database maker clears the mark when its NPC
// leaves, so its respawn is not loaded.
func TestSpawnRowDBMakerClearsLoaded(t *testing.T) {
	store := gamesql.NewSpawnStore(sqltest.SharedDB(t))
	live := func(name string, value int) *spawn.State {
		return &spawn.State{Name: name, Status: spawn.StatusAlive, CurrentHP: 30, CurrentMP: 10, Location: location.Location{X: 100, Y: 100, Z: 30}, DBValue: value}
	}
	if err := store.SaveStates(context.Background(), map[string]*spawn.State{rowBossKey: live(rowBossKey, 1), rowDBKey: live(rowDBKey, 2)}); err != nil {
		t.Fatalf("seed spawn data: %v", err)
	}
	log := &rowLog{}
	srv := bootRow(t, log)
	expectLines(t, "boot", log.take(), []string{
		fmt.Sprintf("%d loaded=true value=1 memo=unset", rowBossID),
		fmt.Sprintf("%d loaded=true value=2 memo=npc %d", rowDBBossID, rowBossID),
	})
	for _, id := range []int{rowBossID, rowDBBossID} {
		boss := liveOf(t, srv, id).(*npc.Hostile)
		if hp := boss.CurrentHP(); hp != 30 {
			t.Fatalf("restored boss %d HP = %d, want the row's 30", id, hp)
		}
		boss.DeleteMe()
	}
	srv.Settle(t)
	for _, key := range []string{rowBossKey, rowDBKey} {
		srv.NpcSpawns.Respawn(key)
	}
	srv.Settle(t)
	expectLines(t, "respawn", log.take(), []string{
		fmt.Sprintf("%d loaded=true value=4 memo=gone", rowBossID),
		fmt.Sprintf("%d loaded=false value=5 memo=npc %d", rowDBBossID, rowBossID),
	})
}
