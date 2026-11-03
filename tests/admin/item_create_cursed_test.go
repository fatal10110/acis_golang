package admin

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminItemCreatesCursedWeapons creates a cursed weapon through the
// item creation page, for the GM and for a selected player: each becomes
// its holder once (no assimilation into itself), told it picked the weapon
// up before the page. Unlike //cw set, the addition alone starts no life:
// the reference's activation on an added item leaves the end time at zero
// (CursedWeapon.activate never sets it), so the time left reads back to
// the epoch.
func TestAdminItemCreatesCursedWeapons(t *testing.T) {
	t.Parallel()
	srv, gmID := bootCursedAdmin(t, 0, gameservertest.WithHTMLPages(shippedAdminPages(t, "itemcreation.htm")))
	gm := srv.Client
	enterWorld(t, gm)
	noLife := time.Unix(0, 0).Sub(cursedBase)

	frames := exchange(t, gm, encodeBuildCmd("item 8190"))
	picked := indexOfItemMessage(frames, serverpackets.SystemMessageYouPickedUpS1, zaricheID)
	if picked < 0 || picked == len(frames)-1 {
		t.Fatalf("//item 8190 frames = %x, want the pickup message ahead of the page", testsupport.FrameOpcodes(frames))
	}
	assertPage(t, frames[len(frames)-1:], "Item Creation Menu")
	if holder := onlineCharacter(t, srv, gmID); holder.CursedWeaponID() != zaricheID {
		t.Fatalf("gm holds %d, want Zariche", holder.CursedWeaponID())
	}
	if got := srv.PlayerItemCount(t, gmID, zaricheID); got != 1 {
		t.Fatalf("gm's Zariche = %d, want 1", got)
	}
	if left := srv.CursedWeapons.TimeLeft(zaricheID); left != noLife {
		t.Fatalf("Zariche time left = %v, want %v (no life started)", left, noLife)
	}

	holderClient, holderID := addPlayer(t, srv, "player2", "Holder", userLevel)
	drain(t, gm)
	exchange(t, gm, encodeAction(holderID))
	frames = readUntilPage(t, gm, exchange(t, gm, encodeBuildCmd("item 8689")))
	// The holder's appearance reaches the GM first; the report is last
	// before the page.
	assertTexts(t, frames[len(frames)-2:len(frames)-1], "You have spawned 1 Blood Sword Akamanah (8689) in Holder's inventory.")
	if indexOfItemMessage(settle(t, holderClient), serverpackets.SystemMessageYouPickedUpS1, akamanahID) < 0 {
		t.Fatal("the holder was not told it picked Akamanah up")
	}
	if holder := onlineCharacter(t, srv, holderID); holder.CursedWeaponID() != akamanahID {
		t.Fatalf("holder holds %d, want Akamanah", holder.CursedWeaponID())
	}
	if got := srv.PlayerItemCount(t, holderID, akamanahID); got != 1 {
		t.Fatalf("holder's Akamanah = %d, want 1", got)
	}
	if left := srv.CursedWeapons.TimeLeft(akamanahID); left != noLife {
		t.Fatalf("Akamanah time left = %v, want %v (no life started)", left, noLife)
	}
}
