package network

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
)

// partyRegistry is the registry of the parties live players form.
type partyRegistry = party.Registry[*livePlayer]

// dispatchParty decodes and runs one top-level party packet. It reports
// false once the connection has been closed for a malformed packet.
func (l *GameClientLink) dispatchParty(client *Client, live *livePlayer, opcode byte, payload []byte) bool {
	switch opcode {
	case clientpackets.OpcodeRequestJoinParty:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestJoinParty, l.requestJoinParty)
	case clientpackets.OpcodeRequestAnswerJoinParty:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestAnswerJoinParty, l.requestAnswerJoinParty)
	case clientpackets.OpcodeRequestWithdrawParty:
		// The request carries no body. A player in no party leaves nothing
		// and hears nothing: the menu action waits on no answer.
		if live != nil {
			onLive(live, func() { l.withdrawParty(live) })
		}
	case clientpackets.OpcodeRequestOustPartyMember:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestOustPartyMember, l.requestOustPartyMember)
	}
	return true
}

// dispatchPartyExtended decodes and runs one extended party or command
// channel packet; see dispatchParty.
func (l *GameClientLink) dispatchPartyExtended(client *Client, live *livePlayer, second uint16, payload []byte) bool {
	switch second {
	case clientpackets.OpcodeRequestChangePartyLeader:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestChangePartyLeader, l.requestChangePartyLeader)
	case clientpackets.OpcodeRequestExAskJoinMPCC:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestExAskJoinMPCC, l.requestAskJoinChannel)
	case clientpackets.OpcodeRequestExAcceptJoinMPCC:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestExAcceptJoinMPCC, l.requestAcceptJoinChannel)
	case clientpackets.OpcodeRequestExOustFromMPCC:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestExOustFromMPCC, l.requestOustFromChannel)
	case clientpackets.OpcodeRequestExMPCCShowPartyMembersInfo:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestExMPCCShowPartyMembersInfo, l.requestChannelPartyMembers)
	}
	return true
}

// dispatchLive decodes payload and runs handle on live's queue. A packet
// with no player in the world is dropped once decoded. It reports false
// once the connection has been closed for a malformed packet.
func dispatchLive[T any](l *GameClientLink, client *Client, live *livePlayer, payload []byte, decode func([]byte) (T, error), handle func(*livePlayer, T)) bool {
	req, err := decodeClientPacket(l, client, payload, decode)
	if err != nil {
		return !errors.Is(err, errMalformedPacketDisconnect)
	}
	if live != nil {
		onLive(live, func() { handle(live, req) })
	}
	return true
}

func (l *GameClientLink) livePlayerByName(name string) (*livePlayer, bool) {
	if l.world == nil {
		return nil, false
	}
	obj, ok := l.world.PlayerByName(name)
	if !ok {
		return nil, false
	}
	live, ok := obj.(*livePlayer)
	return live, ok
}

// Texts refusing a party invitation the client has no system message for.
const (
	partyInviteDetachedText = "The player you tried to invite is in offline mode."
	partyInviteJailedText   = "The player you tried to invite is currently jailed."
)

// requestJoinParty invites the named player into live's party, or into a
// new one live will lead.
//
// A target blocking everything or blocking live refuses first, before
// even an invitation of oneself; an invisible target is refused as the
// wrong target; a target whose connection is gone, or either side in jail,
// is refused once the party check passed. Either side competing in an
// Olympiad match is then turned away without a word, as the reference does:
// neither the inviter nor the target hears of it.
func (l *GameClientLink) requestJoinParty(live *livePlayer, req clientpackets.RequestJoinParty) {
	target, ok := l.livePlayerByName(req.Target)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFirstSelectUserToInviteToParty))
		return
	}
	if blocked := l.blockRefusal(live, target); blocked != 0 {
		live.SendFrame(serverpackets.FrameSystemMessageString(blocked, target.Name))
		return
	}
	if target.ObjectID() == live.ObjectID() || target.CursedWeaponEquipped() || live.CursedWeaponEquipped() || target.Invisible() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouHaveInvitedTheWrongTarget))
		return
	}
	if l.parties.InParty(target.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsAlreadyInParty, target.Name))
		return
	}
	if target.clientDetached() {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, partyInviteDetachedText))
		return
	}
	if target.Jailed() || live.Jailed() {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, partyInviteJailedText))
		return
	}
	if target.OlympiadMode() || live.OlympiadMode() {
		return
	}
	book := l.tradeBook()
	if book.ProcessingRequest(live.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWaitingForAnotherReply))
		return
	}
	if book.ProcessingRequest(target.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, target.Name))
		return
	}

	status, loot := l.parties.BeginInvite(live.ObjectID(), req.LootRule)
	switch status {
	case party.InviteNotLeader:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyLeaderCanInvite))
		return
	case party.InviteFull:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePartyFull))
		return
	case party.InviteWaiting:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWaitingForAnotherReply))
		return
	case party.InviteBadLoot:
		// No such rule: the request fails without an answer, and invites
		// nobody.
		l.log.Warn().Int32("object_id", live.ObjectID()).Int32("loot_rule", req.LootRule).Msg("party invite with an unknown loot rule")
		return
	}

	// Either side may have become busy on another queue since the checks
	// above; the invitation then asks no one, so the party stops waiting.
	switch book.Invite(tradebook.KindParty, live.ObjectID(), target.ObjectID(), true).Status {
	case tradebook.RequestRequesterBusy:
		l.parties.CancelInvite(live.ObjectID())
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWaitingForAnotherReply))
		return
	case tradebook.RequestTargetBusy:
		l.parties.CancelInvite(live.ObjectID())
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, target.Name))
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageYouInvitedS1ToParty, target.Name))
	target.SendFrame(serverpackets.FrameAskJoinParty(live.Name, int32(loot)))
}

