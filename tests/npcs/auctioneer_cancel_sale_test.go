package npcs

import (
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestAuctionSaleCancelled: the owner withdraws its registered sale
// through the cancel page. Every bidder gets its bid back minus 10%, the
// deposit is lost, and the hall stays its owner's with no sale, no end
// date and no bids left; the owner is told the cancel and shown the chat
// window again.
func TestAuctionSaleCancelled(t *testing.T) {
	t.Parallel()
	paid := time.Now().Add(72 * time.Hour).UnixMilli()
	end := time.Now().Add(48 * time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, adena: 1_000_000, rivalBidAt: moonstone,
		halls: []hallRow{{
			id: moonstone, owner: auctionClanID, paid: true, paidUntil: paid,
			sellerBid: 30_000_000, sellerName: "Talker", sellerCln: "Bidders", endDate: end,
		}},
		bids: []bidRow{{moonstone, rivalClanID, "Rival", "Rivals", 35_000_000, 1}},
	})
	oid := w.oid()
	chat := w.open(t)

	saleInfo := w.page(t, "selectedItems")
	for _, want := range []string{">Moonstone Hall<", ">Bidders<", ">Talker<", ">30000000<", "bypass -h npc_" + oid + "_cancelAuction"} {
		if !strings.Contains(saleInfo, want) {
			t.Fatalf("sale info page lacks %q:\n%s", want, saleInfo)
		}
	}
	cancel := w.page(t, "cancelAuction")
	if want := fill(auctionPage(t, "AgitSaleCancel.htm"), "%AGIT_DEPOSIT%", "500000",
		"%AGIT_LINK_BACK%", "bypass -h npc_"+oid+"_selectedItems", "%objectId%", oid); cancel != want {
		t.Fatalf("sale cancel page =\n%s\nwant\n%s", cancel, want)
	}

	frames := w.bypass(t, "doCancelAuction")
	wantMessages(t, "cancel sale", frames, serverpackets.SystemMessageCanceledBid)
	var order []byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage || f[0] == serverpackets.OpcodeNpcHtmlMessage {
			order = append(order, f[0])
		}
	}
	if string(order) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage}) {
		t.Fatalf("cancel frames % x, want the notice then the chat window", order)
	}
	if got := pageIn(t, frames); got != chat {
		t.Fatalf("page after the cancel =\n%s\nwant the chat window\n%s", got, chat)
	}

	if got := clanAdena(t, w, rivalClanID); got != 31_500_000 {
		t.Fatalf("bidding clan's adena = %d, want 35000000 minus 10%%", got)
	}
	if got := clanAdena(t, w, auctionClanID); got != 1_000_000 {
		t.Fatalf("selling clan's adena = %d, want 1000000: the deposit is not returned", got)
	}
	row := storedHall(t, w, moonstone)
	if row.owner != auctionClanID || !row.paid || row.paidUntil != paid ||
		row.sellerBid != 0 || row.sellerName != "" || row.sellerCln != "" || row.endDate != 0 {
		t.Fatalf("stored hall = %+v, want Bidders still owning it, no sale, endDate 0", row)
	}
	if bids := storedBids(t, w, moonstone); len(bids) != 0 {
		t.Fatalf("stored bids after the cancel = %+v, want none", bids)
	}
	if at := storedBidAt(t, w, rivalClanID); at != 0 {
		t.Fatalf("bidding clan's auction_bid_at = %d, want 0", at)
	}
	if got := w.srv.Halls.BidAt(rivalClanID); got != 0 {
		t.Fatalf("bidding clan bids at %d, want nowhere", got)
	}
	if v, _ := w.srv.Halls.View(moonstone); v.Seller != nil || v.OwnerID != auctionClanID || len(v.Bidders) != 0 {
		t.Fatalf("hall after the cancel = %+v, want owned by Bidders, no sale, no bids", v)
	}
}
