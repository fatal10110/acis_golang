package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// buyWindowRow is one PrivateStoreListBuy row: the seller's object, the
// item, how many of it the row takes from the seller, its price and how
// many units the owner still wants.
type buyWindowRow struct{ objectID, itemID, count, price, quantity int32 }

func decodeBuyWindow(t *testing.T, frame []byte) []buyWindowRow {
	t.Helper()
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // owner
	r.ReadInt32() // seller adena
	rows := make([]buyWindowRow, r.ReadInt32())
	for i := range rows {
		rows[i].objectID, rows[i].itemID = r.ReadInt32(), r.ReadInt32()
		r.ReadUint16() // enchant
		rows[i].count = r.ReadInt32()
		r.ReadInt32()  // reference price
		r.ReadUint16() // padding
		r.ReadInt32()  // body part
		r.ReadUint16() // type2
		rows[i].price, rows[i].quantity = r.ReadInt32(), r.ReadInt32()
	}
	return rows
}

// TestBuyStoreSaleFillsFirstRowOfItem pins which buy row a sale fills
// (TradeList.privateStoreSell then removeItem by item id): the owner wants
// 2 potions at 30, then 5 at 10. Selling 3 at 10 pays the 10-adena price but
// takes the units off the first potion row, which wants nothing more and
// leaves the list; the window then shows the 10-adena row alone, still
// wanting 5, a sale at 30 finds no row, and selling those 5 closes the
// store.
func TestBuyStoreSaleFillsFirstRowOfItem(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.srv.GiveItem(t, h.firstID, potionID, 1)
	h.srv.GiveItem(t, h.firstID, item.AdenaID, 1000)
	sellerPotions := h.srv.GiveItem(t, h.secondID, potionID, 10)
	h.enterAll(t)

	openBuyStore(t, h, buyRow{potionID, 0, 2, 30}, buyRow{potionID, 0, 5, 10})
	window := soleFrame(t, clickStore(t, h.second, h.firstID), serverpackets.OpcodePrivateStoreListBuy, "store window")
	drainUntilQuiet(t, h.first) // the click's TargetSelected
	want := []buyWindowRow{{sellerPotions, potionID, 2, 30, 2}, {sellerPotions, potionID, 5, 10, 5}}
	assertBuyWindow(t, decodeBuyWindow(t, window), want, "opened")

	h.second.Send(encodeRequestPrivateStoreSell(h.firstID, saleRow{sellerPotions, potionID, 0, 3, 10}))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessageS1PurchasedS3S2S, textP("TraderOne"), itemNameP(potionID), numberP(3))
	assertMessage(t, h.first.Read(), serverpackets.SystemMessagePurchasedS3S2SFromS1, textP("TraderTwo"), itemNameP(potionID), numberP(3))
	h.srv.Settle(t)
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateBuy {
		t.Fatalf("owner operate type after the first sale = %d, want buy", got)
	}
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)

	// The target is still the owner: one click opens the window again.
	h.second.Send(encodeAction(h.firstID))
	window = soleFrame(t, drainFrames(t, h.second), serverpackets.OpcodePrivateStoreListBuy, "store window after the sale")
	assertBuyWindow(t, decodeBuyWindow(t, window), []buyWindowRow{{sellerPotions, potionID, 5, 10, 5}}, "after the sale")

	h.second.Send(encodeRequestPrivateStoreSell(h.firstID, saleRow{sellerPotions, potionID, 0, 1, 30}))
	assertSilent(t, h.second, "a sale at the removed row's price")

	h.second.Send(encodeRequestPrivateStoreSell(h.firstID, saleRow{sellerPotions, potionID, 0, 5, 10}))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessageS1PurchasedS3S2S, textP("TraderOne"), itemNameP(potionID), numberP(5))
	h.srv.Settle(t)
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
		t.Fatalf("owner operate type after the list emptied = %d, want none", got)
	}
	h.srv.InventoryUpdates.Tick()
	h.srv.FlushPersistence(t)
	assertItemRows(t, h, h.firstID, "20x9 57x920")
	assertItemRows(t, h, h.secondID, "20x2 57x80")
}

func assertBuyWindow(t *testing.T, got, want []buyWindowRow, when string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: window rows = %+v, want %+v", when, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: window rows = %+v, want %+v", when, got, want)
		}
	}
}
