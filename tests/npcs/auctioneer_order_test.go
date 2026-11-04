package npcs

import (
	"fmt"
	"testing"
	"time"
)

// TestAuctionBidderListOrderAfterResize: twelve bids fill three quarters of
// the bid map's sixteen buckets, so the twelfth doubles the table to 32.
// Clans 0x70000020, 0x70000040 and 0x70000010 share bucket 0 of the small
// table, in that bid order (the stored order, highest bid first). The
// doubling moves 0x70000010 to bucket 16 and puts the two that stay in
// bucket 0 in front of each other one at a time, so 0x70000040 now lists
// before 0x70000020, against bid order. Checked against the JDK 21 map
// filled in the same order: 70000040 70000020 70000001 ... 70000009 70000010.
func TestAuctionBidderListOrderAfterResize(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(72 * time.Hour).UnixMilli()
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local).UnixMilli()
	bids := []bidRow{
		{moonstone, 0x70000020, "Ann", "FirstBid", 40_000_000, at},  // bucket 0, stays
		{moonstone, 0x70000040, "Bob", "SecondBid", 39_000_000, at}, // bucket 0, stays
		{moonstone, 0x70000010, "Cid", "ThirdBid", 38_000_000, at},  // bucket 0, moves to 16
	}
	for i := 1; i <= 9; i++ { // buckets 1 to 9 in both tables
		bids = append(bids, bidRow{moonstone, 0x70000000 + i, fmt.Sprintf("Lone%d", i), fmt.Sprintf("Bucket%d", i), 30_000_000 - i*100_000, at})
	}
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, halls: []hallRow{{id: moonstone, endDate: end}},
		bids: bids,
	})
	oid := w.oid()
	w.open(t)
	w.page(t, "list")
	w.page(t, "bidding 22")
	list := w.page(t, "bidlist 22")
	row := func(b bidRow) string {
		return "<tr><td width=90 align=center>" + b.clanName + "</td><td width=90 align=center>" + b.name +
			"</td><td width=90 align=center>" + endDay(b.time) + "</td></tr>"
	}
	rows := row(bids[1]) + row(bids[0])
	for _, b := range bids[3:] {
		rows += row(b)
	}
	rows += row(bids[2])
	want := fill(auctionPage(t, "AgitBidderList.htm"),
		"%AGIT_LIST%", rows,
		"%AGIT_LINK_BACK%", "bypass -h npc_"+oid+"_bidding 22",
		"%objectId%", oid)
	if list != want {
		t.Fatalf("bidders list =\n%s\nwant\n%s", list, want)
	}
}
