package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestClearedJailReleasedAtLogin pins #5128: a stored jail cleared by name
// with //unban chat, which leaves the character in the jail, no longer
// strands it there: its next login takes it to Floran village.
func TestClearedJailReleasedAtLogin(t *testing.T) {
	t.Parallel()
	srv := bootPunish(t)
	gm := srv.Client
	enterWorld(t, gm)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("jail Player")), "Player has been jailed.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban chat Player")), "Player's chat ban has been lifted.")
	srv.FlushPersistence(t)
	if level, _ := storedPunishment(t, srv, ch.ID); level != punishNone {
		t.Fatalf("stored punishment = %d, want none", level)
	}
	if at := storedPosition(t, srv, ch.ID); at != jailAt {
		t.Fatalf("stored position = %v, want the jail %v", at, jailAt)
	}

	user := srv.DialClient(t, "player2", 1)
	frames := burst(t, user)
	teleport := index(frames, serverpackets.OpcodeTeleportToLocation)
	if teleport < 0 {
		t.Fatalf("burst = %x, want the release teleport", testsupport.FrameOpcodes(frames))
	}
	if _, at := teleportTo(t, frames[teleport]); !near(at, floranAt, 20) {
		t.Fatalf("release teleport = %v, want within 20 of Floran %v", at, floranAt)
	}
	appear(t, user)

	logout(t, user)
	leftWorld(t, srv, "Player")
	srv.FlushPersistence(t)
	if at := storedPosition(t, srv, ch.ID); !near(at, floranAt, 20) {
		t.Fatalf("stored position = %v, want within 20 of Floran %v", at, floranAt)
	}
	if level, _ := storedPunishment(t, srv, ch.ID); level != punishNone {
		t.Fatalf("stored punishment after the login = %d, want none", level)
	}
}