// requestAnswerJoinParty answers live's pending party invitation. With no
// invitation pending, or its inviter gone, nothing answers: the dialog
// closed on the client when it answered.
func (l *GameClientLink) requestAnswerJoinParty(live *livePlayer, req clientpackets.Response) {
	requesterID, ok := l.tradeBook().TakeInvite(tradebook.KindParty, live.ObjectID())
	if !ok {
		return
	}
	requester, ok := l.livePlayerByID(requesterID)
	if !ok {
		return
	}
	requester.SendFrame(serverpackets.FrameJoinParty(req.Response))
	l.applyPartyNotices(l.parties.Answer(requester, live, req.Response == 1))
	if req.Response == 1 {
		l.applyRoomNotices(l.rooms.PartyJoined(requester, live))
	}
}

// requestOustPartyMember expels the named member from the party live
// leads. Anyone else, or a name no member has, expels nobody and hears
// nothing.
func (l *GameClientLink) requestOustPartyMember(live *livePlayer, req clientpackets.TargetName) {
	l.applyPartyNotices(l.parties.Expel(live.ObjectID(), req.Name))
}

// requestChangePartyLeader hands the party live leads to the named member.
func (l *GameClientLink) requestChangePartyLeader(live *livePlayer, req clientpackets.TargetName) {
	l.applyPartyNotices(l.parties.ChangeLeader(live, req.Name))
}

// leaveParty takes a player leaving the world out of its party.
func (l *GameClientLink) leaveParty(live *livePlayer) {
	if l.parties == nil {
		return
	}
	l.applyPartyNotices(l.parties.Leave(live, party.Disconnected))
	l.parties.Forget(live.ObjectID())
}

// sendPartyVitals answers a vitals change of live's: its party-window row
// is refreshed on the other members' windows, and its row in its duel
// opponents' window, when a gauge left its segment.
func (l *GameClientLink) sendPartyVitals(live *livePlayer) {
	var view partyView
	inParty := false
	if l.parties != nil {
		view, inParty = l.parties.View(live.ObjectID())
	}
	cpOrHP, partyRow := live.VitalsGaugesStale(inParty)
	l.sendDuelVitals(live, cpOrHP)
	if !partyRow {
		return
	}
	row := partyMemberRow(live)
	l.broadcastToMembers(without(view.Members, live), func() wire.Frame {
		return serverpackets.FramePartySmallWindowUpdate(row)
	})
}

