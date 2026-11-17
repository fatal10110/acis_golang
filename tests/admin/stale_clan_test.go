package admin

import "testing"

// TestAdminSetTitleStaleClan sets the title of a player whose row names a
// clan that no longer exists: the selection leaves it clanless, its row
// too, so the next save stores the title.
func TestAdminSetTitleStaleClan(t *testing.T) {
	t.Parallel()
	srv, gm, user, userID := bootEditAdmin(t, "UPDATE characters SET clanid = 987654 WHERE obj_Id = ?")
	if target := onlineCharacter(t, srv, userID); target.ClanID() != 0 {
		t.Fatalf("clan id = %d after selection, want 0", target.ClanID())
	}
	storedColumn(t, srv, userID, "clanid", "0")
	selectPlayer(t, gm, user, userID)

	exchange(t, gm, encodeBuildCmd("set title Hero"))
	settle(t, user)
	settle(t, gm)
	srv.TickAutosave(t)
	storedColumn(t, srv, userID, "title", "Hero")
}
