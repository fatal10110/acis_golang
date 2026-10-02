package network

import (
	"fmt"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
)

// requestAskJoinChannel invites the named player's party into the command
// channel live's party leads, or into one it would form. Either side
// without a party, or both in one, is ignored without an answer.
func (l *GameClientLink) requestAskJoinChannel(live *livePlayer, req clientpackets.TargetName) {
	target, ok := l.livePlayerByName(req.Name)
	if !ok {
		return
	}
	status, targetLeader := l.parties.ChannelInvite(live.ObjectID(), target.ObjectID())
	switch status {
	case party.ChannelInviteIgnored:
		return
	case party.ChannelInviteNotLeader:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotInviteToCommandChannel))
		return
	case party.ChannelInviteTargetInChannel:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AlreadyMemberOfCommandChannel, target.Name))
		return
	}
	if !l.commandChannelAuthority(live, false) {
		return
	}
	if l.tradeBook().Invite(tradebook.KindCommandChannel, live.ObjectID(), targetLeader.ObjectID(), false).Status != tradebook.RequestStarted {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, targetLeader.Name))
		return
	}
	targetLeader.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageCommandChannelConfirmFromS1, live.Name))
	targetLeader.SendFrame(serverpackets.FrameExAskJoinMPCC(live.Name))
}

// requestAcceptJoinChannel answers live's pending command channel
// invitation. With none pending, its inviter gone, or either party gone,
// nothing answers.
func (l *GameClientLink) requestAcceptJoinChannel(live *livePlayer, req clientpackets.Response) {
	requesterID, ok := l.tradeBook().TakeInvite(tradebook.KindCommandChannel, live.ObjectID())
	if !ok {
		return
	}
	requester, ok := l.livePlayerByID(requesterID)
	if !ok {
		return
	}
	forms, ok := l.parties.ChannelAnswerForms(requesterID, live.ObjectID())
	if !ok {
		return
	}
	if req.Response != 1 {
		requester.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeclinedChannelInvitation, live.Name))
		return
	}
	if !forms {
		l.applyPartyNotices(l.parties.JoinFormedChannel(requester, live))
		return
	}
	// Forming a channel is authorized, and its Strategy Guide paid, on the
	// requester; the answer runs on live's queue as the reference runs it
	// on the answering client's thread.
	if !l.commandChannelAuthority(requester, true) {
		return
	}
	l.applyPartyNotices(l.parties.JoinChannel(requester, live))
}

// requestOustFromChannel dismisses the named player's party from the
// command channel live leads.
func (l *GameClientLink) requestOustFromChannel(live *livePlayer, req clientpackets.TargetName) {
	target, ok := l.livePlayerByName(req.Name)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetCantFound))
		return
	}
	if target.ObjectID() == live.ObjectID() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	status, notices := l.parties.Oust(live, target.ObjectID())
	switch status {
	case party.OustInvalidTarget:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	case party.OustNotAuthorized:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	l.applyPartyNotices(notices)
}

// requestChannelPartyMembers lists the members of the party the given
// player is in. A player not online or in no party gets no list.
func (l *GameClientLink) requestChannelPartyMembers(live *livePlayer, req clientpackets.RequestExMPCCShowPartyMembersInfo) {
	leader, ok := l.livePlayerByID(req.PartyLeaderID)
	if !ok {
		return
	}
	view, ok := l.parties.View(leader.ObjectID())
	if !ok {
		return
	}
	rows := make([]serverpackets.PartyChannelMember, len(view.Members))
	for i, m := range view.Members {
		rows[i] = serverpackets.PartyChannelMember{Name: m.Name, ObjectID: m.ObjectID(), ClassID: int32(m.ClassID())}
	}
	live.SendFrame(serverpackets.FrameExMPCCShowPartyMemberInfo(rows))
}

// Command channel authority: the clan level its former must lead, the
// clan skill that is authority enough, and the item that otherwise is.
const (
	channelClanLevel    = 5
	clanImperiumSkillID = 391
	strategyGuideItemID = 8871
)

// commandChannelAuthority reports whether live may form a command channel,
// telling it why not: only the leader of a clan of level 5 or more may,
// and then by holding Clan Imperium or a Strategy Guide. With pay set, the
// guide is destroyed as the channel forms; otherwise holding one is
// enough.
func (l *GameClientLink) commandChannelAuthority(live *livePlayer, pay bool) bool {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok || !cl.IsLeader(live.ObjectID()) || cl.Level() < channelClanLevel {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCommandChannelOnlyByLevel5ClanLeader))
		return false
	}
	if live.HasSkill(clanImperiumSkillID) {
		return true
	}
	var held bool
	if pay {
		held = destroyHeldItems(live, strategyGuideItemID, 1)
	} else if inv := live.Inventory(); inv != nil {
		held = inv.ItemByTemplateID(strategyGuideItemID) != nil
	}
	if !held {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotLongerSetupCommandChannel))
	}
	return held
}

const (
	// partyPositionPeriod is how often a party's members are told each
	// other's positions; the first report comes half a period after the
	// party forms.
	partyPositionPeriod = 12 * time.Second
)

// partyPositions owns the queue each party's position reports run on. mu
// guards queues.
type partyPositions struct {
	mu     sync.Mutex
	queues map[party.ID]*sim.Queue
}

func (l *GameClientLink) startPartyPositions(id party.ID) {
	if l.queues == nil {
		return
	}
	q := l.queues.NewQueue(fmt.Sprintf("party-%d", id))
	l.partyPositions.mu.Lock()
	if l.partyPositions.queues == nil {
		l.partyPositions.queues = make(map[party.ID]*sim.Queue)
	}
	l.partyPositions.queues[id] = q
	l.partyPositions.mu.Unlock()
	q.After(partyPositionPeriod/2, func() {
		l.sendPartyPositions(id)
		q.Every(partyPositionPeriod, func() { l.sendPartyPositions(id) })
	})
	// Formed and Dispersed are applied outside the registry lock, possibly
	// on different queues, so the party may already have dispersed and its
	// Dispersed found no queue to stop. The registry drops a party before
	// it reports Dispersed, so a party still listed here will have its
	// queue, now registered, stopped by that report.
	if _, ok := l.parties.Members(id); !ok {
		l.stopPartyPositions(id)
	}
}

func (l *GameClientLink) stopPartyPositions(id party.ID) {
	l.partyPositions.mu.Lock()
	q := l.partyPositions.queues[id]
	delete(l.partyPositions.queues, id)
	l.partyPositions.mu.Unlock()
	if q != nil {
		q.Close()
	}
}

// sendPartyPositions tells party id's members where each of them is.
func (l *GameClientLink) sendPartyPositions(id party.ID) {
	members, ok := l.parties.Members(id)
	if !ok {
		return
	}
	at := make([]serverpackets.PartyMemberAt, len(members))
	for i, m := range members {
		loc := m.CurrentLocation()
		at[i] = serverpackets.PartyMemberAt{ObjectID: m.ObjectID(), X: int32(loc.X), Y: int32(loc.Y), Z: int32(loc.Z)}
	}
	l.broadcastToMembers(members, func() wire.Frame { return serverpackets.FramePartyMemberPosition(at) })
}
