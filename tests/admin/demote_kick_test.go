package admin

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// readUntilClose returns c's frames ahead of the ServerClose it is
// disconnected with, then requires the connection closed.
func readUntilClose(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		f := c.Read()
		if f[0] == serverpackets.OpcodeServerClose {
			c.ExpectClosed()
			return frames
		}
		frames = append(frames, f)
	}
	t.Fatal("no ServerClose within 100 frames")
	return nil
}

// expectDeleted reads c's frames up to the DeleteObject of objectID.
func expectDeleted(t *testing.T, c *testsupport.ScriptedClient, objectID int32) {
	t.Helper()
	for range 100 {
		f := c.Read()
		if f[0] == serverpackets.OpcodeDeleteObject && int32(binary.LittleEndian.Uint32(f[1:])) == objectID {
			return
		}
	}
	t.Fatalf("no DeleteObject of %d within 100 frames", objectID)
}

// TestAdminSetAccessDemoteKicks pins the owner decision of #5122: an online
// game master moved to a level that is not a game master's is disconnected,
// its logout save storing the level, and the issuing GM is told so; its
// observers see it leave. A move between game masters' levels keeps it in
// the game.
func TestAdminSetAccessDemoteKicks(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", adminLevel)
	watcher, _ := addPlayer(t, srv, "player3", "Watcher", userLevel)
	drain(t, gm)
	drain(t, watcher)
	exchange(t, user, encodeBuildCmd("invul"))
	drain(t, user)

	// Admin to Master: still a game master, still playing.
	_, rest := split(append(exchange(t, gm, encodeBuildCmd("set access Player 8")), settle(t, gm)...))
	assertTexts(t, rest, "Player's access level is now set to 8.")
	if got := onlyUserInfo(t, settle(t, user)); !got.gm {
		t.Fatalf("UserInfo after //set access 8 = %+v, want a GM", got)
	}
	assertGMs(t, exchange(t, user, encodeGmList()), "Admin", "Player")
	drain(t, watcher)

	sent := exchange(t, gm, encodeBuildCmd("set access Player 0"))
	expectKicked(t, user)
	expectDeleted(t, watcher, userID)
	_, rest = split(append(sent, settle(t, gm)...))
	assertTexts(t, withoutDeletes(rest),
		"Player's access level is now set to 0.",
		"Player lost GM status and was disconnected.")
	srv.FlushPersistence(t)
	storedAccessLevel(t, srv, userID, userLevel)
	if got := storedNow(t, srv, userID, "title"); got != "Master" {
		t.Fatalf("stored title = %q, want the Master title kept", got)
	}
	assertGMs(t, withoutDeletes(exchange(t, gm, encodeGmList())), "Admin")
}

// TestAdminSetAccessSelfDemoteKicks pins #5122 on the GM itself: //set
// access without a selection to a level that is not a game master's shows
// the change and both reports, then disconnects it.
func TestAdminSetAccessSelfDemoteKicks(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)

	gm.Send(encodeBuildCmd("set access 6"))
	frames := readUntilClose(t, gm)
	if len(frames) != 3 {
		t.Fatalf("frames = %x, want UserInfo then two reports", testsupport.FrameOpcodes(frames))
	}
	if got := decodeUserInfoAccess(t, frames[0]); got.gm {
		t.Fatalf("UserInfo = %+v, want no GM", got)
	}
	assertTexts(t, frames[1:],
		"Admin's access level is now set to 6.",
		"Admin lost GM status and was disconnected.")
	srv.FlushPersistence(t)
	storedAccessLevel(t, srv, gmID, 6)
}
