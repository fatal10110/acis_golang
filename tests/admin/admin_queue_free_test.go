package admin

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The admin commands that read or write the database or the data files do
// it off the game master's queue, and off the target's (#4659): while one
// waits, both queues still run work posted to them, and the command answers
// as before once the wait is over.

// TestAdminBanOfflineLookupHoldsNoQueue bans an offline character by name
// while the characters table is locked: the name lookup waits off the game
// master's queue, and the ban lands and is reported once it is unlocked.
func TestAdminBanOfflineLookupHoldsNoQueue(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)

	unlock := srv.LockTable(t, "characters")
	gm.Send(encodeBuildCmd("ban player Player"))
	srv.AwaitLockWait(t, "SELECT obj_Id, char_name")
	srv.AwaitQueueRuns(t, gmID)
	unlock()

	assertTexts(t, settle(t, gm), "Player now has an access level of -1.")
	srv.FlushPersistence(t)
	if got := readAccessLevel(t, srv, ch.ID); got != -1 {
		t.Fatalf("stored access level = %d, want -1", got)
	}
}

// TestAdminSetNameCheckHoldsNoQueue renames a player while the characters
// table is locked: the name check waits off the game master's queue and
// the player's, and the rename goes on once it is unlocked.
func TestAdminSetNameCheckHoldsNoQueue(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)

	unlock := srv.LockTable(t, "characters")
	gm.Send(encodeBuildCmd("set name Renamed7"))
	srv.AwaitLockWait(t, "SELECT COUNT(*) FROM characters WHERE char_name")
	srv.AwaitQueueRuns(t, gmID)
	srv.AwaitQueueRuns(t, userID)
	unlock()

	_, rest := split(settle(t, gm))
	assertTexts(t, rest, "You successfully set your target's name to Renamed7.")
	if got := rowName(t, srv, userID); got != "Renamed7" {
		t.Fatalf("stored name = %q, want Renamed7", got)
	}
}

// TestAdminSetNameStoreHoldsNoQueue renames a player while the characters
// table is locked for reading: the check passes, and the write of the name
// waits off the game master's queue and the player's; once it lands the
// game master is told.
func TestAdminSetNameStoreHoldsNoQueue(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)

	unlock := srv.LockTableRead(t, "characters")
	gm.Send(encodeBuildCmd("set name Renamed8"))
	srv.AwaitLockWait(t, "UPDATE characters SET char_name")
	srv.AwaitQueueRuns(t, gmID)
	srv.AwaitQueueRuns(t, userID)
	unlock()

	_, rest := split(settle(t, gm))
	assertTexts(t, rest, "You successfully set your target's name to Renamed8.")
	if got := rowName(t, srv, userID); got != "Renamed8" {
		t.Fatalf("stored name = %q, want Renamed8", got)
	}
}

// TestAdminRespawnAllHoldsNoQueue holds //respawnall inside its spawn list
// reload: the game master's queue still runs, and the notice comes once the
// reload is over.
func TestAdminRespawnAllHoldsNoQueue(t *testing.T) {
	t.Parallel()
	fresh := newDen(t)
	inside, release := make(chan struct{}), make(chan struct{})
	reloads := network.DataReloads{
		SpawnList: func(context.Context, *gamemanager.Spawns) (*gamemanager.Spawns, error) {
			close(inside)
			<-release
			return gamemanager.NewSpawns(fresh, nil), nil
		},
	}
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithNPCs(spawnTemplates()), gameservertest.WithNpcSpawns(wolfDen(t)), gameservertest.WithDataReloads(reloads))
	gm := srv.Client
	enterWorld(t, gm)

	gm.Send(encodeBuildCmd("respawnall"))
	select {
	case <-inside:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("//respawnall never reached its spawn list reload")
	}
	srv.AwaitQueueRuns(t, gmID)
	close(release)

	if got := noticesIn(settle(t, gm)); len(got) != 1 || got[0] != "NPCs' respawn is now complete." {
		t.Fatalf("game master messages = %q, want the respawn notice", got)
	}
}

// TestAdminReloadHoldsNoQueue holds //reload inside a data read: the game
// master's queue still runs, and the reload is reported once it is over,
// before the next type reloads.
func TestAdminReloadHoldsNoQueue(t *testing.T) {
	t.Parallel()
	inside, release := make(chan struct{}), make(chan struct{})
	var walkers atomic.Int32
	reloads := network.DataReloads{
		HTML: func() error {
			close(inside)
			<-release
			return nil
		},
		WalkerRoutes: func() error {
			walkers.Add(1)
			return nil
		},
	}
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithDataReloads(reloads))
	gm := srv.Client
	enterWorld(t, gm)

	gm.Send(encodeBuildCmd("reload htm npcwalker"))
	select {
	case <-inside:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("//reload never reached its HTML read")
	}
	srv.AwaitQueueRuns(t, gmID)
	close(release)

	assertTexts(t, settle(t, gm), "The HTM cache has been reloaded.", "Walking routes have been reloaded.")
	if got := walkers.Load(); got != 1 {
		t.Fatalf("walker route reloads = %d, want 1", got)
	}
}

// TestAdminBookmarkReadHoldsNoQueue opens the game master's bookmarks for
// the first time while the bookmarks table is locked: the read waits off
// the game master's queue, and the page opens once it is unlocked.
func TestAdminBookmarkReadHoldsNoQueue(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, "bk.htm")))
	gm := srv.Client
	if _, err := srv.DB.ExecContext(context.Background(), "INSERT INTO bookmarks (name, obj_Id, x, y, z) VALUES ('Alpha', ?, 1, 2, 3)", gmID); err != nil {
		t.Fatalf("seed bookmark: %v", err)
	}
	enterWorld(t, gm)

	unlock := srv.LockTable(t, "bookmarks")
	gm.Send(encodeBuildCmd("bk"))
	srv.AwaitLockWait(t, "SELECT name, COALESCE(x, 0)")
	srv.AwaitQueueRuns(t, gmID)
	unlock()

	assertOnePage(t, "//bk", settle(t, gm), bookmarkPage(t, bookmarkRow(0, "Alpha", 1, 2, 3)))
}
