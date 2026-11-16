package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// bootJailLanding boots the GM with the jail pages and zones and Player
// online beside it.
func bootJailLanding(t *testing.T) (srv *gameservertest.Server, user *testsupport.ScriptedClient, userID int32) {
	t.Helper()
	srv = bootPunish(t)
	enterWorld(t, srv.Client)
	user, userID = addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, srv.Client)
	settle(t, user)
	return srv, user, userID
}

// landedTeleport sends Appearing for user's teleport in flight and returns
// the frames it brought, the teleport a pending move makes among them.
func landedTeleport(t *testing.T, user *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	return exchange(t, user, wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
}

// TestJailDuringLandingAppliesOnLanding pins the owner-approved retry of a
// jail refused while the player's own teleport lands (#5121): the GM's
// recall is in flight when the jail comes, so the jail's jump is held, and
// the landing of the recall takes the player on to the jail.
func TestJailDuringLandingAppliesOnLanding(t *testing.T) {
	t.Parallel()
	srv, user, userID := bootJailLanding(t)
	gm := srv.Client

	exchange(t, gm, encodeBuildCmd("recall Player"))
	teleportOf(t, settle(t, user), userID)
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("jail Player 30"))), "Player is jailed for 30 minutes.")
	noTeleport(t, settle(t, user))

	frames := landedTeleport(t, user)
	if at := teleportOf(t, frames, userID); at != jailAt {
		t.Fatalf("teleport after the landing = %v, want the jail %v", at, jailAt)
	}
	if ui, jump := index(frames, serverpackets.OpcodeUserInfo), index(frames, serverpackets.OpcodeTeleportToLocation); ui < 0 || jump < ui {
		t.Fatalf("landing frames = %x, want the landing's UserInfo before the jail teleport", testsupport.FrameOpcodes(frames))
	}
	appear(t, user)
	if x, y, _ := srv.PlayerPosition(t, userID); int32(x) != jailAt[0] || int32(y) != jailAt[1] {
		t.Fatalf("Player stands at %d %d, want in the jail at %v", x, y, jailAt)
	}
	if timer := waitPunishment(t, srv, userID, punishJail); timer != 30*60000 {
		t.Fatalf("stored timer = %d, want 1800000", timer)
	}
}

// TestUnjailDuringJailLandingReleases pins the release refused while the
// jail's own jump lands (#5121): jailed and at once unjailed, the player
// lands in the jail, is then taken to Floran, and stays unpunished.
func TestUnjailDuringJailLandingReleases(t *testing.T) {
	t.Parallel()
	srv, user, userID := bootJailLanding(t)
	gm := srv.Client

	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("jail Player 30"))), "Player is jailed for 30 minutes.")
	if at := teleportOf(t, settle(t, user), userID); at != jailAt {
		t.Fatalf("jail teleport = %v, want %v", at, jailAt)
	}
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("unjail Player"))), "Player has been unjailed.")
	noTeleport(t, settle(t, user))

	if at := teleportOf(t, landedTeleport(t, user), userID); !near(at, floranAt, 20) {
		t.Fatalf("teleport after the landing = %v, want within 20 of Floran %v", at, floranAt)
	}
	appear(t, user)
	if x, y, _ := srv.PlayerPosition(t, userID); !near([3]int32{int32(x), int32(y), floranAt[2]}, floranAt, 20) {
		t.Fatalf("Player stands at %d %d, want within 20 of Floran %v", x, y, floranAt)
	}
	waitPunishment(t, srv, userID, punishNone)
}

// TestJailAndUnjailDuringLandingStayOut pins a jail and its release both
// refused while the GM's recall lands (#5121): neither moves the player on
// once it lands, as the jail no longer holds and the player is in no jail,
// so it stays where the recall took it, unpunished.
func TestJailAndUnjailDuringLandingStayOut(t *testing.T) {
	t.Parallel()
	srv, user, userID := bootJailLanding(t)
	gm := srv.Client

	exchange(t, gm, encodeBuildCmd("recall Player"))
	recalled := teleportOf(t, settle(t, user), userID)
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("jail Player 30"))), "Player is jailed for 30 minutes.")
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("unjail Player"))), "Player has been unjailed.")
	noTeleport(t, settle(t, user))

	noTeleport(t, landedTeleport(t, user))
	if x, y, _ := srv.PlayerPosition(t, userID); int32(x) != recalled[0] || int32(y) != recalled[1] {
		t.Fatalf("Player stands at %d %d, want where the recall took it %v", x, y, recalled)
	}
	waitPunishment(t, srv, userID, punishNone)
}
