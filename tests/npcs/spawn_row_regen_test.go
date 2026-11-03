package npcs

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// TestSpawnRowRestoredShortArmsRegenAtSpawn covers an NPC spawned with a
// saved HP below max: its regeneration phase arms as it enters the world,
// before any sweep runs, instead of at the first backstop pass.
func TestSpawnRowRestoredShortArmsRegenAtSpawn(t *testing.T) {
	store := gamesql.NewSpawnStore(sqltest.SharedDB(t))
	live := func(name string) *spawn.State {
		return &spawn.State{Name: name, Status: spawn.StatusAlive, CurrentHP: 30, CurrentMP: 10, Location: location.Location{X: 100, Y: 100, Z: 30}}
	}
	if err := store.SaveStates(context.Background(), map[string]*spawn.State{rowBossKey: live(rowBossKey), rowDBKey: live(rowDBKey)}); err != nil {
		t.Fatalf("seed spawn data: %v", err)
	}
	srv := bootRow(t, &rowLog{})
	boss := liveOf(t, srv, rowBossID).(*npc.Hostile)
	if hp := boss.CurrentHP(); hp != 30 {
		t.Fatalf("restored boss HP = %d, want the row's 30", hp)
	}
	if !boss.Regen().Active() {
		t.Fatal("regeneration idle after the boss spawned short of HP")
	}
}
