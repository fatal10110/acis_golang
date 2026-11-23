package admin

import (
	"testing"
	"time"
)

// TestAdminGMOffDemotionInWindowWins pins #5123: a level change another GM
// makes inside a //gmoff window is not undone when the window ends, even a
// change to the very level //gmoff set; the logout save stores it.
func TestAdminGMOffDemotionInWindowWins(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	other, _ := addPlayer(t, srv, "player2", "Other", adminLevel)
	drain(t, gm)

	assertTexts(t, messages(exchange(t, gm, encodeBuildCmd("gmoff 5"))), "You no longer have GM status, but will be rehabilitated after 5 minutes.")
	drain(t, other)
	assertTexts(t, messages(exchange(t, other, encodeBuildCmd("set access Admin 0"))), "Admin's access level is now set to 0.")
	drain(t, gm)

	srv.Advance(t, 6*time.Minute)
	if frames := exchange(t, gm, encodeGmList()); textAt(frames, "Your previous access level has been rehabilitated.") >= 0 {
		t.Fatal("the //gmoff restore undid the demotion made in its window")
	}
	drain(t, other)
	assertGMs(t, exchange(t, other, encodeGmList()), "Other")

	logout(t, gm)
	leftWorld(t, srv, "Admin")
	srv.FlushPersistence(t)
	if got := readAccessLevel(t, srv, gmID); got != userLevel {
		t.Fatalf("stored access level = %d, want the demotion's %d", got, userLevel)
	}
}

// TestAdminGMOffLeaveInWindowKeepsLevel pins #5123: a GM leaving inside its
// //gmoff window is stored at the level the window gives back, so its next
// session is a GM again instead of staying at the user level.
func TestAdminGMOffLeaveInWindowKeepsLevel(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)

	exchange(t, gm, encodeBuildCmd("gmoff 5"))
	logout(t, gm)
	leftWorld(t, srv, "Admin")
	srv.FlushPersistence(t)
	if got := readAccessLevel(t, srv, gmID); got != adminLevel {
		t.Fatalf("stored access level after leaving in the window = %d, want %d", got, adminLevel)
	}

	back := srv.DialClient(t, srv.Account(), 1)
	enterWorld(t, back)
	assertGMs(t, exchange(t, back, encodeGmList()), "Admin")
}
