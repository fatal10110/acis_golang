package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// questTokenID is the harness catalog's quest item.
const questTokenID = 9001

// TestAddTradeItemDropsUnofferableAdds pins the silent refusals of
// TradeList.addItem (TradeList.java:128-147): an item the trader may move but
// not offer as asked (a quest item, more than held, more than one of an item
// that does not stack, a zero or negative count) is dropped with no frame to
// either side, and the offer stays empty.
func TestAddTradeItemDropsUnofferableAdds(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	potions := h.srv.GiveItem(t, h.firstID, potionID, 3)
	sword := h.srv.GiveItem(t, h.firstID, swordID, 1)
	token := h.srv.GiveItem(t, h.firstID, questTokenID, 2)
	h.enterAll(t)
	h.startTrade(t)

	for _, tc := range []struct {
		name     string
		objectID int32
		count    int32
	}{
		{"quest item", token, 1},
		{"more than held", potions, 4},
		{"two of a non-stackable", sword, 2},
		{"zero count", potions, 0},
		{"negative count", potions, -1},
	} {
		h.first.Send(encodeAddTradeItem(0, tc.objectID, tc.count))
		assertSilent(t, h.first, tc.name)
		assertSilent(t, h.second, "partner of "+tc.name)
	}

	// The window is still open and the offer still empty: one potion goes in
	// with all three still available before it.
	h.first.Send(encodeAddTradeItem(0, potions, 1))
	assertOwnOfferFrames(t, h.first.Read(), h.first.Read(), h.first.Read(), potions, potionID, 1, 2)
}

// TestAddTradeItemNonPositiveCountRunsEveryGate pins that a zero or negative
// count is not dropped up front: it meets the confirm gates first
// (ONCE_THE_TRADE_IS_CONFIRMED_THE_ITEM_CANNOT_BE_MOVED_AGAIN for the
// confirmer, CANNOT_ADJUST_ITEMS_AFTER_TRADE_CONFIRMED for its partner) and,
// before any confirm, the item manipulation check, whose NOTHING_HAPPENED
// answers an object the trader does not hold.
func TestAddTradeItemNonPositiveCountRunsEveryGate(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	potions := h.srv.GiveItem(t, h.secondID, potionID, 3)
	h.enterAll(t)
	h.startTrade(t)

	h.first.Send(encodeAddTradeItem(0, potions, 0)) // the partner's item
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageNothingHappened)
	assertSilent(t, h.second, "partner of an unheld zero-count offer")

	h.first.Send(encodeTradeDone(1))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodeTradePressOwnOk, "first TradePressOwnOk")
	drainUntilQuiet(t, h.second)

	for _, count := range []int32{0, -5} {
		h.first.Send(encodeAddTradeItem(0, adena, count))
		assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageOnceTradeConfirmedCannotMove)
		assertSilent(t, h.second, "partner of the confirmer's non-positive offer")

		h.second.Send(encodeAddTradeItem(0, potions, count))
		assertStaticSystemMessage(t, h.second.Read(), serverpackets.SystemMessageCannotAdjustItemsAfterConfirm)
		assertSilent(t, h.first, "confirmer on the partner's non-positive offer")
	}
}
