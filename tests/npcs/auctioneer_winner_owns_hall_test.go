package npcs

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The gates below are an owner-approved divergence (#5110): a clan never
// holds two halls through the auction, and a dissolving clan does not bid.

// assignHall gives free hall hallID to clanID as the admin command does.
func assignHall(t *testing.T, w *auctionWorld, hallID, clanID int) {
	t.Helper()
	cl, ok := w.srv.Clans.Table().Get(int32(clanID))
	if !ok {
		t.Fatalf("no clan %#x", clanID)
	}
	if got := w.srv.Halls.Assign(int32(hallID), cl); got != clanhall.Assigned {
		t.Fatalf("assign hall %d = %v", hallID, got)
	}
}

// TestAuctionRestartsWhenTheWinnerOwnsAHall: Bidders stands highest on
// free Moonstone Hall, then comes to own Onyx Hall. At the end it does not
// take Moonstone Hall: nobody is told of a winner, every bid comes back
// minus 10% (the highest too), and the auction runs again a week from the
// end. Bidders keeps Onyx Hall alone.
func TestAuctionRestartsWhenTheWinnerOwnsAHall(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, bidAt: moonstone, rivalBidAt: moonstone,
		halls: []hallRow{{id: moonstone, endDate: end}, {id: onyx, endDate: end + 72*hourMs}},
		bids: []bidRow{
			{moonstone, auctionClanID, "Talker", "Bidders", 25_000_000, 1},
			{moonstone, rivalClanID, "Rival", "Rivals", 22_000_000, 2},
		},
	})
	if !w.srv.DrivesClock() {
		t.Skip("the auction is waited out on the test clock")
	}
	assignHall(t, w, onyx, auctionClanID)
	drainFrames(t, w.c)

	w.srv.Advance(t, time.Hour+time.Minute)
	frames := drainFrames(t, w.c)
	if got := messages(frames); len(got) != 0 {
		t.Fatalf("messages at the end = %v, want none", got)
	}
	if _, ok := firstOpcode(frames, serverpackets.OpcodePledgeShowInfoUpdate); ok {
		t.Fatalf("clan header sent at the end: % x", opcodes(frames))
	}
	if row := storedHall(t, w, moonstone); row.owner != 0 || row.paid || row.endDate != end+7*dayMs {
		t.Fatalf("stored Moonstone Hall = %+v, want free, auction ending %d", row, end+7*dayMs)
	}
	if row := storedHall(t, w, onyx); row.owner != auctionClanID {
		t.Fatalf("stored Onyx Hall = %+v, want Bidders owning it", row)
	}
	if v, _ := w.srv.Halls.View(moonstone); v.OwnerID != 0 || !v.Auction || v.EndDate != end+7*dayMs || len(v.Bidders) != 0 || v.Highest != nil {
		t.Fatalf("Moonstone Hall = %+v, want free, its auction run again without bids", v)
	}
	if cl, _ := w.srv.Clans.Table().Get(auctionClanID); cl.HallID() != onyx {
		t.Fatalf("Bidders' hall = %d, want Onyx Hall", cl.HallID())
	}
	if bids := storedBids(t, w, moonstone); len(bids) != 0 {
		t.Fatalf("stored bids = %+v, want none", bids)
	}
	if got := clanAdena(t, w, auctionClanID); got != 22_500_000 {
		t.Fatalf("Bidders' adena = %d, want 22500000", got)
	}
	if got := clanAdena(t, w, rivalClanID); got != 19_800_000 {
		t.Fatalf("Rivals' adena = %d, want 19800000", got)
	}
	for _, id := range []int{auctionClanID, rivalClanID} {
		if at := storedBidAt(t, w, id); at != 0 {
			t.Fatalf("clan %#x auction_bid_at = %d, want 0", id, at)
		}
	}
}

// TestAuctionRestartsWhenTheWinnerIsDissolving: Bidders, standing highest
// on free Moonstone Hall, asked to be dissolved since it bid. At the end it
// does not take the hall: its bid comes back minus 10% and the auction runs
// again a week from the end.
func TestAuctionRestartsWhenTheWinnerIsDissolving(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(time.Hour).UnixMilli()
	setup := auctionSetup{
		clanLevel: 2, bidAt: moonstone,
		halls: []hallRow{{id: moonstone, endDate: end}},
		bids:  []bidRow{{moonstone, auctionClanID, "Talker", "Bidders", 25_000_000, 1}},
	}
	w := bootAuction(t, setup, gameservertest.WithClanSeed(func(db *sql.DB) {
		seedAuction(t, db, setup)
		if _, err := db.ExecContext(context.Background(), "UPDATE clan_data SET dissolving_expiry_time = ? WHERE clan_id = ?",
			end+48*hourMs, auctionClanID); err != nil {
			t.Fatal(err)
		}
	}))
	if !w.srv.DrivesClock() {
		t.Skip("the auction is waited out on the test clock")
	}
	w.srv.Advance(t, time.Hour+time.Minute)
	if frames := drainFrames(t, w.c); len(messages(frames)) != 0 {
		t.Fatalf("messages at the end = %v, want none", messages(frames))
	}
	if row := storedHall(t, w, moonstone); row.owner != 0 || row.endDate != end+7*dayMs {
		t.Fatalf("stored Moonstone Hall = %+v, want free, auction ending %d", row, end+7*dayMs)
	}
	if cl, _ := w.srv.Clans.Table().Get(auctionClanID); cl.HallID() != 0 {
		t.Fatalf("Bidders' hall = %d, want none", cl.HallID())
	}
	if got := clanAdena(t, w, auctionClanID); got != 22_500_000 {
		t.Fatalf("Bidders' adena = %d, want 22500000", got)
	}
	if bids := storedBids(t, w, moonstone); len(bids) != 0 {
		t.Fatalf("stored bids = %+v, want none", bids)
	}
}

