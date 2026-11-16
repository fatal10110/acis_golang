package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/botclient"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
	loginpackets "github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// loginTimeout bounds each step of a scripted login-server session.
const loginTimeout = 10 * time.Second

// expectAccountInUse logs account in at the login server and checks the
// login is refused as already in use.
func expectAccountInUse(t *testing.T, loginAddr, account string) {
	t.Helper()
	_, err := botclient.Login(context.Background(), loginAddr, account, "secret", 1, loginTimeout)
	var refused *botclient.LoginError
	if !errors.As(err, &refused) || refused.Packet != "LoginFail" || refused.Reason != int32(loginpackets.LoginFailAccountInUse) {
		t.Fatalf("login while in play = %v, want LoginFail account in use", err)
	}
}

// TestSecondLoginKicksAccountInPlay logs an account in at the login server
// while its character is in the world here, in combat: the login server
// refuses the login as in use and asks this server to kick the account.
// The client reads ServerClose last and is closed, the character leaves
// the world without the in-combat linger, the login server is told the
// account logged out, and the next login succeeds.
func TestSecondLoginKicksAccountInPlay(t *testing.T) {
	t.Parallel()
	const account = "kickee"
	srv := gameservertest.Boot(t, gameservertest.WithAccount(account), gameservertest.WithCharacter("Kickee", 5, 0), gameservertest.WithWantChars(1))
	loginAddr := srv.StartLoginClients(t)
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)
	srv.SetPlayerInCombat(t, objID, true)
	if !srv.LoginAccountOnline(t, account) {
		t.Fatal("login server does not list the account in play after it entered")
	}

	expectAccountInUse(t, loginAddr, account)
	if got := srv.AwaitLoginKick(t); got != account {
		t.Fatalf("kick handled for %q, want %s", got, account)
	}

	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeServerClose, "kick ServerClose")
	srv.Client.ExpectClosed()
	srv.AwaitLoginLogout(t, account)
	if _, ok := srv.State.Player(objID); ok {
		t.Fatal("kicked character still in the world after its logout was reported")
	}

	res, err := botclient.Login(context.Background(), loginAddr, account, "secret", 1, loginTimeout)
	if err != nil {
		t.Fatalf("login after the kick: %v", err)
	}
	key := link.SessionKey{LoginKey1: res.Keys.LoginKey1, LoginKey2: res.Keys.LoginKey2, PlayKey1: res.Keys.PlayKey1, PlayKey2: res.Keys.PlayKey2}
	again := srv.DialAuthLogin(t, account, key)
	assertFrameOpcode(t, again.Read(), serverpackets.OpcodeCharSelectInfo, "relogin CharSelectInfo")
}

// TestKickForAccountNotHereChangesNothing has the login server hold a
// stale in-play entry for an account no connection here claims: a login
// for it is refused as in use and the kick it sends this server is
// ignored, leaving the player in the world and the login link untouched.
func TestKickForAccountNotHereChangesNothing(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Stayer", 5, 0), gameservertest.WithWantChars(1))
	loginAddr := srv.StartLoginClients(t)
	startInWorld(t, srv.Client)
	srv.ListOnLogin("ghost")

	expectAccountInUse(t, loginAddr, "ghost")
	if got := srv.AwaitLoginKick(t); got != "ghost" {
		t.Fatalf("kick handled for %q, want ghost", got)
	}

	srv.Client.Send(encodeRequestItemList())
	assertFrameOpcode(t, srv.Client.Read(), serverpackets.OpcodeItemList, "item list after the stray kick")
	if !srv.LoginAccountOnline(t, "player1") || !srv.LoginAccountOnline(t, "ghost") {
		t.Fatal("stray kick changed the login server's roster")
	}
}

// TestEvictedRequestApprovalIsNotReportedInPlay holds the login server's
// answers while a second AuthLogin evicts the first: the approval of the
// evicted request, which no client waits on any more, must not report the
// account in play to the login server.
func TestEvictedRequestApprovalIsNotReportedInPlay(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	const account = "lateapproval"
	hold := srv.HoldLoginAnswers()

	issued := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	srv.IssueLoginSession(account, issued)
	first := srv.DialAuthLogin(t, account, issued)
	hold.Await(t, 1)
	second := srv.DialAuthLogin(t, account, link.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4})
	if !first.AwaitClose(5 * time.Second) {
		t.Fatal("evicted connection still open")
	}
	hold.Await(t, 2)
	hold.Release()
	assertFrameOpcode(t, second.Read(), serverpackets.OpcodeAuthLoginFail, "replacement AuthLogin answer")

	if srv.LoginAccountOnline(t, account) {
		t.Fatal("login server lists the account in play on an approval no client waited on")
	}
}

// TestApprovedLoginIsReportedInPlay checks a login whose client waited on
// the approval: by the time the client holds its character list, the
// login server lists the account in play.
func TestApprovedLoginIsReportedInPlay(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	if !srv.LoginAccountOnline(t, "player1") {
		t.Fatal("login server does not list the logged-in account in play")
	}
}
