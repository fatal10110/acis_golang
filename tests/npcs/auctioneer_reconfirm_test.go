package npcs

import (
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
)

// TestAuctionSaleReconfirmTakesNoSecondDeposit: Talker sets up the sale of
// Moonstone Hall, and the same sale is registered (by another member of
// the clan holding the right) before Talker confirms. Talker's confirm is
// answered with the registered sale's page and nothing else: no second
// deposit is taken and the sale's terms and end stay as they were stored
// (#5018).
func TestAuctionSaleReconfirmTakesNoSecondDeposit(t *testing.T) {
	t.Parallel()
	paid := time.Now().Add(72 * time.Hour).UnixMilli()
	w := bootAuction(t, auctionSetup{
		clanLevel: 2, adena: 1_000_000,
		halls: everyHall(paid, hallRow{id: moonstone, owner: auctionClanID, paid: true, paidUntil: paid, endDate: paid}),
	})
	oid := w.oid()
	w.open(t)
	w.page(t, "selectedItems")
	w.page(t, "sale")
	w.page(t, "sale2")
	if summary := w.page(t, "auction 7 30000000"); !strings.Contains(summary, "bypass -h npc_"+oid+"_confirmAuction") {
		t.Fatalf("sale summary lacks the confirm link:\n%s", summary)
	}
	cl, ok := w.srv.Clans.Table().Get(auctionClanID)
	if !ok {
		t.Fatal("no clan Bidders")
	}
	if got := w.srv.Halls.ConfirmSale(cl); got != clanhall.SaleRegistered {
		t.Fatalf("first confirm = %v, want SaleRegistered", got)
	}
	registered := storedHall(t, w, moonstone)
	if got := clanAdena(t, w, auctionClanID); got != 500_000 {
		t.Fatalf("clan adena after the deposit = %d, want 500000", got)
	}

	frames := w.bypass(t, "confirmAuction")
	saleInfo := pageIn(t, frames)
	for _, want := range []string{">Moonstone Hall<", ">Bidders<", ">Talker<", ">30000000<", "bypass -h npc_" + oid + "_cancelAuction"} {
		if !strings.Contains(saleInfo, want) {
			t.Fatalf("answer to the second confirm lacks %q:\n%s", want, saleInfo)
		}
	}
	if got := messages(frames); len(got) != 0 {
		t.Fatalf("messages on the second confirm = %v, want none", got)
	}
	if got := clanAdena(t, w, auctionClanID); got != 500_000 {
		t.Fatalf("clan adena after the second confirm = %d, want 500000: no second deposit", got)
	}
	if row := storedHall(t, w, moonstone); row != registered {
		t.Fatalf("stored hall after the second confirm = %+v, want %+v unchanged", row, registered)
	}
}