// TestAuctionSaleRestartsWhenTheWinnerOwnsAHall: Bidders sells Moonstone
// Hall and Rivals, the only bidder, comes to own Onyx Hall. At the end the
// sale stands, a week further, its deposit still held: Bidders keeps the
// hall and is paid nothing, and Rivals gets its bid back minus 10%.
func TestAuctionSaleRestartsWhenTheWinnerOwnsAHall(t *testing.T) {
	t.Parallel()
	paid := time.Now().Add(72 * time.Hour).UnixMilli()
	end := time.Now().Add(time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, rivalBidAt: moonstone,
		halls: []hallRow{
			{
				id: moonstone, owner: auctionClanID, paid: true, paidUntil: paid,
				sellerBid: 30_000_000, sellerName: "Talker", sellerCln: "Bidders", endDate: end,
			},
			{id: onyx, endDate: end + 72*hourMs},
		},
		bids: []bidRow{{moonstone, rivalClanID, "Rival", "Rivals", 35_000_000, 1}},
	})
	if !w.srv.DrivesClock() {
		t.Skip("the auction is waited out on the test clock")
	}
	assignHall(t, w, onyx, rivalClanID)
	drainFrames(t, w.c)

	w.srv.Advance(t, time.Hour+time.Minute)
	if frames := drainFrames(t, w.c); len(messages(frames)) != 0 {
		t.Fatalf("messages at the end = %v, want none", messages(frames))
	}
	row := storedHall(t, w, moonstone)
	if row.owner != auctionClanID || row.sellerBid != 30_000_000 || row.sellerName != "Talker" || row.sellerCln != "Bidders" || row.endDate != end+7*dayMs {
		t.Fatalf("stored Moonstone Hall = %+v, want Bidders' sale standing until %d", row, end+7*dayMs)
	}
	if got := clanAdena(t, w, auctionClanID); got != 0 {
		t.Fatalf("selling clan's adena = %d, want 0", got)
	}
	if got := clanAdena(t, w, rivalClanID); got != 31_500_000 {
		t.Fatalf("Rivals' adena = %d, want 31500000", got)
	}
	if row := storedHall(t, w, onyx); row.owner != rivalClanID {
		t.Fatalf("stored Onyx Hall = %+v, want Rivals owning it", row)
	}
	if bids := storedBids(t, w, moonstone); len(bids) != 0 {
		t.Fatalf("stored bids = %+v, want none", bids)
	}
	if at := storedBidAt(t, w, rivalClanID); at != 0 {
		t.Fatalf("Rivals' auction_bid_at = %d, want 0", at)
	}
}

// TestAuctionDissolvingClanMayNotBid: Bidders asked to be dissolved. The bid
// form is refused with the list and 680, and a bid with 680's outcome:
// nothing is taken from the warehouse and no bid is stored.
func TestAuctionDissolvingClanMayNotBid(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(72 * time.Hour).UnixMilli()
	setup := auctionSetup{clanLevel: 2, adena: 30_000_000, halls: everyHall(end)}
	w := bootAuction(t, setup, gameservertest.WithClanSeed(func(db *sql.DB) {
		seedAuction(t, db, setup)
		if _, err := db.ExecContext(context.Background(), "UPDATE clan_data SET dissolving_expiry_time = ? WHERE clan_id = ?",
			time.Now().Add(48*time.Hour).UnixMilli(), auctionClanID); err != nil {
			t.Fatal(err)
		}
	}))
	w.open(t)
	w.page(t, "list")
	w.page(t, "bidding 22")
	frames := w.bypass(t, "bid1 22")
	if page := pageIn(t, frames); !strings.Contains(page, "_bidding 22\">Moonstone Hall") {
		t.Fatalf("refusal page is not the list:\n%s", page)
	}
	wantMessages(t, "bid1 22", frames, serverpackets.SystemMessageCannotParticipateInAuction)

	cl, ok := w.srv.Clans.Table().Get(auctionClanID)
	if !ok {
		t.Fatal("no Bidders")
	}
	if got := w.srv.Halls.Bid(moonstone, cl, "Talker", 25_000_000); got != clanhall.BidNotAllowed {
		t.Fatalf("bid = %v, want refused", got)
	}
	if got := clanAdena(t, w, auctionClanID); got != 30_000_000 {
		t.Fatalf("clan adena = %d, want 30000000 untouched", got)
	}
	if bids := storedBids(t, w, moonstone); len(bids) != 0 {
		t.Fatalf("stored bids = %+v, want none", bids)
	}
}
