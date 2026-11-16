package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAdminKickAllWithStalledClient runs //kick all while one player's
// client has stopped reading, the server's writes to it held: the command
// returns and the GM's next request is answered without waiting on that
// client, the other player is kicked as usual, and the stalled client still
// reads ServerClose as its last frame once it reads again.
func TestAdminKickAllWithStalledClient(t *testing.T) {
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
	exchange(t, gm, encodeBuildCmd("kick all"))
	expectKicked(t, other)
	drain(t, gm)
	assertGMs(t, withoutDeletes(exchange(t, gm, encodeGmList())), "Admin")

	release()
	expectKicked(t, stalled)
	srv.AwaitLoginLogout(t, "player2")
}
