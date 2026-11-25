package npcs

import (
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestAuctionEndsWithNoBidOnAFreeHall: free Moonstone Hall's auction ends
// with nobody having bid. Nobody is told anything and the auction runs
// again for a week from its end, stored, the hall still free.
func TestAuctionEndsWithNoBidOnAFreeHall(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, adena: 1_000_000,
		halls: []hallRow{{id: moonstone, endDate: end}},
	})
	if !w.srv.DrivesClock() {
		t.Skip("the auction is waited out on the test clock")
	}
	drainFrames(t, w.c)

	w.srv.Advance(t, time.Hour+time.Minute)
	if got := messages(drainFrames(t, w.c)); len(got) != 0 {
		t.Fatalf("messages at the end = %v, want none", got)
	}
	if row := storedHall(t, w, moonstone); row.owner != 0 || row.endDate != end+7*dayMs {
		t.Fatalf("stored Moonstone Hall = %+v, want free, auction ending %d", row, end+7*dayMs)
	}
	if v, _ := w.srv.Halls.View(moonstone); v.OwnerID != 0 || !v.Auction || v.EndDate != end+7*dayMs || v.Highest != nil {
		t.Fatalf("Moonstone Hall = %+v, want free, its auction running a week more", v)
	}
	if got := clanAdena(t, w, auctionClanID); got != 1_000_000 {
		t.Fatalf("Bidders' adena = %d, want 1000000", got)
	}
}

// TestAuctionEndsWithNoBidOnAHallForSale: Bidders put Moonstone Hall, which
// it owns, up for sale and nobody bids. When the auction ends the clan's
// members in the world are told CLANHALL_NOT_SOLD; the hall stays Bidders'
// and the seller's deposit is not paid back. The sale is dropped, its end
// cleared and the hall off the listing (parity exception, #5018, #5356).
func TestAuctionEndsWithNoBidOnAHallForSale(t *testing.T) {
	t.Parallel()
	end := time.Now().Add(time.Hour).UnixMilli()
	paid := time.Now().Add(72 * time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, adena: 1_000_000,
		halls: []hallRow{{
			id: moonstone, owner: auctionClanID, paid: true, paidUntil: paid,
			sellerBid: 30_000_000, sellerName: "Talker", sellerCln: "Bidders", endDate: end,
		}},
	})
	if !w.srv.DrivesClock() {
		t.Skip("the auction is waited out on the test clock")
	}
	drainFrames(t, w.c)

	w.srv.Advance(t, time.Hour+time.Minute)
	wantMessages(t, "auction end", drainFrames(t, w.c), serverpackets.SystemMessageClanHallNotSold)
	if row := storedHall(t, w, moonstone); row.owner != auctionClanID || !row.paid || row.paidUntil != paid ||
		row.sellerBid != 0 || row.sellerName != "" || row.sellerCln != "" || row.endDate != 0 {
		t.Fatalf("stored Moonstone Hall = %+v, want still Bidders', no sale, no end", row)
	}
	if v, _ := w.srv.Halls.View(moonstone); v.Seller != nil || v.EndDate != 0 {
		t.Fatalf("Moonstone Hall = %+v, want its sale dropped", v)
	}
	for _, v := range w.srv.Halls.Auctionable() {
		if v.ID == moonstone {
			t.Fatal("Moonstone Hall still listed for auction")
		}
	}
	if got := clanAdena(t, w, auctionClanID); got != 1_000_000 {
		t.Fatalf("Bidders' adena = %d, want 1000000: the deposit is not paid back", got)
	}
}

// TestAuctioneerSelectedItemsOfAClanWithNoHallNorBid: a clan that owns no
// hall and bids on none asks the auctioneer for its own offerings. It is
// shown the list of halls for auction, then told NO_OFFERINGS_OWN_OR_MADE_BID_FOR.
func TestAuctioneerSelectedItemsOfAClanWithNoHallNorBid(t *testing.T) {
	t.Parallel()
	w := bootAuction(t, auctionSetup{clanLevel: 2, adena: 1_000_000, halls: everyHall(time.Now().Add(time.Hour).UnixMilli())})
	w.open(t)

	frames := w.bypass(t, "selectedItems")
	var order []byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage || f[0] == serverpackets.OpcodeNpcHtmlMessage {
			order = append(order, f[0])
		}
	}
	if string(order) != string([]byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("frames % x, want the list page then the message", order)
	}
	if page := pageIn(t, frames); !strings.Contains(page, "_bidding 22\">Moonstone Hall") {
		t.Fatalf("page is not the list of halls for auction:\n%s", page)
	}
	wantMessages(t, "selectedItems", frames, serverpackets.SystemMessageNoOfferingsOwnOrMadeBidFor)
}
