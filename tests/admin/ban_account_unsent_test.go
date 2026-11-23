package admin

import "testing"

// TestAdminBanAccountNotSent pins #5127: with no login server link, //ban
// account and //unban account tell the GM the request was not sent, and an
// online target is neither reported banned nor disconnected.
func TestAdminBanAccountNotSent(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	seedAccount(t, srv, "player2")
	seedAccount(t, srv, "offline")
	srv.DropLoginLink()

	assertTexts(t, exchange(t, gm, encodeBuildCmd("ban account offline")), "Ban request for account offline could not be sent.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unban account offline")), "Unban request for account offline could not be sent.")

	exchange(t, gm, encodeAction(srvObjectID(t, srv, "Player")))
	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("ban account"))), "Ban request for account player2 could not be sent.")
	if _, ok := srv.State.PlayerByName("Player"); !ok {
		t.Fatal("the target of a ban not sent left the world")
	}
	drain(t, user)
	if frames := exchange(t, user, encodeGmList()); len(frames) == 0 {
		t.Fatal("the target of a ban not sent no longer answers")
	}
	accountAccessLevel(t, srv, "player2", userLevel)
	accountAccessLevel(t, srv, "offline", userLevel)
}
