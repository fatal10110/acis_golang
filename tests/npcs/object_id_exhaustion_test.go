package npcs

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestMerchantBuyWithoutObjectIDsChargesNothing: a purchase whose new items
// can get no object id is refused unanswered before the stock or the adena
// is taken.
func TestMerchantBuyWithoutObjectIDsChargesNothing(t *testing.T) {
	t.Parallel()
	w := bootShop(t, 10000)
	w.openBuy(t, "1")
	w.srv.ExhaustObjectIDs()

	if frames := w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopSwordID, 1}, buyRow{arrowID, 2})); len(frames) != 0 {
		t.Fatalf("a purchase without object ids answered %x, want nothing", opcodes(frames))
	}
	if adena, swords, arrows := w.countOf(t, item.AdenaID), w.countOf(t, shopSwordID), w.countOf(t, arrowID); adena != 10000 || swords+arrows != 0 {
		t.Fatalf("after the refused purchase: adena %d swords %d arrows %d, want 10000 and nothing bought", adena, swords, arrows)
	}
	if swords, arrows := w.shownCount(t, shopSwordID), w.shownCount(t, arrowID); swords != 2 || arrows != 3 {
		t.Fatalf("window after the refusal: swords %d arrows %d, want 2 and 3", swords, arrows)
	}
	w.srv.FlushPersistence(t)
	if saved, err := w.srv.BuyListRows.LoadStock(context.Background()); err != nil || len(saved) != 0 {
		t.Fatalf("buylists rows after the refusal = %+v, %v; want none", saved, err)
	}
}

// TestMultisellChooseWithoutObjectIDsTakesNoIngredient: an exchange whose
// products can get no object id is refused unanswered before any
// ingredient is taken.
func TestMultisellChooseWithoutObjectIDsTakesNoIngredient(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{item.AdenaID, 1000}, {msOreID, 10}}, nil)
	w.mustOpen(t, "9001")
	w.srv.ExhaustObjectIDs()

	if frames := w.choose(t, "9001", 1, 1); len(frames) != 0 {
		t.Fatalf("an exchange without object ids answered %d frames, want none", len(frames))
	}
	want := map[int32]int{item.AdenaID: 1000, msOreID: 10, msBladeID: 0}
	for id, n := range want {
		if got := w.held(t, id); got != n {
			t.Fatalf("held %d = %d, want %d", id, got, n)
		}
	}
	w.srv.FlushItems(t)
	rows, err := w.srv.Items.ListByOwner(context.Background(), w.player)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int32]int{}
	for _, row := range rows {
		got[row.TemplateID] += row.Count
	}
	for id, n := range want {
		if got[id] != n {
			t.Fatalf("persisted %d = %d, want %d (rows %v)", id, got[id], n, got)
		}
	}
}
