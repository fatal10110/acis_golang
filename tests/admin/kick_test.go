package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// expectKicked reads c's frames up to the ServerClose it is disconnected
// with, then requires the connection closed.
func expectKicked(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for range 100 {
		if c.Read()[0] == serverpackets.OpcodeServerClose {
			c.ExpectClosed()
			return
		}
	}
	t.Fatal("no ServerClose within 100 frames")
}

// TestAdminKick pins //kick (AdminPunish.java:131-163): a named player, or
// the selected one when no player has that name, is disconnected with
// ServerClose; //kick all disconnects every player that is not a GM; no
// argument answers the usage and a name with no selection is an invalid
// target.
func TestAdminKick(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	first, _ := addPlayer(t, srv, "player2", "First", userLevel)
	second, secondID := addPlayer(t, srv, "player3", "Second", userLevel)
	third, _ := addPlayer(t, srv, "player4", "Third", userLevel)
	otherGM, _ := addPlayer(t, srv, "player5", "OtherGM", adminLevel)
	drain(t, gm)
	drain(t, first)
	drain(t, second)
	drain(t, third)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("kick")), "Usage : //kick [all|name]")
	frames := exchange(t, gm, encodeBuildCmd("kick Nobody"))
	if len(frames) != 1 {
		t.Fatalf("//kick Nobody frames = %x, want INVALID_TARGET", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)

	exchange(t, gm, encodeBuildCmd("kick first"))
	expectKicked(t, first)

	// An unknown name falls back to the selected player.
	exchange(t, gm, encodeAction(secondID))
	exchange(t, gm, encodeBuildCmd("kick Nobody"))
	expectKicked(t, second)

	exchange(t, gm, encodeBuildCmd("kick all"))
	expectKicked(t, third)
	// Game masters stay.
	drain(t, otherGM)
	assertGMs(t, exchange(t, otherGM, encodeGmList()), "Admin", "OtherGM")
	drain(t, gm)
	assertGMs(t, exchange(t, gm, encodeGmList()), "Admin", "OtherGM")
}
