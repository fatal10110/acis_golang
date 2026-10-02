package social

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestFriendInviteToTargetHoldingPartyInvitation pins RequestFriendInvite's
// target.isProcessingRequest() against a party invitation: one pending
// request of any kind refuses the invitation with WAITING_FOR_ANOTHER_REPLY
// and a failed FriendAddRequestResult.
func TestFriendInviteToTargetHoldingPartyInvitation(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)
	carol, _ := p.third(t, "player3", "Carol")

	join := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	join.WriteString("Bobby")
	join.WriteInt32(0)
	carol.Send(join.Bytes())
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeAskJoinParty, "Carol's AskJoinParty")
	drainUntilQuiet(t, carol)

	p.alice.Send(encodeFriendInvite("Bobby"))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageWaitingForAnotherReply)
	assertAddResult(t, p.alice.Read(), false)
	assertSilent(t, p.bobby, "party-invited target after the refusal")
}

// TestFriendAnswerLeavesTradeRequest pins the answer side of the shared
// slot: a friend answer from a player holding a trade request is no answer
// to it. Nobody becomes friends, nobody hears anything, and the trade
// request still holds its target.
func TestFriendAnswerLeavesTradeRequest(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)
	carol, _ := p.third(t, "player3", "Carol")

	p.alice.Send(encodeTradeRequest(p.bobbyID))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	drainUntilQuiet(t, p.alice)

	p.bobby.Send(encodeAnswerFriendInvite(1))
	assertSilent(t, p.bobby, "target answering a trade request as a friend invitation")
	assertSilent(t, p.alice, "trade requester after the stray friend answer")
	if p.srv.Relations.AreFriends(p.aliceID, p.bobbyID) {
		t.Fatal("a friend answer to a trade request made friends")
	}

	carol.Send(encodeFriendInvite("Bobby"))
	assertStaticSystemMessage(t, carol.Read(), serverpackets.SystemMessageWaitingForAnotherReply)
	assertAddResult(t, carol.Read(), false)
}
