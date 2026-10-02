package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeFriendInvite(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestFriendInvite)
	w.WriteString(name)
	return w.Bytes()
}

// inviteFriend has from send a friend invitation to the named player and
// consumes the FriendAddRequest it shows to.
func inviteFriend(t *testing.T, from, to *testsupport.ScriptedClient, name string) {
	t.Helper()
	from.Send(encodeFriendInvite(name))
	assertFrameOpcode(t, to.Read(), serverpackets.OpcodeFriendAddRequest, "FriendAddRequest")
}

// TestTradeRequestMeetsPendingFriendInvitation pins TradeRequest.java's busy
// checks against the one pending-request slot every player has: a target
// holding a friend invitation is refused with S1_IS_BUSY_TRY_LATER, and a
// requester waiting on one it sent with ALREADY_TRADING, exactly as for a
// pending trade request, and so is a trade request to the inviter. No
// target hears anything.
func TestTradeRequestMeetsPendingFriendInvitation(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.enterAll(t)
	carol, carolID := h.third(t, "player3", "Carol")

	inviteFriend(t, carol, h.second, "TraderTwo")

	h.first.Send(encodeTradeRequest(h.secondID))
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageS1IsBusyTryLater, "TraderTwo")
	assertSilent(t, h.second, "invited target after a refused trade request")

	carol.Send(encodeTradeRequest(h.firstID))
	assertStaticSystemMessage(t, carol.Read(), serverpackets.SystemMessageAlreadyTrading)
	assertSilent(t, h.first, "trade target of a refused inviter")

	// The inviter, waiting on its invitation, is a busy target too.
	h.first.Send(encodeTradeRequest(carolID))
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageS1IsBusyTryLater, "Carol")
	assertSilent(t, carol, "inviter after a refused trade request")
}

// TestLogoutHoldingFriendInviteCancelsTrade pins Player.deleteMe with an
// active requester that is a friend invitation: the trader who logs out
// while it waits for an answer cancels its open trade, so the partner gets
// SendTradeDone(0) and S1_CANCELED_TRADE naming the one who left.
func TestLogoutHoldingFriendInviteCancelsTrade(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.enterAll(t)
	carol, _ := h.third(t, "player3", "Carol")
	h.startTrade(t)

	inviteFriend(t, carol, h.second, "TraderTwo")

	h.second.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	h.awaitOffline(t, h.secondID)
	frames := drainFrames(t, h.first)
	done := indexOfOpcode(frames, serverpackets.OpcodeSendTradeDone)
	if done < 0 || done+1 >= len(frames) || tradeDones(frames) != 1 {
		t.Fatalf("partner frames = %x, want one SendTradeDone then the canceled-trade message", opcodes(frames))
	}
	if got := wire.NewReader(frames[done][1:]).ReadInt32(); got != 0 {
		t.Fatalf("SendTradeDone success = %d, want 0", got)
	}
	assertSystemMessageText(t, frames[done+1], serverpackets.SystemMessageS1CanceledTrade, "TraderTwo")
}

// TestLogoutTradeCancelFollowsPartyLeave pins the order Player.deleteMe
// tells a partner who is the leaver's party member, trade partner and
// friend: the party leave first (the two-member party's
// PartySmallWindowDeleteAll), then the trade cancel of an active requester
// (SendTradeDone(0), S1_CANCELED_TRADE), then L2FriendStatus offline.
func TestLogoutTradeCancelFollowsPartyLeave(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.srv.Relations.AddFriend(h.firstID, h.secondID)
	h.enterAll(t)
	carol, _ := h.third(t, "player3", "Carol")

	join := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	join.WriteString("TraderTwo")
	join.WriteInt32(0)
	h.first.Send(join.Bytes())
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty")
	answer := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	answer.WriteInt32(1)
	h.second.Send(answer.Bytes())
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)

	h.startTrade(t)
	inviteFriend(t, carol, h.second, "TraderTwo")

	h.second.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	h.awaitOffline(t, h.secondID)
	frames := drainFrames(t, h.first)
	party := indexOfOpcode(frames, serverpackets.OpcodePartySmallWindowDeleteAll)
	done := indexOfOpcode(frames, serverpackets.OpcodeSendTradeDone)
	friend := indexOfOpcode(frames, serverpackets.OpcodeL2FriendStatus)
	if party < 0 || done < 0 || friend < 0 || party > done || done > friend || tradeDones(frames) != 1 {
		t.Fatalf("partner frames = %x, want the party leave, then one trade cancel, then the friend status", opcodes(frames))
	}
	assertSystemMessageText(t, frames[done+1], serverpackets.SystemMessageS1CanceledTrade, "TraderTwo")
}
