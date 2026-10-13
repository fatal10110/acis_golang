package npcs

import (
	"context"
	"sync"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// Spawn-data saves, each after a sync of the live rows as the shutdown save
// does, run without pause while a database-tracked boss dies, decays and
// respawns over and over. Under -race no save reads a row a decay, a
// respawn or a script writes meanwhile, and the row the shutdown saves is
// the last life's: alive, with every created hook's db value counted.
func TestSpawnRowSaveRacesDecayAndRespawn(t *testing.T) {
	const lives = 6
	log := &rowLog{}
	srv := bootRow(t, log)
	log.take()
	store := gamesql.NewSpawnStore(srv.DB)

	stop := make(chan struct{})
	var saver sync.WaitGroup
	saver.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			srv.NpcSpawns.SyncPersistedState()
			if err := srv.NpcSpawns.Spawns().Save(context.Background(), store); err != nil {
				t.Errorf("save spawn data: %v", err)
				return
			}
		}
	})

	for range lives {
		boss := liveOf(t, srv, rowDBBossID).(*npc.Hostile)
		onQueueOf(t, boss, func() {
			if !boss.Die(nil, nil) {
				t.Error("the boss did not die")
			}
		})
		onQueueOf(t, boss, func() { boss.Decay(srv.State, srv.NpcSpawns.RespawnHook(boss.ObjectID())) })
		srv.Settle(t)
		srv.NpcSpawns.Respawn(rowDBKey)
		srv.Settle(t)
	}
	close(stop)
	saver.Wait()

	srv.Shutdown(t)
	if row := rowState(t, srv, rowDBKey); row.Status != spawn.StatusAlive || row.RespawnTime != 0 || row.CurrentHP <= 0 || row.DBValue != 3*(lives+1) {
		t.Fatalf("saved row = status %d respawn %d hp %d value %d, want alive with no respawn time, its HP and value %d",
			row.Status, row.RespawnTime, row.CurrentHP, row.DBValue, 3*(lives+1))
	}
}