// applyPartyNotices sends the packets of a party change, in order.
func (l *GameClientLink) applyPartyNotices(notices []party.Notice) {
	for _, n := range notices {
		switch n := n.(type) {
		case party.WindowAll[*livePlayer]:
			rows := make([]serverpackets.PartyMember, len(n.Others))
			for i, m := range n.Others {
				rows[i] = partyMemberRow(m)
			}
			n.To.SendFrame(serverpackets.FramePartySmallWindowAll(n.Leader, int32(n.Loot), rows))
		case party.WindowAdd[*livePlayer]:
			row := partyMemberRow(n.Member)
			l.broadcastToMembers(n.To, func() wire.Frame {
				return serverpackets.FramePartySmallWindowAdd(n.Leader, int32(n.Loot), row)
			})
		case party.WindowDelete[*livePlayer]:
			l.broadcastToMembers(n.To, func() wire.Frame {
				return serverpackets.FramePartySmallWindowDelete(n.Member.ObjectID(), n.Member.Name)
			})
		case party.WindowDeleteAll[*livePlayer]:
			n.To.SendFrame(serverpackets.FramePartySmallWindowDeleteAll())
		case party.Msg[*livePlayer]:
			id := partyMessages[n.ID]
			l.broadcastToMembers(n.To, func() wire.Frame {
				if n.Name != "" {
					return serverpackets.FrameSystemMessageString(id, n.Name)
				}
				return serverpackets.FrameSystemMessage(id)
			})
		case party.IconRefresh[*livePlayer]:
			// A member who never held an effect has no icons to show.
			if n.Member.EffectList().HasHeld() {
				sendPartySpelled(n.Member, n.To)
			}
		case party.InfoRefresh[*livePlayer]:
			l.broadcastCharacterInfo(n.Member)
		case party.FusionStop[*livePlayer]:
			if n.Member.fusionTargetID.Load() != 0 {
				n.Member.Character.StopCast()
			}
			l.abortFusionTargeting(n.Member)
		case party.ChannelOpen[*livePlayer]:
			l.broadcastToMembers(n.To, serverpackets.FrameExOpenMPCC)
		case party.ChannelClose[*livePlayer]:
			l.broadcastToMembers(n.To, serverpackets.FrameExCloseMPCC)
		case party.ChannelPartyUpdate[*livePlayer]:
			l.broadcastToMembers(n.To, func() wire.Frame {
				return serverpackets.FrameExMPCCPartyInfoUpdate(n.Leader.Name, n.Leader.ObjectID(), int32(n.Count), n.Added)
			})
		case party.LeaderChanged[*livePlayer]:
			if l.rooms != nil {
				l.applyRoomNotices(l.rooms.PartyLeaderChanged(n.Leader))
			}
		case party.Formed:
			l.startPartyPositions(n.ID)
		case party.Dispersed:
			l.stopPartyPositions(n.ID)
		case party.Edited[*livePlayer]:
			l.cancelPartyDuel(n.Leader)
		}
	}
}

// broadcastToMembers sends each of members its own copy of one frame.
func (l *GameClientLink) broadcastToMembers(members []*livePlayer, build func() wire.Frame) {
	sendToMembers(members, build)
}

// sendToMembers sends each of members its own copy of one frame.
func sendToMembers(members []*livePlayer, build func() wire.Frame) {
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, m := range members {
			send(m)
		}
	})
}

// partyMemberRow is live's row in a party window.
func partyMemberRow(live *livePlayer) serverpackets.PartyMember {
	res := live.ResourceValues()
	return serverpackets.PartyMember{
		ObjectID: live.ObjectID(),
		Name:     live.Name,
		CP:       int32(res.CurrentCP),
		MaxCP:    int32(res.MaxCP),
		HP:       int32(res.CurrentHP),
		MaxHP:    int32(res.MaxHP),
		MP:       int32(res.CurrentMP),
		MaxMP:    int32(res.MaxMP),
		Level:    int32(live.Level()),
		ClassID:  int32(live.ClassID()),
		Race:     int32(live.Race),
	}
}

func without(members []*livePlayer, self *livePlayer) []*livePlayer {
	out := make([]*livePlayer, 0, len(members))
	for _, m := range members {
		if m.ObjectID() != self.ObjectID() {
			out = append(out, m)
		}
	}
	return out
}

var partyMessages = map[party.MessageID]int{
	party.MsgYouJoinedParty:            serverpackets.SystemMessageYouJoinedS1Party,
	party.MsgJoinedParty:               serverpackets.SystemMessageS1JoinedParty,
	party.MsgPartyDispersed:            serverpackets.SystemMessagePartyDispersed,
	party.MsgExpelledFromParty:         serverpackets.SystemMessageHaveBeenExpelledFromParty,
	party.MsgWasExpelled:               serverpackets.SystemMessageS1WasExpelledFromParty,
	party.MsgYouLeftParty:              serverpackets.SystemMessageYouLeftParty,
	party.MsgLeftParty:                 serverpackets.SystemMessageS1LeftParty,
	party.MsgBecameLeader:              serverpackets.SystemMessageS1HasBecomeAPartyLeader,
	party.MsgCannotTransferToSelf:      serverpackets.SystemMessageYouCannotTransferRightsToYourself,
	party.MsgOnlyLeaderTransfers:       serverpackets.SystemMessageOnlyPartyLeaderCanTransferRights,
	party.MsgChannelLeaderNow:          serverpackets.SystemMessageCommandChannelLeaderNowS1,
	party.MsgChannelFormed:             serverpackets.SystemMessageCommandChannelFormed,
	party.MsgJoinedChannel:             serverpackets.SystemMessageJoinedCommandChannel,
	party.MsgChannelDisbanded:          serverpackets.SystemMessageCommandChannelDisbanded,
	party.MsgDismissedFromChannel:      serverpackets.SystemMessageDismissedFromCommandChannel,
	party.MsgPartyDismissedFromChannel: serverpackets.SystemMessageS1PartyDismissedFromCommandChannel,
	party.MsgLeftChannel:               serverpackets.SystemMessageLeftCommandChannel,
	party.MsgPartyLeftChannel:          serverpackets.SystemMessageS1PartyLeftCommandChannel,
}
