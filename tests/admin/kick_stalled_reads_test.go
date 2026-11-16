package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminKickedStalledClientIsServedNothingMore kicks a player whose
// client has stopped reading but still sends: a whisper it sends after the
// kick is never delivered, the player leaves the world and its logout is
// reported while its writes are still held, and the client reads
// ServerClose as its last frame once it reads again.
func TestAdminKickedStalledClientIsServedNothingMore(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithWriteStalls())
	gm := srv.Client
	enterWorld(t, gm)
	stalled, _ := addPlayer(t, srv, "player2", "Stalled", userLevel)
	other, _ := addPlayer(t, srv, "player3", "Other", userLevel)
	drain(t, gm)
	drain(t, stalled)
	drain(t, other)

	release := srv.StallWrites(t, stalled)
	exchange(t, gm, encodeBuildCmd("kick Stalled"))
	stalled.Send(encodeWhisper("still here", "Other"))
	srv.AwaitLoginLogout(t, "player2")

	for _, frame := range exchange(t, other, encodeGmList()) {
		if frame[0] == serverpackets.OpcodeCreatureSay {
			t.Fatalf("whisper sent after the kick reached its target: %x", testsupport.FrameOpcodes([][]byte{frame}))
		}
	}

	release()
	expectKicked(t, stalled)
}
