package npcs

import (
	"context"
	"fmt"
	"testing"
	"time"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// A database-tracked boss's death and the db values its scripts change are
// written as they happen, so a server stopped with no shutdown save restarts
// with the boss still dead, its respawn deadline and db value kept, and the
// live boss's changed db value kept too (owner-approved divergence, #4685).
func TestSpawnRowDeathSurvivesAStopWithoutTheShutdownSave(t *testing.T) {
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
	deadline, ok := srv.NpcSpawns.Spawns().RespawnDeadline(rowDBKey, time.Now())
	if !ok {
		t.Fatal("the dead boss's row waits for no respawn")
	}
	srv.Halt(t)

	if row := rowState(t, srv, rowDBKey); row.Status != spawn.StatusDead || row.DBValue != 3 || row.RespawnTime != deadline.UnixMilli() {
		t.Fatalf("saved dead row = status %d value %d respawn %d, want dead with 3 and %d", row.Status, row.DBValue, row.RespawnTime, deadline.UnixMilli())
	}
	if row := rowState(t, srv, rowBossKey); row.Status != spawn.StatusAlive || row.DBValue != 3 {
		t.Fatalf("saved live row = status %d value %d, want alive with 3", row.Status, row.DBValue)
	}

	srv = bootRow(t, log)
	expectLines(t, "restart", log.take(), []string{fmt.Sprintf("%d loaded=true value=3 memo=unset", rowBossID)})
	if got, ok := srv.NpcSpawns.Spawns().RespawnDeadline(rowDBKey, time.Now()); !ok || !got.Equal(time.UnixMilli(deadline.UnixMilli())) {
		t.Fatalf("restored respawn deadline = %v (waiting %v), want %v", got, ok, deadline)
	}
}

// The spawn list a //respawnall puts in place writes its rows as they change
// the way the boot one did.
func TestSpawnRowWritesOutliveARespawnAll(t *testing.T) {
	log := &rowLog{}
	srv := bootRow(t, log)
	srv.NpcSpawns.DespawnAll()
	srv.Settle(t)
	srv.NpcSpawns.RespawnAll(gamemanager.NewSpawns(rowTable(t), nil))
	srv.Settle(t)
	log.take()
	boss := liveOf(t, srv, rowDBBossID).(*npc.Hostile)
	onQueueOf(t, boss, func() {
		if !boss.Die(nil, nil) {
			t.Error("the boss did not die")
		}
	})
	onQueueOf(t, boss, func() { boss.Decay(srv.State, srv.NpcSpawns.RespawnHook(boss.ObjectID())) })
	srv.Halt(t)
	if row := rowState(t, srv, rowDBKey); row.Status != spawn.StatusDead || row.DBValue != 3 {
		t.Fatalf("saved row = status %d value %d, want dead with 3", row.Status, row.DBValue)
	}
}

// A shutdown save with a row the table refuses commits nothing: the rows
// the running server wrote stay as they were (owner-approved divergence,
// #4685).
func TestSpawnRowFailedSaveKeepsTheSavedRows(t *testing.T) {
	log := &rowLog{}
	srv := bootRow(t, log)
	log.take()
	srv.Shutdown(t)
	store := gamesql.NewSpawnStore(srv.DB)
	ctx := context.Background()
	saved, err := store.LoadStates(ctx)
	if err != nil {
		t.Fatalf("load spawn data: %v", err)
	}
	if len(saved) == 0 {
		t.Fatal("the shutdown saved no rows")
	}

	rows := map[string]*spawn.State{"refused": {Name: "refused", Status: spawn.StatusAlive, CurrentHP: -1}}
	for name, state := range saved {
		changed := *state
		changed.DBValue = 9
		rows[name] = &changed
	}
	if err := store.SaveStates(ctx, rows); err == nil {
		t.Fatal("the save with a refused row reported no error")
	}

	after, err := store.LoadStates(ctx)
	if err != nil {
		t.Fatalf("load spawn data: %v", err)
	}
	if len(after) != len(saved) {
		t.Fatalf("rows after the failed save = %d, want the %d saved", len(after), len(saved))
	}
	for name, state := range saved {
		if got := after[name]; got == nil || *got != *state {
			t.Fatalf("row %q after the failed save = %+v, want %+v", name, got, state)
		}
	}
}
