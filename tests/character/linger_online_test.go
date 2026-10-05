package character

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// storedOnline reads objID's characters.online column.
func storedOnline(t *testing.T, srv *gameservertest.Server, objID int32) int {
	t.Helper()
	var online int
	if err := srv.DB.QueryRowContext(context.Background(), `SELECT online FROM characters WHERE obj_Id = ?`, objID).Scan(&online); err != nil {
		t.Fatalf("read online: %v", err)
	}
	return online
}

// TestAutosaveWhileLingeringStoresOnline2 saves a player whose connection
// dropped in combat while it lingers in the world: Player.isOnlineInt
// (Player.java:4504-4510) stores 2 for a detached client, 1 for a connected
// one. Once the player leaves, the row reads offline.
func TestAutosaveWhileLingeringStoresOnline2(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	objID := srv.SoleObjectID(t)

	srv.FlushPersistence(t)
	if got := storedOnline(t, srv, objID); got != 1 {
		t.Fatalf("online in game = %d, want 1", got)
	}

	srv.SetPlayerInCombat(t, objID, true)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("a player dropped in combat left the world at once")
	}
	for deadline := time.Now().Add(5 * time.Second); !network.ClientDetached(obj); {
		if time.Now().After(deadline) {
			t.Fatal("the server never noticed the connection drop")
		}
		time.Sleep(time.Millisecond)
	}
	// The first autosave falls due 5 minutes after entering the world.
	srv.TickAutosave(t)
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatal("the player left the world before the lingering autosave")
	}
	if got := storedOnline(t, srv, objID); got != 2 {
		t.Fatalf("online after an autosave while lingering = %d, want 2", got)
	}

	// The in-combat linger is 15 s, past AdvanceUntil's limit.
	for start := c.Now(); ; srv.Advance(t, 100*time.Millisecond) {
		if _, ok := srv.State.Player(objID); !ok {
			break
		}
		if c.Now().Sub(start) > 30*time.Second {
			t.Fatal("the lingering player was still in the world 30 s later")
		}
	}
	srv.FlushPersistence(t)
	if got := storedOnline(t, srv, objID); got != 0 {
		t.Fatalf("online after leaving = %d, want 0", got)
	}
}
