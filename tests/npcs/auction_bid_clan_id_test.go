package npcs

import (
	"testing"
	"time"
)

// TestAuctionWinnerIsTheBiddingClanByID: Moonstone Hall's only stored bid
// was placed by a clan gone since, under the name Rivals, a clan that
// stands but never bid. At the end the bid resolves to the clan that placed
// it, by id, never to the namesake (#5113): Rivals takes no hall and is
// refunded nothing, and the auction runs again a week from its end.
func TestAuctionWinnerIsTheBiddingClanByID(t *testing.T) {
	t.Parallel()
	const goneClanID = 0x70000029
	end := time.Now().Add(time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2,
		halls:     []hallRow{{id: moonstone, endDate: end}},
		bids:      []bidRow{{moonstone, goneClanID, "Ghost", "Rivals", 25_000_000, 1}},
	})
	if !w.srv.DrivesClock() {
		t.Skip("the auction is waited out on the test clock")
	}
	drainFrames(t, w.c)

	w.srv.Advance(t, time.Hour+time.Minute)
	if row := storedHall(t, w, moonstone); row.owner != 0 || row.endDate != end+7*dayMs {
		t.Fatalf("stored Moonstone Hall = %+v, want free, auction ending %d", row, end+7*dayMs)
	}
	if cl, _ := w.srv.Clans.Table().Get(rivalClanID); cl.HallID() != 0 {
		t.Fatalf("Rivals' hall = %d, want none", cl.HallID())
	}
	if got := clanAdena(t, w, rivalClanID); got != 0 {
		t.Fatalf("Rivals' adena = %d, want 0: it never bid", got)
	}
	if bids := storedBids(t, w, moonstone); len(bids) != 0 {
		t.Fatalf("stored bids = %+v, want none", bids)
	}
}
