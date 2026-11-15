package trade

import (
	"context"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestTradeRefusedWhenReceiverStackWouldOverflow pins the settle-time count
// check (PcInventory.validateTradeListCount, run by TradeList.doExchange after
// the weight and slot checks): the second trader already holds
// MaxInt32-10 of a stackable item and would receive 50 more, so the exchange
// ends failed for both with no system message of its own, and neither
// inventory nor any DB row changes. Without the check the receiver's stack
// clamps at the limit and the sender's excess is destroyed.
func TestTradeRefusedWhenReceiverStackWouldOverflow(t *testing.T) {
	t.Parallel()
	const held = math.MaxInt32 - 10
	for _, tc := range []struct {
		name       string
		templateID int32
	}{
		{"adena", item.AdenaID},
		{"potion", potionID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootTraders(t)
			offered := h.srv.GiveItem(t, h.firstID, tc.templateID, 100)
			stack := h.srv.GiveItem(t, h.secondID, tc.templateID, held)
			h.enterAll(t)
			h.startTrade(t)

			h.first.Send(encodeAddTradeItem(0, offered, 50))
			assertOwnOfferFrames(t, h.first.Read(), h.first.Read(), h.first.Read(), offered, tc.templateID, 50, 50)
			drainUntilQuiet(t, h.second)

			confirmBoth(t, h)
			for _, who := range h.both() {
				frame := who.client.Read()
				assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, who.name+" SendTradeDone")
				if got := wire.NewReader(frame[1:]).ReadInt32(); got != 0 {
					t.Fatalf("%s SendTradeDone success = %d, want 0", who.name, got)
				}
				assertStaticSystemMessage(t, who.client.Read(), serverpackets.SystemMessageExchangeHasEnded)
			}
			h.srv.Settle(t)

			if got := h.srv.PlayerInventory(t, h.firstID).ItemCount(tc.templateID, -1, true); got != 100 {
				t.Fatalf("sender count after the refused trade = %d, want 100", got)
			}
			if got := h.srv.PlayerInventory(t, h.secondID).ItemCount(tc.templateID, -1, true); got != held {
				t.Fatalf("receiver count after the refused trade = %d, want %d", got, held)
			}

			h.srv.FlushItems(t)
			ctx := context.Background()
			for _, owner := range []struct {
				id       int32
				objectID int32
				count    int
			}{{h.firstID, offered, 100}, {h.secondID, stack, held}} {
				rows, err := h.srv.Items.ListByOwner(ctx, owner.id)
				if err != nil {
					t.Fatalf("list items of %d: %v", owner.id, err)
				}
				if len(rows) != 1 || rows[0].ObjectID != owner.objectID || int(rows[0].Count) != owner.count {
					t.Fatalf("persisted rows of %d after the refused trade = %+v, want object %d count %d", owner.id, rows, owner.objectID, owner.count)
				}
			}
		})
	}
}
