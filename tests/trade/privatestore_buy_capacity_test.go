package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// assertHeld asserts how many units of itemID objID holds, after the server
// has settled.
func assertHeld(t *testing.T, h *traders, objID, itemID int32, want int, what string) {
	t.Helper()
	if got := h.srv.PlayerInventory(t, objID).ItemCount(itemID, -1, true); got != want {
		t.Fatalf("%s: player %d holds %d of item %d, want %d", what, objID, got, itemID, want)
	}
}

// TestBuyStoreSaleRefusedWhenOwnerSlotsFull pins #4961 (owner decision:
// diverge from the reference, whose TradeList.privateStoreSell skips the
// owner's capacity check): a buy-store owner at the two-slot limit, holding
// adena and a sword, cannot take a second sword. The seller reads SLOTS_FULL,
// the owner hears nothing, no item or adena moves, and the store stays open.
func TestBuyStoreSaleRefusedWhenOwnerSlotsFull(t *testing.T) {
	t.Parallel()
	h := bootTraders(t, gameservertest.WithInventorySlots(2, 2))
	h.srv.GiveItem(t, h.firstID, swordID, 1)
	h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	sword := h.srv.GiveItem(t, h.secondID, swordID, 1)
	h.enterAll(t)
	openBuyStore(t, h, buyRow{swordID, 0, 1, 10})

	h.second.Send(encodeRequestPrivateStoreSell(h.firstID, saleRow{sword, swordID, 0, 1, 10}))
	assertStaticSystemMessage(t, h.second.Read(), serverpackets.SystemMessageSlotsFull)
	assertSilent(t, h.second, "seller after the refusal")
	assertSilent(t, h.first, "owner after the refusal")
	h.srv.Settle(t)

	assertHeld(t, h, h.firstID, swordID, 1, "owner after the refused sale")
	assertHeld(t, h, h.firstID, item.AdenaID, 100, "owner after the refused sale")
	assertHeld(t, h, h.secondID, swordID, 1, "seller after the refused sale")
	assertHeld(t, h, h.secondID, item.AdenaID, 0, "seller after the refused sale")
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateBuy {
		t.Fatalf("owner operate type after the refused sale = %d, want buy", got)
	}
}

// TestBuyStoreSaleWithinOwnerSlots is the slot control: with one slot to
// spare the same sale goes through, fills the list and closes the store.
func TestBuyStoreSaleWithinOwnerSlots(t *testing.T) {
	t.Parallel()
	h := bootTraders(t, gameservertest.WithInventorySlots(3, 3))
	h.srv.GiveItem(t, h.firstID, swordID, 1)
	h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	sword := h.srv.GiveItem(t, h.secondID, swordID, 1)
	h.enterAll(t)
	openBuyStore(t, h, buyRow{swordID, 0, 1, 10})

	h.second.Send(encodeRequestPrivateStoreSell(h.firstID, saleRow{sword, swordID, 0, 1, 10}))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessageS1PurchasedS2, textP("TraderOne"), itemNameP(swordID))
	assertMessage(t, h.first.Read(), serverpackets.SystemMessagePurchasedS2FromS1, textP("TraderTwo"), itemNameP(swordID))
	h.srv.Settle(t)

	assertHeld(t, h, h.firstID, swordID, 2, "owner after the sale")
	assertHeld(t, h, h.firstID, item.AdenaID, 90, "owner after the sale")
	assertHeld(t, h, h.secondID, swordID, 0, "seller after the sale")
	assertHeld(t, h, h.secondID, item.AdenaID, 10, "seller after the sale")
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
		t.Fatalf("owner operate type after the list filled = %d, want none", got)
	}
}

// TestBuyStoreSaleRefusedWhenOwnerOverWeight pins #4961's weight half: the
// owner carries 50 of a 63 limit (a 40-weight wolf food and one 10-weight
// ingot) and wants two more ingots. A sale of two rows of one ingot each
// fits row by row but not as a whole, so it is refused: the seller reads
// WEIGHT_LIMIT_EXCEEDED, the owner hears nothing and nothing moves. A sale
// of one ingot then fits and leaves the store open for the last one.
func TestBuyStoreSaleRefusedWhenOwnerOverWeight(t *testing.T) {
	t.Parallel()
	h := bootTraders(t, gameservertest.WithWeightLimitMultiplier(0.002))
	h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	h.srv.GiveItem(t, h.firstID, wolfFood, 1)
	h.srv.GiveItem(t, h.firstID, heavyIngot, 1)
	ingots := h.srv.GiveItem(t, h.secondID, heavyIngot, 3)
	h.enterAll(t)
	if got := traderWeightLimit(t, h, h.firstID); got != 63 {
		t.Fatalf("owner weight limit = %d, want 63", got)
	}
	openBuyStore(t, h, buyRow{heavyIngot, 0, 2, 5})

	one := saleRow{ingots, heavyIngot, 0, 1, 5}
	h.second.Send(encodeRequestPrivateStoreSell(h.firstID, one, one))
	assertStaticSystemMessage(t, h.second.Read(), serverpackets.SystemMessageWeightLimitExceeded)
	assertSilent(t, h.second, "seller after the refusal")
	assertSilent(t, h.first, "owner after the refusal")
	h.srv.Settle(t)
	assertHeld(t, h, h.firstID, heavyIngot, 1, "owner after the refused sale")
	assertHeld(t, h, h.firstID, item.AdenaID, 100, "owner after the refused sale")
	assertHeld(t, h, h.secondID, heavyIngot, 3, "seller after the refused sale")
	assertHeld(t, h, h.secondID, item.AdenaID, 0, "seller after the refused sale")

	h.second.Send(encodeRequestPrivateStoreSell(h.firstID, one))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessageS1PurchasedS3S2S, textP("TraderOne"), itemNameP(heavyIngot), numberP(1))
	assertMessage(t, h.first.Read(), serverpackets.SystemMessagePurchasedS3S2SFromS1, textP("TraderTwo"), itemNameP(heavyIngot), numberP(1))
	h.srv.Settle(t)
	assertHeld(t, h, h.firstID, heavyIngot, 2, "owner after the sale")
	assertHeld(t, h, h.firstID, item.AdenaID, 95, "owner after the sale")
	assertHeld(t, h, h.secondID, heavyIngot, 2, "seller after the sale")
	assertHeld(t, h, h.secondID, item.AdenaID, 5, "seller after the sale")
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateBuy {
		t.Fatalf("owner operate type with one ingot still wanted = %d, want buy", got)
	}
}
