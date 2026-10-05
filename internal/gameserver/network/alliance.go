package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// An alliance invitation answered with nothing pending, and an alliance
// crest upload refused (from anyone but the leader of the alliance's
// leading clan, deleting a crest that is not set, declaring more than 192
// bytes or an image that is not stored), are not answered, as specified:
// the invitation dialog closed when the client answered, and the crest
// dialog leaves no client action pending.

// requestJoinAlly invites the clan leader live names into the alliance
// live's clan leads.
func (l *GameClientLink) requestJoinAlly(live *livePlayer, req clientpackets.RequestJoinAlly) {
	c := live.Character
	cl, ok := l.clanService().ClanOf(c)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouAreNotAClanMember))
		return
	}
	target, ok := l.livePlayerByID(req.TargetID)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouHaveInvitedTheWrongTarget))
		return
	}
	if check := l.clanService().CheckAllyJoin(c.ID, target.Character, time.Now()); check.Refusal != clan.AllyJoinAllowed {
		sendAllyJoinRefusal(live, target.Name, check)
		return
	}
	switch l.clanService().Invites().Send(clan.InviteJoinAlly, c.ID, c.Name, target.ObjectID(), 0) {
	case clan.InviteTargetBusy:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, target.Name))
		return
	case clan.InviteRequesterBusy:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWaitingForAnotherReply))
		return
	}
	allyName := cl.Info().AllyName
	target.SendFrame(serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS2AllianceLeaderOfS1Requested,
		serverpackets.TextParam(allyName), serverpackets.TextParam(c.Name)))
	target.SendFrame(serverpackets.FrameAskJoinAlly(c.ObjectID(), allyName))
}

// sendAllyJoinRefusal tells to, the inviter, why its invitation of the
// player named target was refused; nothing when it has left.
func sendAllyJoinRefusal(to *livePlayer, target string, check clan.AllyJoinCheck) {
	if to == nil {
		return
	}
	var frame wire.Frame
	switch check.Refusal {
	case clan.AllyJoinNotAllyLeader:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageFeatureOnlyForAllianceLeader)
	case clan.AllyJoinInvitePenalty:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantInviteClanWithin1Day)
	case clan.AllyJoinSelf:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotInviteYourself)
	case clan.AllyJoinTargetNoClan:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetMustBeInClan)
	case clan.AllyJoinTargetNotLeader:
		frame = serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsNotAClanLeader, target)
	case clan.AllyJoinTargetAllied:
		info := check.Target.Info()
		frame = serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1ClanAlreadyMemberOfS2Alliance,
			serverpackets.TextParam(info.Name), serverpackets.TextParam(info.AllyName))
	case clan.AllyJoinTargetLeftPenalty:
		frame = serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1CantEnterAllianceWithin1Day, check.Target.Name())
	case clan.AllyJoinTargetDismissedPenalty:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantEnterAllianceWithin1Day)
	case clan.AllyJoinAtWar:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageMayNotAllyClanBattle)
	case clan.AllyJoinFull:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouHaveExceededTheLimit)
	default:
		return
	}
	to.SendFrame(frame)
}

// requestAnswerJoinAlly answers the alliance invitation live was sent. A
// refusal answers both sides whatever invitation is pending, as specified.
// An acceptance of anything but an alliance invitation, or one the
// invitation rules now refuse, leaves live's side pending; a refusal by
// those rules is told to the inviter.
func (l *GameClientLink) requestAnswerJoinAlly(live *livePlayer, req clientpackets.RequestAnswerJoinAlly) {
	c := live.Character
	invite, ok := l.clanService().Invites().Partner(c.ID)
	if !ok {
		return
	}
	requester, _ := l.livePlayerByID(invite.RequesterID)
	if req.Answer == 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouDidNotRespondToAllyInvitation))
		if requester != nil {
			requester.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoResponseToAllyInvitation))
		}
		l.clanService().Invites().Answered(c.ID)
		return
	}
	sent, ok := l.clanService().Invites().Requested(invite.RequesterID)
	if !ok || sent.Kind != clan.InviteJoinAlly {
		return
	}
	check := l.clanService().JoinAlly(invite.RequesterID, c, time.Now())
	if check.Refusal != clan.AllyJoinAllowed {
		sendAllyJoinRefusal(requester, c.Name, check)
		return
	}
	l.refreshClanAppearance(live, check.Target)
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouAcceptedAlliance))
	l.clanService().Invites().Answered(c.ID)
}

