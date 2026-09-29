package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestTradeRefusedWhenReceiverInventoryFull pins the settle-time slot check
// (TradeList.doExchange, validateTradeListCapacity) against the configured
// player slot limit: a receiver already holding MaximumSlotsForNoDwarf
// stacks cannot take a new one, both players read SlotsFull, the exchange
// ends failed for both, and neither side's items move.
func TestTradeRefusedWhenReceiverInventoryFull(t *testing.T) {
	h := bootTraders(t, gameservertest.WithInventorySlots(2, 2))
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	ingots := h.srv.GiveItem(t, h.secondID, heavyIngot, 4)
	h.srv.GiveItem(t, h.secondID, 30, 1)
	h.enterAll(t)
	h.startTrade(t)
	offerBoth(t, h, adena, 10, ingots, 2)

	confirmBoth(t, h)
	for _, who := range h.both() {
		assertStaticSystemMessage(t, who.client.Read(), serverpackets.SystemMessageSlotsFull)
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, who.name+" SendTradeDone")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != 0 {
			t.Fatalf("%s SendTradeDone success = %d, want 0", who.name, got)
		}
		assertStaticSystemMessage(t, who.client.Read(), serverpackets.SystemMessageExchangeHasEnded)
	}
	h.srv.Settle(t)

	first, second := h.srv.PlayerInventory(t, h.firstID), h.srv.PlayerInventory(t, h.secondID)
	if got := first.ItemCount(item.AdenaID, -1, true); got != 100 {
		t.Fatalf("first adena after the refused trade = %d, want 100", got)
	}
	if got := first.ItemCount(heavyIngot, -1, true); got != 0 {
		t.Fatalf("first ingots after the refused trade = %d, want 0", got)
	}
	if got := second.ItemCount(heavyIngot, -1, true); got != 4 {
		t.Fatalf("second ingots after the refused trade = %d, want 4", got)
	}
	if got := second.ItemCount(item.AdenaID, -1, true); got != 0 {
		t.Fatalf("second adena after the refused trade = %d, want 0", got)
	}
}
