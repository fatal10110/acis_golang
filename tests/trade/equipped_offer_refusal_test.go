package trade

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// equipSword gives the first trader a sword, enters both and has the first
// wear it, asserting the sword reached the paperdoll.
func equipSword(t *testing.T, h *traders) int32 {
	t.Helper()
	sword := h.srv.GiveItem(t, h.firstID, swordID, 1)
	h.enterAll(t)
	h.first.Send(encodeUseItem(sword))
	h.srv.Settle(t)
	drainFrames(t, h.first)
	drainFrames(t, h.second)
	assertSwordLocation(t, h, sword)
	return sword
}

// assertSwordLocation checks the sword is still worn by the first trader.
func assertSwordLocation(t *testing.T, h *traders, sword int32) {
	t.Helper()
	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(context.Background(), h.firstID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	for _, row := range rows {
		if row.ObjectID == sword {
			if row.Location != item.LocationPaperdoll {
				t.Fatalf("sword location = %v, want paperdoll", row.Location)
			}
			return
		}
	}
	t.Fatalf("sword %d left the first trader", sword)
}

// TestEquippedItemRefusedOnSellListAndTradeOffer pins the #4826 parity
// exception for worn items: a sell list naming one is refused whole and
// opens no store, and a trade offer of one is answered with the
// nothing-happened notice and offers nothing, so the item stays on the
// paperdoll in both cases.
func TestEquippedItemRefusedOnSellListAndTradeOffer(t *testing.T) {
	t.Parallel()
	t.Run("sell list", func(t *testing.T) {
		t.Parallel()
		h := bootTraders(t)
		sword := equipSword(t, h)

		h.first.Send(encodeRequestActionUse(actionStoreSell))
		assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListSell, "PrivateStoreManageListSell")
		h.first.Send(encodeSetPrivateStoreListSell(false, sellRow{sword, 1, 300}))
		assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageExceededTheMaximum)
		assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListSell, "manage window again")
		assertSilent(t, h.first, "listing a worn sword")
		assertSilent(t, h.second, "bystander of a refused sell list")
		if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateSellManage {
			t.Fatalf("operate type = %d, want sell manage", got)
		}
		assertSwordLocation(t, h, sword)
	})
	t.Run("trade offer", func(t *testing.T) {
		t.Parallel()
		h := bootTraders(t)
		sword := equipSword(t, h)
		h.startTrade(t)

		h.first.Send(encodeAddTradeItem(0, sword, 1))
		assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageNothingHappened)
		assertSilent(t, h.first, "offering a worn sword")
		assertSilent(t, h.second, "partner of a worn-sword offer")
		assertSwordLocation(t, h, sword)
	})
}
