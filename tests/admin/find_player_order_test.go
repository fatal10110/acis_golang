package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAdminFindPlayersInOnlineTableOrder: //find player and //find ip walk
// the online players in the order the online-player table lists them, a
// player in bucket id&15 (folded with the id's high half) ahead of one in a
// later bucket, not by object id: BoxLate, whose higher id falls in bucket
// 0, is listed before BoxEarly in bucket 15.
func TestAdminFindPlayersInOnlineTableOrder(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, findPages...)))
	gm := srv.Client
	enterWorld(t, gm)

	bucket := func(id int32) uint32 { h := uint32(id); return (h ^ h>>16) & 15 }
	for bucket(srv.NewObjectID()+1) != 15 {
	}
	early := srv.SeedCharacterFor(t, "boxearly", "BoxEarly", 1, 0)
	late := srv.SeedCharacterFor(t, "boxlate", "BoxLate", 1, 0)
	if late.ID <= early.ID || bucket(late.ID) >= bucket(early.ID) {
		t.Fatalf("fixture ids %d (bucket %d), %d (bucket %d): want the later id in an earlier bucket",
			early.ID, bucket(early.ID), late.ID, bucket(late.ID))
	}
	for _, account := range []string{"boxearly", "boxlate"} {
		c := srv.DialClientFrom(t, account, 1, "127.0.0.9")
		enterWorld(t, c)
	}
	drain(t, gm)

	assertOnePage(t, "//find player box", exchange(t, gm, encodeBuildCmd("find player box")),
		findPlayerPage("box", findPlayerRow(0, "BoxLate"), findPlayerRow(1, "BoxEarly")))
	assertOnePage(t, "//find ip 127.0.0.9", exchange(t, gm, encodeBuildCmd("find ip 127.0.0.9")),
		shippedPage(t, "ipfind.htm", "%number%", "2", "%ip%", "127.0.0.9", "%results%", ipRow("BoxLate")+ipRow("BoxEarly")))
}
