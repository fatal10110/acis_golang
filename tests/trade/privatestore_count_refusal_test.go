package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// TestStoreListIncorrectItemCount pins INCORRECT_ITEM_COUNT on both store
// lists: an empty list and a list longer than the owner's store limit (4 rows
// for a non-dwarf, 5 for a dwarf) each close the store and answer the message,
// for a sell store and a buy store alike.
func TestStoreListIncorrectItemCount(t *testing.T) {
	t.Parallel()
	// Past either race's limit, so the case does not depend on the race.
	const overLimit = 6
	t.Run("sell store", func(t *testing.T) {
		h := bootTraders(t)
		rows := make([]sellRow, overLimit)
		for i := range rows {
			rows[i] = sellRow{h.srv.GiveItem(t, h.firstID, swordID, 1), 1, 10}
		}
		h.enterAll(t)

		for name, list := range map[string][]sellRow{"empty list": nil, "list past the limit": rows} {
			h.first.Send(encodeRequestActionUse(actionStoreSell))
			assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListSell, "PrivateStoreManageListSell")
			h.first.Send(encodeSetPrivateStoreListSell(false, list...))
			assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageIncorrectItemCount)
			assertSilent(t, h.first, name)
			if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
				t.Fatalf("%s: operate type = %d, want none", name, got)
			}
		}
	})
	t.Run("buy store", func(t *testing.T) {
		h := bootTraders(t)
		ids := []int32{potionID, swordID, 14, 1463, 955, 6575}
		rows := make([]buyRow, overLimit)
		for i := range rows {
			h.srv.GiveItem(t, h.firstID, ids[i], 1)
			rows[i] = buyRow{ids[i], 0, 1, 1}
		}
		h.srv.GiveItem(t, h.firstID, item.AdenaID, 1000)
		h.enterAll(t)

		for name, list := range map[string][]buyRow{"empty list": nil, "list past the limit": rows} {
			h.first.Send(encodeRequestActionUse(actionStoreBuy))
			assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListBuy, "PrivateStoreManageListBuy")
			h.first.Send(encodeSetPrivateStoreListBuy(list...))
			assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageIncorrectItemCount)
			assertSilent(t, h.first, name)
			if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
				t.Fatalf("%s: operate type = %d, want none", name, got)
			}
		}
	})
}
