package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: RequestDestroyItem.java:34-38 refuses a player that
// isProcessingTransaction() — an active requester, an open trade list, or
// its own unexpired outgoing request (Player.java:3047-3050) — with
// CANNOT_TRADE_DISCARD_DROP_ITEM_WHILE_IN_SHOPMODE before anything else.

func encodeRequestDestroyItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDestroyItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

// assertDestroyRefused sends a destroy of count units of objectID and
// requires the shop-mode refusal with the stack untouched.
func assertDestroyRefused(t *testing.T, h *traders, c *testsupport.ScriptedClient, ownerID, objectID, count int32, held int) {
	t.Helper()
	c.Send(encodeRequestDestroyItem(objectID, count))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotTradeDiscardDropInShopMode)
	drainUntilQuiet(t, c)
	inst := h.srv.PlayerInventory(t, ownerID).ItemByObjectID(objectID)
	if inst == nil {
		t.Fatalf("item %d gone after a refused destroy", objectID)
	}
	if got := inst.Snapshot().Count; got != held {
		t.Fatalf("item %d count after a refused destroy = %d, want %d", objectID, got, held)
	}
}

// TestDestroyRefusedWhilePendingTradeRequest: the requester and the target
// of a pending trade request are both refused; once the target declines,
// the requester's destroy goes through and names what disappeared.
func TestDestroyRefusedWhilePendingTradeRequest(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	firstAdena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	secondAdena := h.srv.GiveItem(t, h.secondID, item.AdenaID, 100)
	h.enterAll(t)

	h.first.Send(encodeTradeRequest(h.secondID))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")

	assertDestroyRefused(t, h, h.first, h.firstID, firstAdena, 10, 100)
	assertDestroyRefused(t, h, h.second, h.secondID, secondAdena, 10, 100)

	h.second.Send(encodeAnswerTradeRequest(0))
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageS1DeniedTradeRequest, "TraderTwo")
	drainUntilQuiet(t, h.second)

	h.first.Send(encodeRequestDestroyItem(firstAdena, 10))
	var disappeared bool
	for _, f := range drainFrames(t, h.first) {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if id := wire.NewReader(f[1:]).ReadInt32(); id != serverpackets.SystemMessageS2S1Disappeared {
			t.Fatalf("destroy after the declined request sent message %d, want S2_S1_DISAPPEARED", id)
		}
		disappeared = true
	}
	if !disappeared {
		t.Fatal("destroy after the declined request sent no S2_S1_DISAPPEARED")
	}
}

// TestDestroyRefusedDuringOpenTrade: both participants of an open trade
// window are refused.
func TestDestroyRefusedDuringOpenTrade(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	firstAdena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	ingots := h.srv.GiveItem(t, h.secondID, heavyIngot, 4)
	h.enterAll(t)
	h.startTrade(t)

	assertDestroyRefused(t, h, h.first, h.firstID, firstAdena, 1, 100)
	assertDestroyRefused(t, h, h.second, h.secondID, ingots, 4, 4)
}
