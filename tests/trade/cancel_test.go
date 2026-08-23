package trade

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestCancelMidWayReturnsItemsUntouched pins the mid-trade cancel: one side
// answering TradeDone with response 0 ends the session for both players with
// SendTradeDone failure plus the canceled-trade message, offered items stay
// in their owners' inventories, and the cleared session ignores further
// add-item packets.
func TestCancelMidWayReturnsItemsUntouched(t *testing.T) {
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	h.enterAll(t)
	h.startTrade(t)

	h.first.Send(encodeAddTradeItem(0, adena, 40))
	assertOwnOfferFrames(t, h.first.Read(), h.first.Read(), h.first.Read(), adena, item.AdenaID, 40, 60)
	drainUntilQuiet(t, h.second)

	h.first.Send(encodeTradeDone(0))
	for _, who := range []struct {
		name   string
		client *testsupport.ScriptedClient
		text   string
	}{
		{"first", h.first, "TraderTwo"},
		{"second", h.second, "TraderOne"},
	} {
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, who.name+" SendTradeDone")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != 0 {
			t.Fatalf("%s SendTradeDone success = %d, want 0", who.name, got)
		}
		assertSystemMessageText(t, who.client.Read(), serverpackets.SystemMessageS1CanceledTrade, who.text)
	}
	drainUntilQuiet(t, h.second)

	// The cleared session swallows a late add-item without any reply.
	h.first.Send(encodeAddTradeItem(0, adena, 40))
	if frame := h.first.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("late add after cancel replied %#x, want silence", frame[0])
	}
	if frame := h.second.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("partner received %#x after cancel, want silence", frame[0])
	}

	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(context.Background(), h.firstID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(rows) != 1 || rows[0].ObjectID != adena || rows[0].Count != 100 || rows[0].OwnerID != h.firstID {
		t.Fatalf("persisted rows after cancel = %+v, want adena object %d count 100 owner %d",
			rows, adena, h.firstID)
	}
}
