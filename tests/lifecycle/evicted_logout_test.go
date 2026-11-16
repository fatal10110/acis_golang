package lifecycle

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
)

// TestEvictedConnectionTeardownKeepsAccountOnLogin lets an approved login
// take over an account in play here, then waits for the replaced
// connection's whole teardown: it reports no logout, so the login server
// still lists the account for the newer connection. That connection's own
// end then reports the logout.
func TestEvictedConnectionTeardownKeepsAccountOnLogin(t *testing.T) {
	t.Parallel()
	const account = "player1"
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Holder", 5, 0), gameservertest.WithWantChars(1))
	startInWorld(t, srv.Client)

	issued := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	srv.IssueLoginSession(account, issued)
	replacement := srv.DialAuthLogin(t, account, issued)
	assertFrameOpcode(t, replacement.Read(), serverpackets.OpcodeCharSelectInfo, "replacement CharSelectInfo")
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "replaced holder ServerClose")
	srv.Client.ExpectClosed()
	srv.AwaitConnectionEnded(t, srv.Client)
	if !srv.LoginAccountOnline(t, account) {
		t.Fatal("the replaced connection's teardown logged the account out from under its replacement")
	}

	_ = replacement.Close()
	srv.AwaitConnectionEnded(t, replacement)
	srv.AwaitLoginLogout(t, account)
}
