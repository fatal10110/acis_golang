package trade

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// heavyIngot is the harness catalog's weighted, stackable, tradable item.
const heavyIngot = 9500

// TestTradeSettleLeavesWeightToInventoryTick pins the settle packet order
// against the reference's TradeList.doExchange: the exchange answers both
// players with SendTradeDone and TradeSuccessful and nothing ahead of them.
// The carried load only changes on the client with the inventory-update
// tick, InventoryUpdate first and StatusUpdate(CUR_LOAD) after it, exactly as
// for any other inventory change.
func TestTradeSettleLeavesWeightToInventoryTick(t *testing.T) {
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	ingots := h.srv.GiveItem(t, h.secondID, heavyIngot, 4)
	h.enterAll(t)
	h.startTrade(t)
	offerBoth(t, h, adena, 10, ingots, 2)

	confirmBoth(t, h)
	for _, who := range h.both() {
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, who.name+" SendTradeDone")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != 1 {
			t.Fatalf("%s SendTradeDone success = %d, want 1", who.name, got)
		}
		assertStaticSystemMessage(t, who.client.Read(), serverpackets.SystemMessageTradeSuccessful)
		if frame := who.client.ReadWithTimeout(300 * time.Millisecond); frame != nil {
			t.Fatalf("%s got %#x after the trade finished, before any inventory tick", who.name, frame[0])
		}
	}

	h.srv.InventoryUpdates.Tick()
	for _, who := range []struct {
		name   string
		client *testsupport.ScriptedClient
		load   int32
	}{
		{"first", h.first, 20},
		{"second", h.second, 20},
	} {
		assertFrameOpcode(t, who.client.Read(), serverpackets.OpcodeInventoryUpdate, who.name+" InventoryUpdate")
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, who.name+" StatusUpdate")
		if got := currentLoad(t, frame); got != who.load {
			t.Fatalf("%s CUR_LOAD = %d, want %d", who.name, got, who.load)
		}
	}
}

// TestTradeCancelsWhenOfferedItemLeavesBeforeSettle pins the settle-time
// re-check (TradeList.confirm, validate with item checks): an offered stack
// that left its owner's inventory between offer and the final confirm
// cancels the trade for both players, the cancel every other path sends, and
// neither side's items move.
func TestTradeCancelsWhenOfferedItemLeavesBeforeSettle(t *testing.T) {
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	ingots := h.srv.GiveItem(t, h.secondID, heavyIngot, 4)
	h.enterAll(t)
	h.startTrade(t)
	offerBoth(t, h, adena, 40, ingots, 2)

	h.first.Send(encodeTradeDone(1))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodeTradePressOwnOk, "first TradePressOwnOk")
	drainUntilQuiet(t, h.second)

	// The offered adena leaves on its owner's own queue, the way any use
	// or consume would, before the partner's confirm settles the trade.
	firstInv := h.srv.PlayerInventory(t, h.firstID)
	h.srv.PlayerQueue(t, h.firstID).Post(func() { firstInv.DestroyByObjectID(adena, 100) })
	h.srv.Settle(t)

	h.second.Send(encodeTradeDone(1))
	for _, who := range []struct {
		name   string
		client *testsupport.ScriptedClient
		text   string
	}{
		{"first", h.first, "TraderTwo"},
		{"second", h.second, "TraderTwo"},
	} {
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, who.name+" SendTradeDone")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != 0 {
			t.Fatalf("%s SendTradeDone success = %d, want 0", who.name, got)
		}
		assertSystemMessageText(t, who.client.Read(), serverpackets.SystemMessageS1CanceledTrade, who.text)
	}
	h.srv.Settle(t)

	if got := h.srv.PlayerInventory(t, h.secondID).ItemCount(heavyIngot, -1, true); got != 4 {
		t.Fatalf("second ingots after the cancelled trade = %d, want 4", got)
	}
	if got := firstInv.ItemCount(heavyIngot, -1, true); got != 0 {
		t.Fatalf("first ingots after the cancelled trade = %d, want 0", got)
	}
}

func (h *traders) both() []struct {
	name   string
	client *testsupport.ScriptedClient
} {
	return []struct {
		name   string
		client *testsupport.ScriptedClient
	}{{"first", h.first}, {"second", h.second}}
}

// offerBoth has the first trader offer firstCount of firstItem and the
// second secondCount of secondItem, consuming the offer packets.
func offerBoth(t *testing.T, h *traders, firstItem, firstCount, secondItem, secondCount int32) {
	t.Helper()
	h.first.Send(encodeAddTradeItem(0, firstItem, firstCount))
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)
	h.second.Send(encodeAddTradeItem(0, secondItem, secondCount))
	drainUntilQuiet(t, h.second)
	drainUntilQuiet(t, h.first)
}

// confirmBoth sends both confirms, consuming the first confirm's packets.
func confirmBoth(t *testing.T, h *traders) {
	t.Helper()
	h.first.Send(encodeTradeDone(1))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodeTradePressOwnOk, "first TradePressOwnOk")
	assertSystemMessageText(t, h.second.Read(), serverpackets.SystemMessageS1ConfirmedTrade, "TraderOne")
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeTradePressOtherOk, "second TradePressOtherOk")
	h.second.Send(encodeTradeDone(1))
}

func currentLoad(t *testing.T, frame []byte) int32 {
	t.Helper()
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // object id
	n := r.ReadInt32()
	load, found := int32(0), false
	for i := int32(0); i < n; i++ {
		typ, value := serverpackets.StatusType(r.ReadInt32()), r.ReadInt32()
		if typ == serverpackets.StatusCurrentLoad {
			load, found = value, true
		}
	}
	if err := r.Err(); err != nil || !found {
		t.Fatalf("StatusUpdate without CUR_LOAD (err %v)", err)
	}
	return load
}
