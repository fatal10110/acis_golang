package trade

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// enchantSeeded sets the enchant level of one persisted item row, before the
// owner enters the world.
func enchantSeeded(t *testing.T, h *traders, ownerID, objectID int32, level int) {
	t.Helper()
	rows, err := h.srv.Items.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items of %d: %v", ownerID, err)
	}
	for _, inst := range rows {
		if inst.ObjectID != objectID {
			continue
		}
		inst.EnchantLevel = level
		if err := h.srv.Items.Update(context.Background(), inst); err != nil {
			t.Fatalf("seed enchant level: %v", err)
		}
		return
	}
	t.Fatalf("no item row %d for owner %d", objectID, ownerID)
}

// TestPrivateStoreEnchantedDealMessages pins the deal messages of a
// non-stackable item carrying an enchant level: both sides get the
// S1_PURCHASED_S2_S3 / PURCHASED_S2_S3_FROM_S1 pair, naming the other
// trader, the enchant level as a number, then the item name, in a sell store
// and in a buy store alike.
func TestPrivateStoreEnchantedDealMessages(t *testing.T) {
	t.Parallel()
	t.Run("sell store", func(t *testing.T) {
		h := bootTraders(t)
		sword := h.srv.GiveItem(t, h.firstID, swordID, 1)
		enchantSeeded(t, h, h.firstID, sword, 3)
		h.srv.GiveItem(t, h.secondID, item.AdenaID, 1000)
		h.enterAll(t)

		openSellStore(t, h, h.first, h.second, actionStoreSell, false, sellRow{sword, 1, 300})
		clickStore(t, h.second, h.firstID)
		drainUntilQuiet(t, h.first)

		h.second.Send(encodeRequestPrivateStoreBuy(h.firstID, sellRow{sword, 1, 300}))
		assertMessage(t, h.second.Read(), serverpackets.SystemMessagePurchasedS2S3FromS1, textP("TraderOne"), numberP(3), itemNameP(swordID))
		assertMessage(t, h.first.Read(), serverpackets.SystemMessageS1PurchasedS2S3, textP("TraderTwo"), numberP(3), itemNameP(swordID))
	})
	t.Run("buy store", func(t *testing.T) {
		h := bootTraders(t)
		wanted := h.srv.GiveItem(t, h.firstID, swordID, 1)
		enchantSeeded(t, h, h.firstID, wanted, 3)
		h.srv.GiveItem(t, h.firstID, item.AdenaID, 1000)
		sword := h.srv.GiveItem(t, h.secondID, swordID, 1)
		enchantSeeded(t, h, h.secondID, sword, 3)
		h.enterAll(t)

		openBuyStore(t, h, buyRow{swordID, 3, 1, 300})
		clickStore(t, h.second, h.firstID)
		drainUntilQuiet(t, h.first)

		h.second.Send(encodeRequestPrivateStoreSell(h.firstID, saleRow{sword, swordID, 3, 1, 300}))
		assertMessage(t, h.second.Read(), serverpackets.SystemMessageS1PurchasedS2S3, textP("TraderOne"), numberP(3), itemNameP(swordID))
		assertMessage(t, h.first.Read(), serverpackets.SystemMessagePurchasedS2S3FromS1, textP("TraderTwo"), numberP(3), itemNameP(swordID))
	})
}