// allyLeave takes live's clan out of its alliance.
func (l *GameClientLink) allyLeave(live *livePlayer) {
	cl, result := l.clanService().LeaveAlly(live.Character, time.Now())
	var msg int
	switch result {
	case clan.AllyLeaveNoClan:
		msg = serverpackets.SystemMessageYouAreNotAClanMember
	case clan.AllyLeaveNotLeader:
		msg = serverpackets.SystemMessageOnlyClanLeaderWithdrawAlly
	case clan.AllyLeaveNoAlly:
		msg = serverpackets.SystemMessageNoCurrentAlliances
	case clan.AllyLeaveIsAllyLeader:
		msg = serverpackets.SystemMessageAllianceLeaderCantWithdraw
	case clan.AllyLeft:
		l.refreshClanAppearance(live, cl)
		msg = serverpackets.SystemMessageYouHaveWithdrawnFromAlliance
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msg))
}

// allyDismiss has live, its alliance's leader, dismiss the clan it names.
func (l *GameClientLink) allyDismiss(live *livePlayer, req clientpackets.AllyDismiss) {
	cl, result := l.clanService().DismissAllyClan(live.Character, req.Name, time.Now())
	var msg int
	switch result {
	case clan.AllyDismissNoClan:
		msg = serverpackets.SystemMessageYouAreNotAClanMember
	case clan.AllyDismissNoAlly:
		msg = serverpackets.SystemMessageNoCurrentAlliances
	case clan.AllyDismissNotAllyLeader:
		msg = serverpackets.SystemMessageFeatureOnlyForAllianceLeader
	case clan.AllyDismissNoSuchClan:
		msg = serverpackets.SystemMessageClanDoesntExist
	case clan.AllyDismissOwnClan:
		msg = serverpackets.SystemMessageAllianceLeaderCantWithdraw
	case clan.AllyDismissDifferentAlly:
		msg = serverpackets.SystemMessageDifferentAlliance
	case clan.AllyDismissed:
		l.refreshClanAppearance(live, cl)
		msg = serverpackets.SystemMessageYouHaveExpelledAClan
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msg))
}

// requestDismissAlly dissolves the alliance live's clan leads. A player
// leading no clan is refused before anything else is checked.
func (l *GameClientLink) requestDismissAlly(live *livePlayer) {
	c := live.Character
	if cl, ok := l.clanService().ClanOf(c); !ok || !cl.IsLeader(c.ID) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFeatureOnlyForAllianceLeader))
		return
	}
	l.dissolveAlly(live)
}

// dissolveAlly dissolves the alliance live's clan leads: every member of
// the alliance learns it, every clan that loses its alliance crest is
// refreshed, then live takes a full death's experience loss. A clanless
// player's request does nothing.
func (l *GameClientLink) dissolveAlly(live *livePlayer) {
	out, result := l.clanService().DissolveAlly(live.Character, l.crests, time.Now())
	switch result {
	case clan.AllyDissolveNoClan:
		l.log.Debug().Int32("object_id", live.ObjectID()).Msg("alliance dissolution from a clanless player")
		return
	case clan.AllyDissolveNoAlly:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoCurrentAlliances))
		return
	case clan.AllyDissolveNotAllyLeader:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFeatureOnlyForAllianceLeader))
		return
	}
	l.broadcastToClans(out.Clans, func() wire.Frame {
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageAllianceDissolved)
	})
	for _, cl := range out.Cleared {
		l.refreshClanAppearance(live, cl)
	}
	live.Character.ApplyDeathPenalty()
}

// broadcastToClans sends the built packet to every member in the world of
// each clan of clans, each member its own copy.
func (l *GameClientLink) broadcastToClans(clans []*clan.Clan, build func() wire.Frame) {
	var recipients []*livePlayer
	for _, cl := range clans {
		recipients = append(recipients, l.onlineClanMembers(cl, 0)...)
	}
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, member := range recipients {
			send(member)
		}
	})
}

