package social

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestTradeRequestRefusedWhileTargetBlocksEverything pins the TradeRequest
// block-everything gate (TradeRequest.java): a request to a player in
// /allblock answers S1_BLOCKED_EVERYTHING naming the target, and the target
// hears nothing. The refusal records no request, so once the target lifts
// the block the same requester's next request goes through.
func TestTradeRequestRefusedWhileTargetBlocksEverything(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)

	p.bobby.Send(encodeBlock(clientpackets.BlockAll, ""))
	assertStaticSystemMessage(t, p.bobby.Read(), serverpackets.SystemMessageBlockingAll)
	assertEtcBlocked(t, p.bobby.Read(), true)

	p.alice.Send(encodeTradeRequest(p.bobbyID))
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1BlockedEverything, "Bobby")
	assertSilent(t, p.bobby, "target blocking everything")
	assertSilent(t, p.alice, "after the block-everything refusal")

	p.bobby.Send(encodeBlock(clientpackets.BlockAllRelease, ""))
	assertStaticSystemMessage(t, p.bobby.Read(), serverpackets.SystemMessageNotBlockingAll)
	assertEtcBlocked(t, p.bobby.Read(), false)

	p.alice.Send(encodeTradeRequest(p.bobbyID))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest after /allunblock")
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageRequestS1ForTrade, "Bobby")
}

// TestTradeRequestRefusedByTargetBlockList pins the TradeRequest ignore-list
// gate: a requester on the target's block list is answered
// S1_HAS_ADDED_YOU_TO_IGNORE_LIST (619, not the 620 friend-message variant)
// naming the target, and the target hears nothing. The block is one way: the
// blocker can still ask the player it blocks.
func TestTradeRequestRefusedByTargetBlockList(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)

	p.bobby.Send(encodeBlock(clientpackets.BlockAdd, "Alice"))
	assertSystemMessageText(t, p.bobby.Read(), serverpackets.SystemMessageS1AddedToYourIgnoreList, "Alice")
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1HasAddedYouToIgnoreList, "Bobby")

	p.alice.Send(encodeTradeRequest(p.bobbyID))
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1HasAddedYouToIgnoreList, "Bobby")
	assertSilent(t, p.bobby, "target of a blocked requester")
	assertSilent(t, p.alice, "after the ignore-list refusal")

	p.bobby.Send(encodeTradeRequest(p.aliceID))
	assertOpcode(t, p.alice.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest from the blocker")
	assertSystemMessageText(t, p.bobby.Read(), serverpackets.SystemMessageRequestS1ForTrade, "Alice")
}

// TestTradeRequestBusyTargetAnsweredBeforeBlock pins the reference order of
// the TradeRequest gates: a target already holding a request is answered
// S1_IS_BUSY_TRY_LATER even while it blocks everything, since the busy check
// comes first.
func TestTradeRequestBusyTargetAnsweredBeforeBlock(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)
	carol, _ := p.third(t, "player3", "Carol")

	carol.Send(encodeTradeRequest(p.bobbyID))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeSendTradeRequest, "Carol's SendTradeRequest")
	assertSystemMessageText(t, carol.Read(), serverpackets.SystemMessageRequestS1ForTrade, "Bobby")

	p.bobby.Send(encodeBlock(clientpackets.BlockAll, ""))
	assertStaticSystemMessage(t, p.bobby.Read(), serverpackets.SystemMessageBlockingAll)
	assertEtcBlocked(t, p.bobby.Read(), true)

	p.alice.Send(encodeTradeRequest(p.bobbyID))
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1IsBusyTryLater, "Bobby")
	assertSilent(t, p.bobby, "busy target")
}
