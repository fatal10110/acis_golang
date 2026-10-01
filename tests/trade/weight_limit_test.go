package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// wolfFood is the harness catalog's 40-weight stackable item.
const wolfFood = 2515

// traderWeightLimit returns the live player's current weight limit.
func traderWeightLimit(t *testing.T, h *traders, objID int32) int {
	t.Helper()
	obj, ok := h.srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d not online", objID)
	}
	carrier, ok := obj.(interface{ WeightLimit() int })
	if !ok {
		t.Fatalf("player %d = %T reports no weight limit", objID, obj)
	}
	return carrier.WeightLimit()
}

// bootWeightTraders boots the two traders under the WeightLimit config
// multiplier m, the first carrying 100 adena and one 40-weight wolf food
// (two stacks), the second three 10-weight ingots, and opens a trade where
// the first offers 10 adena and the second all three ingots. It returns the
// adena and ingot object ids.
//
// The harness class's CON bonus is 0.46, so the weight limit is
// int(69000 * 0.46 * m), the reference's Player.getWeightLimit with no
// weightLimit stat bonus.
func bootWeightTraders(t *testing.T, m float64, wantLimit int, opts ...gameservertest.Option) (*traders, int32, int32) {
	t.Helper()
	h := bootTraders(t, append([]gameservertest.Option{gameservertest.WithWeightLimitMultiplier(m)}, opts...)...)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	h.srv.GiveItem(t, h.firstID, wolfFood, 1)
	ingots := h.srv.GiveItem(t, h.secondID, heavyIngot, 3)
	h.enterAll(t)
	for _, id := range []int32{h.firstID, h.secondID} {
		if got := traderWeightLimit(t, h, id); got != wantLimit {
			t.Fatalf("player %d weight limit = %d, want %d", id, got, wantLimit)
		}
	}
	h.startTrade(t)
	offerBoth(t, h, adena, 10, ingots, 3)
	return h, adena, ingots
}

// TestTradeRefusedWhenReceiverOverWeightLimit pins TradeList.doExchange's
// weight check against the receiver's live weight limit
// (PcInventory.validateWeight: carried weight plus the incoming offer's
// weight within Player.getWeightLimit): the first trader carries 40 of a
// 63 limit and would receive 30 more, so both players read
// WEIGHT_LIMIT_EXCEEDED, the exchange ends failed for both, and no item
// moves. The first trader is also at its two-slot limit and the ingots would
// need a new slot: the weight check runs first, so no SLOTS_FULL follows.
func TestTradeRefusedWhenReceiverOverWeightLimit(t *testing.T) {
	t.Parallel()
	h, _, _ := bootWeightTraders(t, 0.002, 63, gameservertest.WithInventorySlots(2, 2))

	confirmBoth(t, h)
	for _, who := range h.both() {
		assertStaticSystemMessage(t, who.client.Read(), serverpackets.SystemMessageWeightLimitExceeded)
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
	if got := second.ItemCount(heavyIngot, -1, true); got != 3 {
		t.Fatalf("second ingots after the refused trade = %d, want 3", got)
	}
	if got := second.ItemCount(item.AdenaID, -1, true); got != 0 {
		t.Fatalf("second adena after the refused trade = %d, want 0", got)
	}
}

// TestTradeSettlesWithinRaisedWeightLimit is the control: the same trade
// under a WeightLimit multiplier that lifts the limit to 95 leaves the first
// trader at 70 carried, so it settles and the ingots move.
func TestTradeSettlesWithinRaisedWeightLimit(t *testing.T) {
	t.Parallel()
	h, _, _ := bootWeightTraders(t, 0.003, 95)

	confirmBoth(t, h)
	for _, who := range h.both() {
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, who.name+" SendTradeDone")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != 1 {
			t.Fatalf("%s SendTradeDone success = %d, want 1", who.name, got)
		}
		assertStaticSystemMessage(t, who.client.Read(), serverpackets.SystemMessageTradeSuccessful)
	}
	h.srv.Settle(t)

	if got := h.srv.PlayerInventory(t, h.firstID).ItemCount(heavyIngot, -1, true); got != 3 {
		t.Fatalf("first ingots after the trade = %d, want 3", got)
	}
}
