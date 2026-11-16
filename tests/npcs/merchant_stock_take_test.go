package npcs

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// shownCount is the count the buy window shows for itemID, -1 when the
// window leaves it out.
func (w *shopWorld) shownCount(t *testing.T, itemID int32) int32 {
	t.Helper()
	_, _, _, rows := decodeBuyList(t, w.openBuy(t, "1")[0])
	for _, row := range rows {
		if row.itemID == itemID {
			return row.count
		}
	}
	return -1
}

// TestMerchantLimitedRowsSummedAgainstStock pins the owner-approved fix of
// #4958, diverging from the reference, which checks each row alone and
// keeps the price of the units it cannot deliver: the rows of one limited
// item are summed against its stock, and a request asking for more than
// the stock holds is refused unanswered, taking nothing, selling nothing
// and saving no stock row. Rows within the stock still sell.
func TestMerchantLimitedRowsSummedAgainstStock(t *testing.T) {
	t.Parallel()
	w := bootShop(t, 10000)
	w.openBuy(t, "1")

	for _, req := range [][]buyRow{
		{{shopSwordID, 1}, {shopSwordID, 1}, {shopSwordID, 1}},
		{{arrowID, 2}, {arrowID, 2}},
		{{shopPotionID, 1}, {arrowID, 1}, {shopSwordID, 1}, {arrowID, 3}},
	} {
		if frames := w.send(t, encodeRequestBuyItem(shopListID, req...)); len(frames) != 0 {
			t.Fatalf("request %v past the stock answered %x, want nothing", req, opcodes(frames))
		}
		if adena, swords, arrows, potions := w.countOf(t, item.AdenaID), w.countOf(t, shopSwordID), w.countOf(t, arrowID), w.countOf(t, shopPotionID); adena != 10000 || swords+arrows+potions != 0 {
			t.Fatalf("after %v: adena %d swords %d arrows %d potions %d, want 10000 and nothing bought", req, adena, swords, arrows, potions)
		}
	}
	if swords, arrows := w.shownCount(t, shopSwordID), w.shownCount(t, arrowID); swords != 2 || arrows != 3 {
		t.Fatalf("window after the refusals: swords %d arrows %d, want 2 and 3", swords, arrows)
	}
	w.srv.FlushPersistence(t)
	if saved, err := w.srv.BuyListRows.LoadStock(context.Background()); err != nil || len(saved) != 0 {
		t.Fatalf("buylists rows after the refusals = %+v, %v; want none", saved, err)
	}

	w.send(t, encodeRequestBuyItem(shopListID, buyRow{arrowID, 1}, buyRow{arrowID, 2}))
	if adena, arrows := w.countOf(t, item.AdenaID), w.countOf(t, arrowID); adena != 10000-3 || arrows != 3 {
		t.Fatalf("after three arrows in two rows: adena %d arrows %d, want %d/3", adena, arrows, 10000-3)
	}
	if shown := w.shownCount(t, arrowID); shown != -1 {
		t.Fatalf("sold-out arrows still shown at %d", shown)
	}
	w.srv.FlushPersistence(t)
	saved, err := w.srv.BuyListRows.LoadStock(context.Background())
	if err != nil || len(saved) != 1 || saved[0].ItemID != arrowID || saved[0].Count != 0 {
		t.Fatalf("buylists rows = %+v, %v; want the arrows' row at 0", saved, err)
	}
}

// TestMerchantLimitedBuyWithoutAdenaKeepsStock pins a purchase of limited
// items the buyer cannot pay for: refused as not enough adena, with the
// stock, the window and the stock rows as they were.
func TestMerchantLimitedBuyWithoutAdenaKeepsStock(t *testing.T) {
	t.Parallel()
	w := bootShop(t, 1500)
	w.openBuy(t, "1")

	frames := w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopSwordID, 1}, buyRow{shopSwordID, 1}))
	assertSystemMessages(t, frames, serverpackets.SystemMessageYouNotEnoughAdena)
	if adena, swords := w.countOf(t, item.AdenaID), w.countOf(t, shopSwordID); adena != 1500 || swords != 0 {
		t.Fatalf("after the unpaid buy: adena %d swords %d, want 1500/0", adena, swords)
	}
	if shown := w.shownCount(t, shopSwordID); shown != 2 {
		t.Fatalf("window swords = %d, want 2", shown)
	}
	w.srv.FlushPersistence(t)
	if saved, err := w.srv.BuyListRows.LoadStock(context.Background()); err != nil || len(saved) != 0 {
		t.Fatalf("buylists rows after the unpaid buy = %+v, %v; want none", saved, err)
	}

	w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopSwordID, 1}))
	if adena, swords := w.countOf(t, item.AdenaID), w.countOf(t, shopSwordID); adena != 500 || swords != 1 {
		t.Fatalf("after one sword: adena %d swords %d, want 500/1", adena, swords)
	}
}

// TestMerchantLastUnitsTwoBuyers pins the owner-approved fix of #4958:
// two players each asking for two of the merchant's three arrows at once
// are served one at a time, so exactly one is charged and gets its
// arrows, and the other is refused unanswered, charged nothing. Each round
// restocks the arrows.
func TestMerchantLastUnitsTwoBuyers(t *testing.T) {
	t.Parallel()
	const rounds = 8
	w := bootShop(t, 1000)
	rivalID := w.srv.SeedCharacterFor(t, "player2", "Rival", playerLevel, 0).ID
	w.srv.GiveItem(t, rivalID, item.AdenaID, 1000)
	rival := &shopWorld{
		folkWorld: &folkWorld{srv: w.srv, c: w.srv.DialClient(t, "player2", 1), player: rivalID, at: w.at},
		merchant:  w.merchant,
		clock:     w.clock,
	}
	startInWorld(t, w.srv, rival.c)
	drainFrames(t, w.c)
	w.openBuy(t, "1")
	rival.openBuy(t, "1")
	drainFrames(t, w.c)

	buyers := []*shopWorld{w, rival}
	for round := range rounds {
		var adena, arrows [2]int
		for i, b := range buyers {
			adena[i], arrows[i] = b.countOf(t, item.AdenaID), b.countOf(t, arrowID)
		}
		req := encodeRequestBuyItem(shopListID, buyRow{arrowID, 2})
		w.c.Send(req)
		rival.c.Send(req)
		drainFrames(t, w.c)
		drainFrames(t, rival.c)

		winners := 0
		for i, b := range buyers {
			paid, got := adena[i]-b.countOf(t, item.AdenaID), b.countOf(t, arrowID)-arrows[i]
			switch {
			case paid == 2 && got == 2:
				winners++
			case paid == 0 && got == 0:
			default:
				t.Fatalf("round %d buyer %d paid %d for %d arrows, want 2 for 2 or nothing", round, i, paid, got)
			}
		}
		if winners != 1 {
			t.Fatalf("round %d: %d buyers served, want exactly 1", round, winners)
		}
		if shown := w.shownCount(t, arrowID); shown != 1 {
			t.Fatalf("round %d: window arrows = %d, want 1 left", round, shown)
		}

		w.clock.advance(time.Hour)
		w.srv.BuyListStock.Restock()
	}
}
