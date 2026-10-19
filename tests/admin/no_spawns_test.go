package admin

import (
	"context"
	"slices"
	"testing"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestNoSpawnsSkipsTheSpawnPass pins server.properties NoSpawns=True: the
// spawn list is read but its on-start makers spawn nothing at boot, nor
// when //respawnall reads the list again (the game masters are still told
// the respawn is complete); a spawn asked for by name still happens.
func TestNoSpawnsSkipsTheSpawnPass(t *testing.T) {
	t.Parallel()
	fresh := newDen(t)
	reloads := network.DataReloads{
		NPCs: func() error { return nil },
		SpawnList: func(_ context.Context, _ *gamemanager.Spawns) (*gamemanager.Spawns, error) {
			return gamemanager.NewSpawns(fresh, nil), nil
		},
	}
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithNPCs(spawnTemplates()), gameservertest.WithNpcSpawns(wolfDen(t)),
		gameservertest.WithDataReloads(reloads), gameservertest.WithNoSpawns())
	gm := srv.Client
	enterWorld(t, gm)

	srv.Settle(t)
	if got := allNpcs(srv); len(got) != 0 {
		t.Fatalf("npcs in the world at boot = %v, want none", got)
	}
	if got := srv.NpcSpawns.Spawns().Table().MakerCount(); got != 1 {
		t.Fatalf("makers read = %d, want the den's", got)
	}

	frames := exchange(t, gm, encodeBuildCmd("respawnall"))
	if got := noticesIn(frames); !slices.Equal(got, []string{"NPCs' respawn is now complete."}) {
		t.Fatalf("game master messages = %q, want the respawn notice", got)
	}
	srv.Settle(t)
	if srv.NpcSpawns.Spawns().Table() != fresh {
		t.Fatal("the reloaded spawn list is not in use")
	}
	if got := allNpcs(srv); len(got) != 0 {
		t.Fatalf("npcs in the world after //respawnall = %v, want none", got)
	}

	exchange(t, gm, encodeBuildCmd("spawn 30001"))
	srv.Settle(t)
	if got := len(npcsOf(srv, grocerID)); got != 1 {
		t.Fatalf("grocers after //spawn = %d, want 1", got)
	}
}
