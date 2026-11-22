package npcs

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestAuctionBidFormRestoreHoldsNoQueue holds the items table under a
// write lock while a clan whose warehouse no member opened yet opens a
// hall's bid form, which shows the warehouse's adena. While the
// warehouse's rows are read, the bidder's own queue still runs work posted
// to it and the halls still answer; once the lock is released the form
// shows the stored adena, and the bid it leads to is placed and paid.
func TestAuctionBidFormRestoreHoldsNoQueue(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(72 * time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{clanLevel: 2, adena: 30_000_000, halls: []hallRow{{id: moonstone, endDate: end}}})
	oid := w.oid()
	w.open(t)
	w.page(t, "list")
	w.page(t, "bidding 22")
	ctx := context.Background()

	lock, err := w.srv.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := lock.ExecContext(ctx, `LOCK TABLES items WRITE`); err != nil {
		t.Fatal(err)
	}
	unlocked := false
	unlock := func() {
		if !unlocked {
			unlocked = true
			if _, err := lock.ExecContext(ctx, `UNLOCK TABLES`); err != nil {
				t.Fatal(err)
			}
		}
	}
	defer unlock()

	w.c.Send(encodeBypass("npc_" + oid + "_bid1 22"))
	awaitWarehouseReadOnLock(t, w)
	ran := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player).Post(func() { close(ran) }) {
		t.Fatal("the bidder's queue refused the probe")
	}
	<-ran
	if w.srv.Halls.Now() == 0 {
		t.Fatal("the halls answered no time")
	}

	unlock()
	form := pageIn(t, drainFrames(t, w.c))
	want := fill(auctionPage(t, "AgitBid1.htm"),
		"%AGIT_LINK_BACK%", "bypass -h npc_"+oid+"_bidding 22",
		"%PLEDGE_ADENA%", "30000000",
		"%AGIT_AUCTION_MINBID%", "20000000",
		"npc_%objectId%_bid", "npc_"+oid+"_bid 22")
	if form != want {
		t.Fatalf("bid form =\n%s\nwant\n%s", form, want)
	}
	wantMessages(t, "bid", w.bypass(t, "bid 22 25000000"), serverpackets.SystemMessageBidInClanHallAuction)
	if got := clanAdena(t, w, auctionClanID); got != 5_000_000 {
		t.Fatalf("clan adena after the bid = %d, want 5000000", got)
	}
	if bids := storedBids(t, w, moonstone); len(bids) != 1 || bids[0].clan != auctionClanID || bids[0].bid != 25_000_000 {
		t.Fatalf("stored bids = %+v, want the clan's 25000000", bids)
	}
}

// awaitWarehouseReadOnLock returns once a read of an owner's item rows is
// waiting on the items table lock, failing the test if none gets there.
func awaitWarehouseReadOnLock(t *testing.T, w *auctionWorld) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("no item read waited on the items table lock")
		}
		var n int
		if err := w.srv.DB.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM information_schema.PROCESSLIST
			  WHERE db = DATABASE() AND state LIKE 'Waiting for table%' AND info LIKE 'SELECT i.object_id%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
	}
}