// createAlly founds the alliance name with live's clan as its leading clan.
func (l *GameClientLink) createAlly(live *livePlayer, name string) {
	var msg int
	switch l.clanService().CreateAlly(live.Character, name, time.Now()) {
	case clan.AllyCreated:
		live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
		return
	case clan.AllyCreateNotLeader:
		msg = serverpackets.SystemMessageOnlyClanLeaderCreateAlliance
	case clan.AllyCreateAlreadyAllied:
		msg = serverpackets.SystemMessageAlreadyJoinedAlliance
	case clan.AllyCreateLevelTooLow:
		msg = serverpackets.SystemMessageCreateAllyClanLevel5Needed
	case clan.AllyCreatePenalty:
		msg = serverpackets.SystemMessageCantCreateAlliance10DaysDissolve
	case clan.AllyCreateDissolving:
		msg = serverpackets.SystemMessageCannotCreateAllyWhileDissolving
	case clan.AllyCreateNameInvalid:
		msg = serverpackets.SystemMessageIncorrectAllianceName
	case clan.AllyCreateNameLength:
		msg = serverpackets.SystemMessageIncorrectAllianceNameLength
	case clan.AllyCreateNameTaken:
		msg = serverpackets.SystemMessageAllianceAlreadyExists
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msg))
}

// requestSetAllyCrest stores the alliance crest live uploads for every
// clan of the alliance its clan leads; an empty image deletes it.
func (l *GameClientLink) requestSetAllyCrest(live *livePlayer, req clientpackets.CrestUpload) {
	if req.Length > clientpackets.AllyCrestMaxLength {
		return
	}
	allies, result := l.clanService().SetAllyCrest(live.Character, req.Data, l.crests)
	var msg int
	switch result {
	case clan.CrestDeleted:
		msg = serverpackets.SystemMessageClanCrestHasBeenDeleted
	case clan.CrestRegistered:
		msg = serverpackets.SystemMessageClanEmblemSuccessfullyRegistered
	default:
		return
	}
	for _, cl := range allies {
		l.refreshClanAppearance(live, cl)
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msg))
}

// requestAllyInfo shows live its alliance: the alliance window, then the
// same in system messages, a block per clan.
func (l *GameClientLink) requestAllyInfo(live *livePlayer) {
	var allyID int32
	if cl, ok := l.clanService().ClanOf(live.Character); ok {
		allyID = cl.AllyID()
	}
	info, ok := l.clanService().Table().AllianceInfo(allyID, l.clanMemberConnected)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoCurrentAlliances))
		return
	}
	packet := serverpackets.Alliance{
		Name: info.Name, Total: int32(info.Total), Online: int32(info.Online),
		LeaderClan: info.LeaderClan, LeaderName: info.LeaderName,
	}
	for _, c := range info.Clans {
		packet.Clans = append(packet.Clans, serverpackets.AllianceClan{
			Name: c.Name, Level: int32(c.Level), LeaderName: c.LeaderName, Total: int32(c.Total), Online: int32(c.Online),
		})
	}
	live.SendFrame(serverpackets.FrameAllianceInfo(packet))
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAllianceInfoHead))
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageAllianceNameS1, info.Name))
	live.SendFrame(serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageAllianceLeaderS2OfS1,
		serverpackets.TextParam(info.LeaderClan), serverpackets.TextParam(info.LeaderName)))
	live.SendFrame(frameConnectionCount(info.Online, info.Total))
	live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageAllianceClanTotalS1, int32(len(info.Clans))))
	for i, c := range info.Clans {
		if i == 0 {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanInfoHead))
		} else {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanInfoSeparator))
		}
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageClanInfoNameS1, c.Name))
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageClanInfoLeaderS1, c.LeaderName))
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageClanInfoLevelS1, int32(c.Level)))
		live.SendFrame(frameConnectionCount(c.Online, c.Total))
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanInfoFoot))
}

// frameConnectionCount builds the "connected / total" line of the alliance
// information.
func frameConnectionCount(online, total int) wire.Frame {
	return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageConnectionS1TotalS2,
		serverpackets.NumberParam(int32(online)), serverpackets.NumberParam(int32(total)))
}

// chatAlliance is heard by every member online of every clan of the
// speaker's alliance, the speaker included, whatever their block lists.
// From a player in no alliance it is dropped without an answer.
func (l *GameClientLink) chatAlliance(_ *Client, live *livePlayer, line chat.Line) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok || cl.AllyID() == 0 {
		return
	}
	l.broadcastToClans(l.clanService().Table().Allies(cl.AllyID()), frameCreatureSay(live, line))
}
