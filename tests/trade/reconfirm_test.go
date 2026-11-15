package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestRepeatedConfirmChangesNothing pins that a trader who already confirmed
// and confirms again is answered with nothing, in range or out
// (TradeList.confirm returns before validating once confirmed): the repeat
// neither re-sends the confirm packets nor runs the interaction-range check
// that cancels a first confirm. The trade stays open, and once the partner
// is back in range its own confirm settles it.
func TestRepeatedConfirmChangesNothing(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	h.enterAll(t)
	h.startTrade(t)

	h.first.Send(encodeAddTradeItem(0, adena, 40))
	assertOwnOfferFrames(t, h.first.Read(), h.first.Read(), h.first.Read(), adena, item.AdenaID, 40, 60)
	drainUntilQuiet(t, h.second)

	h.first.Send(encodeTradeDone(1))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodeTradePressOwnOk, "first TradePressOwnOk")
	assertSystemMessageText(t, h.second.Read(), serverpackets.SystemMessageS1ConfirmedTrade, "TraderOne")
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeTradePressOtherOk, "second TradePressOtherOk")

	h.first.Send(encodeTradeDone(1))
	assertSilent(t, h.first, "first repeat confirm in range")
	assertSilent(t, h.second, "second on the first's repeat confirm in range")

	walkSecondTo(t, h, spawnX, spawnX+2*tradeInteractionDistance)
	h.first.Send(encodeTradeDone(1))
	assertSilent(t, h.first, "first repeat confirm out of range")
	assertSilent(t, h.second, "second on the first's repeat confirm out of range")

	walkSecondTo(t, h, spawnX+2*tradeInteractionDistance, spawnX)
	h.second.Send(encodeTradeDone(1))
	for _, who := range h.both() {
		frames := drainFrames(t, who.client)
		i := indexOfOpcode(frames, serverpackets.OpcodeSendTradeDone)
		if i < 0 || i+1 >= len(frames) {
			t.Fatalf("%s frames after the settling confirm = %x, want SendTradeDone then a system message", who.name, opcodes(frames))
		}
		if got := wire.NewReader(frames[i][1:]).ReadInt32(); got != 1 {
			t.Fatalf("%s SendTradeDone success = %d, want 1", who.name, got)
		}
		assertStaticSystemMessage(t, frames[i+1], serverpackets.SystemMessageTradeSuccessful)
	}
	h.srv.Settle(t)
	if got := h.srv.PlayerInventory(t, h.secondID).ItemCount(item.AdenaID, -1, true); got != 40 {
		t.Fatalf("second adena after the settled trade = %d, want 40", got)
	}
}

// walkSecondTo walks the second trader along X from fromX to x and drains
// both clients' movement noise.
func walkSecondTo(t *testing.T, h *traders, fromX, x int32) {
	t.Helper()
	h.second.Send(encodeMoveBackwardToLocation(x, spawnY, spawnZ, fromX, spawnY, spawnZ))
	expectGroundClickAck(t, h.second)
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeMoveToLocation, "second MoveToLocation")
	waitForArrival(t, h, h.secondID, x)
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)
}
