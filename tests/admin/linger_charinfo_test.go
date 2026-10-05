package admin

import (
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestCharacterPageShowsLingeringPlayerDisconnected opens //debug on a
// player whose connection dropped in combat while it lingers in the world:
// the page's address reads "Disconnected" (AdminEditChar.java:586, the
// client detached at once by GameClient.onDisconnection).
func TestCharacterPageShowsLingeringPlayerDisconnected(t *testing.T) {
	t.Parallel()
	srv, gm, user, userID := bootEditAdmin(t)

	srv.SetPlayerInCombat(t, userID, true)
	if err := user.Close(); err != nil {
		t.Fatal(err)
	}
	obj, ok := srv.State.Player(userID)
	if !ok {
		t.Fatal("a player dropped in combat left the world at once")
	}
	for deadline := time.Now().Add(5 * time.Second); !network.ClientDetached(obj); {
		if time.Now().After(deadline) {
			t.Fatal("the server never noticed the connection drop")
		}
		time.Sleep(time.Millisecond)
	}

	pages := only(exchange(t, gm, encodeBuildCmd("debug Player")), serverpackets.OpcodeNpcHtmlMessage)
	if len(pages) != 1 {
		t.Fatalf("//debug answered %d pages, want one", len(pages))
	}
	page := htmlBody(t, pages[0])
	if !strings.Contains(page, ">Disconnected</a>") || strings.Contains(page, "127.0.0.1") {
		t.Fatalf("//debug page of a lingering player does not read Disconnected:\n%s", page)
	}
	if _, ok := srv.State.Player(userID); !ok {
		t.Fatal("the player left the world before the page was read")
	}
}
