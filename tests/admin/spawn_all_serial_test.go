package admin

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminRespawnAllTwiceAtOnce runs //respawnall from two game masters at
// once, the first held inside its spawn list reload: the second waits for
// the first to finish and despawns what it placed, so the world ends with
// one population of the new list, not two. It needs two pool workers to
// race the commands; with one it passes trivially.
func TestAdminRespawnAllTwiceAtOnce(t *testing.T) {
	t.Parallel()
	fresh := newDen(t)
	var calls atomic.Int32
	inside := make(chan struct{})
	second := make(chan struct{})
	release := make(chan struct{})
	reloads := network.DataReloads{
		SpawnList: func(_ context.Context, _ *gamemanager.Spawns) (*gamemanager.Spawns, error) {
			switch calls.Add(1) {
			case 1:
				close(inside)
				<-release
			case 2:
				close(second)
			}
			return gamemanager.NewSpawns(fresh, nil), nil
		},
	}
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithNPCs(spawnTemplates()), gameservertest.WithNpcSpawns(wolfDen(t)), gameservertest.WithDataReloads(reloads),
		// The two game masters' commands must run side by side; the inline
		// executor runs every queue on one goroutine.
		gameservertest.WithRealPool())
	first := srv.Client
	enterWorld(t, first)
	other, _ := addPlayer(t, srv, "other", "Other", adminLevel)

	first.Send(encodeBuildCmd("respawnall"))
	select {
	case <-inside:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("the first //respawnall never reached its spawn list reload")
	}
	other.Send(encodeBuildCmd("respawnall"))
	// Unserialized, the second command reloads its spawn list at once; it
	// must instead wait for the first.
	select {
	case <-second:
	case <-time.After(500 * time.Millisecond):
	}
	close(release)

	for _, c := range []*testsupport.ScriptedClient{first, other} {
		frames := testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeManorBarrier()) }, serverpackets.OpcodeExtended)
		if got := noticesIn(frames); !slices.Contains(got, "NPCs' respawn is now complete.") {
			t.Fatalf("game master messages = %q, want the respawn notice", got)
		}
	}
	srv.Settle(t)
	if got := calls.Load(); got != 2 {
		t.Fatalf("spawn list reloads = %d, want 2", got)
	}
	if got := len(npcsOf(srv, wolfID)); got != 2 {
		t.Fatalf("wolves = %d, want the new den's 2", got)
	}
	if got := srv.NpcSpawns.LiveCount(); got != 3 {
		t.Fatalf("live spawned npcs = %d, want one population of 3", got)
	}
}
