package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminTargetAgainWhileSittingDown pins a second //target on the
// selected player while the GM is still sitting down: as a click would be,
// the follow is queued behind the posture change and the client released
// with ActionFailed alone.
func TestAdminTargetAgainWhileSittingDown(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	exchange(t, gm, encodeBuildCmd("target Player"))

	gm.Send(encodeChangeWaitType(false))
	settle(t, gm)
	frames := exchange(t, gm, encodeBuildCmd("target Player"))
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("//target while sitting down frames = %x, want ActionFailed alone", testsupport.FrameOpcodes(frames))
	}
}
