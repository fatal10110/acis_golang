package lifecycle

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
)

// TestWrongKeyAuthLoginLeavesOnlineSessionAlone sends AuthLogin for an
// account in the world here, with keys the login server never issued: the
// newcomer is refused and the player in the world stays connected, reading
// no ServerClose. A login with the keys the login server issued then takes
// the account over, and the old connection reads ServerClose and closes.
//
// The account changes hands only once the login server approves the new
// keys: an owner-approved divergence (#4652) from closing the holder as
// soon as the AuthLogin arrives.
func TestWrongKeyAuthLoginLeavesOnlineSessionAlone(t *testing.T) {
	t.Parallel()
	const account = "player1"
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Holder", 5, 0), gameservertest.WithWantChars(1))
	startInWorld(t, srv.Client)

	intruder := srv.DialAuthLogin(t, account, link.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4})
	assertFrameOpcode(t, intruder.Read(), serverpackets.OpcodeAuthLoginFail, "wrong-key AuthLogin answer")
	assertFrameOpcode(t, intruder.Read(), serverpackets.OpcodeServerClose, "wrong-key ServerClose")
	intruder.ExpectClosed()

	srv.Client.Send(encodeRequestItemList())
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeItemList, "holder's item list after the refused AuthLogin")
	if !srv.LoginAccountOnline(t, account) {
		t.Fatal("login server no longer lists the holder's account in play after a refused AuthLogin")
	}

	issued := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	srv.IssueLoginSession(account, issued)
	replacement := srv.DialAuthLogin(t, account, issued)
	assertFrameOpcode(t, replacement.Read(), serverpackets.OpcodeCharSelectInfo, "replacement CharSelectInfo")
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "replaced holder ServerClose")
	srv.Client.ExpectClosed()
	if !srv.LoginAccountOnline(t, account) {
		t.Fatal("login server does not list the account in play for the replacement")
	}
}
