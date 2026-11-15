package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestTradeStartHidesSelectedEnchantScroll pins the requester's TradeStart
// list against the available-item filter (ItemInstance.isAvailable): the
// enchant scroll it has selected is left out, while its potions are listed.
func TestTradeStartHidesSelectedEnchantScroll(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	scroll := h.srv.GiveItem(t, h.firstID, enchantScrollD, 1)
	potions := h.srv.GiveItem(t, h.firstID, potionID, 3)
	h.enterAll(t)
	selectEnchantScroll(t, h.first, scroll)

	h.first.Send(encodeTradeRequest(h.secondID))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")
	h.second.Send(encodeAnswerTradeRequest(1))
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageBeginTradeWithS1, "TraderTwo")
	frame := h.first.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeTradeStart, "first TradeStart")

	r := wire.NewReader(frame[1:])
	r.ReadInt32() // partner id
	rows := r.ReadUint16()
	r.ReadUint16() // category
	listed := r.ReadInt32()
	if err := r.Err(); err != nil || rows != 1 || listed != potions {
		t.Fatalf("first TradeStart rows = %d, first object %d (err %v), want only the potions %d", rows, listed, err, potions)
	}
}
