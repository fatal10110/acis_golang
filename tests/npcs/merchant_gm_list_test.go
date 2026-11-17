package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMerchantRefusesGMListToPlayer pins a deliberate divergence: a list no
// NPC sells (npc id 0 or below) opens only through //buy, so a player who is
// not a GM sending it is refused even for a priced row, with nothing
// answered and nothing charged. The reference gates only lists an NPC
// sells, which would let any client buy from a GM list by its id.
func TestMerchantRefusesGMListToPlayer(t *testing.T) {
	t.Parallel()
	const gmListID = 3
	gmShop := buylist.List{ID: gmListID, NPCID: -1, Products: []buylist.Product{
		{BuyListID: gmListID, ItemID: shopPotionID, Price: 50, MaxCount: -1},
	}}
	w := bootShop(t, 1000, gameservertest.WithBuyLists(gmShop))

	if got := w.send(t, encodeRequestBuyItem(gmListID, buyRow{shopPotionID, 2})); len(got) != 0 {
		t.Fatalf("priced row of a GM list = %x, want nothing", opcodes(got))
	}
	if adena, potions := w.countOf(t, item.AdenaID), w.countOf(t, shopPotionID); adena != 1000 || potions != 0 {
		t.Fatalf("after the GM list buy: adena %d potions %d, want 1000/0", adena, potions)
	}

	// The same item at the same price sells from the merchant's own list.
	w.send(t, encodeRequestBuyItem(shopListID, buyRow{shopPotionID, 2}))
	if adena, potions := w.countOf(t, item.AdenaID), w.countOf(t, shopPotionID); adena != 900 || potions != 2 {
		t.Fatalf("after the merchant list buy: adena %d potions %d, want 900/2", adena, potions)
	}
}
