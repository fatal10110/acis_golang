package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestSellOntoHeldAdenaSpendsNoObjectID: the proceeds of a sale join the
// adena the seller holds (ItemContainer.addItem grows a held stackable
// without creating an instance), so the sale takes no object id from the allocator.
func TestSellOntoHeldAdenaSpendsNoObjectID(t *testing.T) {
	t.Parallel()
	w := bootSeller(t, [2]int32{potionID, 10})
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	w.selectFolk(t, f)

	before := w.srv.IssuedObjectIDs()
	w.c.Send(encodeRequestSellItem(merchantID+listOffset, sellRow{w.items[potionID], potionID, 4}))
	drainFrames(t, w.c)

	if got := w.adena(t); got != startAdena+4*20 {
		t.Fatalf("adena = %d, want %d", got, startAdena+4*20)
	}
	if got := w.srv.PlayerInventory(t, w.player).ItemsByTemplateID(item.AdenaID); len(got) != 1 || got[0].ObjectID != w.items[item.AdenaID] {
		t.Fatalf("adena stacks = %d, want the one held stack %d", len(got), w.items[item.AdenaID])
	}
	if got := w.srv.IssuedObjectIDs() - before; got != 0 {
		t.Fatalf("object ids spent by the sale = %d, want 0", got)
	}
}
