package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// TestWorkshopPaymentTakesObjectIDOnlyWhenItSplits places two workshop
// orders and counts the object ids each took. The first order's 100 adena
// start the crafter's stack and its potion the customer's, one id each; the
// second order's adena and potion land on those stacks and take none.
// Reference: the payment moves through ItemContainer.transferItem, which
// creates an instance only for a partial move into a target holding no
// stack (ItemContainer.java:255-305).
func TestWorkshopPaymentTakesObjectIDOnlyWhenItSplits(t *testing.T) {
	t.Parallel()
	w := openWorkshop(t, 500, 4)

	order := func(wantIDs int) {
		t.Helper()
		w.srv.AddPlayerMP(t, w.crafterID, float64(w.srv.PlayerMaxMP(t, w.crafterID)))
		before := w.srv.IssuedObjectIDs()
		w.customer.Send(encodeRecipeShopOrder(clientpackets.OpcodeRequestRecipeShopMakeItem, w.crafterID, commonRecipeID))
		w.srv.Settle(t)
		collectUntilQuiet(t, w.customer)
		collectUntilQuiet(t, w.crafter)
		if got := w.srv.IssuedObjectIDs() - before; got != wantIDs {
			t.Fatalf("the order took %d object ids, want %d", got, wantIDs)
		}
	}
	order(2)
	order(0)

	w.srv.InventoryUpdates.Tick()
	w.srv.FlushItems(t)
	if inst := mustFindItemByTemplate(t, w.srv, w.customerID, commonProductID); inst.Count != 2 {
		t.Fatalf("customer product count = %d, want 2", inst.Count)
	}
	if inst := mustFindItemByTemplate(t, w.srv, w.customerID, item.AdenaID); inst.Count != 300 {
		t.Fatalf("customer adena = %d, want 300", inst.Count)
	}
	if inst := mustFindItemByTemplate(t, w.srv, w.crafterID, item.AdenaID); inst.Count != 2*workshopCost {
		t.Fatalf("crafter adena = %d, want %d", inst.Count, 2*workshopCost)
	}
}
