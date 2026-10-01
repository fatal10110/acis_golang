package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The clan window requests answer nothing when the player has no clan, or
// names a member or clan that does not exist, as specified: none of them
// leaves a client action pending, the clan window asking again on its
// next click.

// enterWorldClan sends the clan part of the login burst, on live's queue
// before it spawns: the clan's skill list, then live is given the clan
// skills its rank reaches (shown by the burst's SkillList), its fellow
// members learn it logged in, and it gets its own roster row and the
// rosters, the main clan's then each sub-unit's. The siege state (#3150)
// joins here once it exists.
func (l *GameClientLink) enterWorldClan(client *Client, live *livePlayer) {
	c := live.Character
	cl, ok := l.clanService().ClanOf(c)
	if !ok {
		return
	}
	client.Session.SendFrame(framePledgeSkillList(cl))
	cl.SetOnline(c.ID, clan.LiveMember(c))
	l.giveClanSkills(live, cl, c.PledgeClass())
	m, _ := cl.Member(c.ID)
	row := liveMemberRow(c, m)
	l.broadcastToClan(cl, c.ID,
		func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageClanMemberS1LoggedIn, c.Name)
		},
		func() wire.Frame { return serverpackets.FramePledgeShowMemberListUpdate(row) })
	client.Session.SendFrame(serverpackets.FramePledgeShowMemberListUpdate(row))
	for _, frame := range l.pledgeListFrames(cl, live) {
		client.Session.SendFrame(frame)
	}
}

// leaveClanOnLogout marks live offline in its clan and shows the rest of
// the clan its row going offline. Its pending invitation, if any, is left
// to its partner.
func (l *GameClientLink) leaveClanOnLogout(live *livePlayer) {
	c := live.Character
	l.clanService().Invites().Left(c.ID)
	cl, ok := l.clanService().ClanOf(c)
	if !ok {
		return
	}
	cl.SetOffline(c.ID, clan.LiveMember(c))
	m, _ := cl.Member(c.ID)
	row := liveMemberRow(c, m)
	row.OnlineObjectID = 0
	l.broadcastToClan(cl, c.ID, func() wire.Frame { return serverpackets.FramePledgeShowMemberListUpdate(row) })
}

// refreshClanMemberLevel shows live's clan its new level, after any level
// change.
func (l *GameClientLink) refreshClanMemberLevel(live *livePlayer) {
	c := live.Character
	cl, ok := l.clanService().ClanOf(c)
	if !ok {
		return
	}
	cl.RefreshLevel(c.ID, c.Level())
	m, _ := cl.Member(c.ID)
	row := liveMemberRow(c, m)
	l.broadcastToClan(cl, 0, func() wire.Frame { return serverpackets.FramePledgeShowMemberListUpdate(row) })
}

// requestJoinPledge sends live's invitation to join its clan. A clanless
// player's request is ignored.
func (l *GameClientLink) requestJoinPledge(live *livePlayer, req clientpackets.RequestJoinPledge) {
	c := live.Character
	cl, ok := l.clanService().ClanOf(c)
	if !ok {
		return
	}
	target, ok := l.livePlayerByID(req.TargetID)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouHaveInvitedTheWrongTarget))
		return
	}
	if refusal := l.clanService().CheckJoin(cl, c.ID, target.Character, int(req.PledgeType), time.Now()); refusal != clan.JoinAllowed {
		sendJoinRefusal(live, cl, target.Name, refusal)
		return
	}
	switch l.clanService().Invites().Send(clan.InviteJoinPledge, c.ID, c.Name, target.ObjectID(), int(req.PledgeType)) {
	case clan.InviteTargetBusy:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, target.Name))
		return
	case clan.InviteRequesterBusy:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWaitingForAnotherReply))
		return
	}
	name := cl.Name()
	target.SendFrame(serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1HasInvitedYouToJoinTheClanS2,
		serverpackets.TextParam(c.Name), serverpackets.TextParam(name)))
	target.SendFrame(serverpackets.FrameAskJoinPledge(c.ObjectID(), name))
}

// sendJoinRefusal tells to, the inviter, why its invitation of the player
// named target into cl was refused; nothing when it has left.
func sendJoinRefusal(to *livePlayer, cl *clan.Clan, target string, refusal clan.JoinRefusal) {
	if to == nil {
		return
	}
	var frame wire.Frame
	switch refusal {
	case clan.JoinNotAuthorized:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat)
	case clan.JoinInviteSelf:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotInviteYourself)
	case clan.JoinTargetInClan:
		frame = serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1WorkingWithAnotherClan, target)
	case clan.JoinClanPenalty:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageMustWaitBeforeAcceptingNewMember)
	case clan.JoinTargetPenalty:
		frame = serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1MustWaitBeforeJoiningAnotherClan, target)
	case clan.JoinClanFull:
		frame = serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1ClanIsFull, cl.Name())
	case clan.JoinSubunitFull:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageSubclanIsFull)
	case clan.JoinAcademyRequirements:
		to.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1NotMeetAcademyRequirements, target))
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageAcademyRequirements)
	default:
		return
	}
	to.SendFrame(frame)
}

// requestAnswerJoinPledge answers the invitation live was sent. With no
// invitation pending it is ignored; an acceptance whose inviter's own
// invitation has lapsed, or whose inviter has no clan any more, is ignored
// too and leaves live's side pending.
func (l *GameClientLink) requestAnswerJoinPledge(live *livePlayer, req clientpackets.RequestAnswerJoinPledge) {
	c := live.Character
	invite, ok := l.clanService().Invites().Partner(c.ID)
	if !ok {
		return
	}
	requester, _ := l.livePlayerByID(invite.RequesterID)
	if req.Answer == 0 {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageYouDidNotRespondToS1ClanInvitation, invite.RequesterName))
		if requester != nil {
			requester.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DidNotRespondToClanInvitation, c.Name))
		}
		l.clanService().Invites().Answered(c.ID)
		return
	}
	sent, ok := l.clanService().Invites().Requested(invite.RequesterID)
	if !ok || sent.Kind != clan.InviteJoinPledge {
		return
	}
	cl, ok := l.clanService().Table().MemberClan(invite.RequesterID)
	if !ok {
		return
	}
	// The clan's skills are given against the rank live held before it
	// joined, as the reference grants them before it recomputes the rank;
	// no SkillList follows.
	rank := c.PledgeClass()
	if refusal := l.clanService().Join(cl, invite.RequesterID, c, sent.PledgeType, time.Now()); refusal != clan.JoinAllowed {
		sendJoinRefusal(requester, cl, c.Name, refusal)
	} else {
		l.giveClanSkills(live, cl, rank)
		l.sendJoinedClan(live, cl)
	}
	l.clanService().Invites().Answered(c.ID)
}

// sendJoinedClan shows live and its new clan that it joined.
func (l *GameClientLink) sendJoinedClan(live *livePlayer, cl *clan.Clan) {
	c := live.Character
	m, _ := cl.Member(c.ID)
	row := liveMemberRow(c, m)
	live.SendFrame(serverpackets.FrameJoinPledge(cl.ID()))
	live.SendFrame(serverpackets.FramePledgeShowMemberListUpdate(row))
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageEnteredTheClan))
	l.broadcastToClan(cl, c.ID,
		func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1HasJoinedClan, c.Name)
		},
		func() wire.Frame { return serverpackets.FramePledgeShowMemberListAdd(row) })
	l.broadcastToClan(cl, 0, func() wire.Frame { return framePledgeShowInfoUpdate(cl) })
	l.sendPledgeLists(live, cl)
	l.broadcastCharacterInfo(live)
	l.refreshWarTags(live, cl)
}

// sendLeftClan shows live, on its own queue, that it is out of cl: the
// clan's skills taken, its skill list, its status, and its clan window
// closed.
func (l *GameClientLink) sendLeftClan(live *livePlayer, cl *clan.Clan) {
	// Leaving closes whatever warehouse live had open, its own included.
	live.storage.active = activeStore{}
	l.takeClanSkills(live, cl)
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	l.broadcastCharacterInfo(live)
	live.SendFrame(serverpackets.FramePledgeShowMemberListDeleteAll())
}

// requestWithdrawPledge takes live out of its clan.
func (l *GameClientLink) requestWithdrawPledge(live *livePlayer) {
	c := live.Character
	cl, _, refusal := l.clanService().Withdraw(c, time.Now())
	switch refusal {
	case clan.LeaveNotMember:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouAreNotAClanMember))
		return
	case clan.LeaveIsLeader:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanLeaderCannotWithdraw))
		return
	case clan.LeaveInCombat:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotLeaveDuringCombat))
		return
	}
	l.sendLeftClan(live, cl)
	l.broadcastToClan(cl, 0,
		func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1HasWithdrawnFromTheClan, c.Name)
		},
		func() wire.Frame { return serverpackets.FramePledgeShowMemberListDelete(c.Name) })
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouHaveWithdrawnFromClan))
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMustWaitBeforeJoiningAnotherClan))
	l.refreshWarTags(live, cl)
}

// requestOustPledgeMember expels the member live names from live's clan.
// An expelled member online learns it on its own queue.
func (l *GameClientLink) requestOustPledgeMember(live *livePlayer, req clientpackets.RequestOustPledgeMember) {
	online := func(id int32) *player.Character {
		if target, ok := l.livePlayerByID(id); ok {
			return target.Character
		}
		return nil
	}
	now := time.Now()
	cl, m, refusal := l.clanService().Oust(live.Character, req.Name, online, now)
	switch refusal {
	case clan.OustNotMember:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouAreNotAClanMember))
		return
	case clan.OustUnknownTarget:
		return
	case clan.OustNotAuthorized:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	case clan.OustSelf:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDismissYourself))
		return
	case clan.OustInCombat:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanMemberCannotBeDismissedCombat))
		return
	}
	var target *livePlayer
	if m.Online {
		target, _ = l.livePlayerByID(m.ObjectID)
	}
	if target != nil {
		postLive(target, func() {
			l.clanService().ApplyLeft(target.Character, m, now)
			l.sendLeftClan(target, cl)
			target.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanMembershipTerminated))
		})
	}
	l.broadcastToClan(cl, 0,
		func() wire.Frame { return serverpackets.FramePledgeShowMemberListDelete(req.Name) },
		func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageClanMemberS1Expelled, m.Name)
		})
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSucceededInExpellingClanMember))
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMustWaitBeforeAcceptingNewMember))
	// The players around the expelling member, not the expelled one, see
	// their war tags refreshed.
	l.refreshWarTags(live, cl)
}

// requestPledgeMemberList sends live its clan's rosters.
func (l *GameClientLink) requestPledgeMemberList(live *livePlayer) {
	if cl, ok := l.clanService().ClanOf(live.Character); ok {
		l.sendPledgeLists(live, cl)
	}
}

// requestPledgeInfo sends live the name card of any clan.
func (l *GameClientLink) requestPledgeInfo(live *livePlayer, req clientpackets.RequestPledgeInfo) {
	cl, ok := l.clanService().Table().Get(req.ClanID)
	if !ok {
		return
	}
	info := cl.Info()
	live.SendFrame(serverpackets.FramePledgeInfo(info.ID, info.Name, info.AllyName))
	live.SendFrame(serverpackets.FramePledgeStatusChanged(info.LeaderID, info.ID, info.CrestID, info.AllyID, info.AllyCrestID))
}

// requestPledgePower shows live a rank's privileges, or has its leader set
// them; a set by anyone else is ignored.
func (l *GameClientLink) requestPledgePower(live *livePlayer, req clientpackets.RequestPledgePower) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	if !req.SetsPrivileges() {
		live.SendFrame(serverpackets.FrameManagePledgePower(req.Rank, req.Action, cl.RankPrivileges(int(req.Rank))))
		return
	}
	if cl, changed := l.clanService().SetRankPrivileges(live.Character, int(req.Rank), req.Privs); changed {
		l.broadcastClanStatus(cl)
	}
}

// requestPledgePowerGradeList sends live how many members hold each rank.
func (l *GameClientLink) requestPledgePowerGradeList(live *livePlayer) {
	if cl, ok := l.clanService().ClanOf(live.Character); ok {
		live.SendFrame(serverpackets.FramePledgePowerGradeList(cl.PowerGradeCounts()))
	}
}

// requestPledgeMemberPowerInfo sends live a member's rank and its
// privileges.
func (l *GameClientLink) requestPledgeMemberPowerInfo(live *livePlayer, req clientpackets.RequestPledgeMemberName) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	m, ok := cl.MemberByName(req.Name)
	if !ok {
		return
	}
	live.SendFrame(serverpackets.FramePledgeReceivePowerInfo(int32(m.PowerGrade), m.Name, cl.RankPrivileges(m.PowerGrade)))
}

// requestPledgeMemberInfo sends live a member's detail card.
func (l *GameClientLink) requestPledgeMemberInfo(live *livePlayer, req clientpackets.RequestPledgeMemberName) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	m, ok := cl.MemberByName(req.Name)
	if !ok {
		return
	}
	live.SendFrame(l.frameMemberInfo(cl, m))
}

// frameMemberInfo builds m's detail card: its clan's name, or its
// sub-unit's.
func (l *GameClientLink) frameMemberInfo(cl *clan.Clan, m clan.Member) wire.Frame {
	title := m.Title
	if m.Online {
		if member, ok := l.livePlayerByID(m.ObjectID); ok {
			title = member.Character.Title()
		}
	}
	info := serverpackets.PledgeMemberInfo{
		PledgeType: int32(m.PledgeType), Name: m.Name, Title: title, PowerGrade: int32(m.PowerGrade),
		Mentor: mentorName(cl, m),
	}
	if m.PledgeType == clan.SubunitMain {
		info.PledgeName = cl.Name()
	} else if unit, ok := cl.Subunit(m.PledgeType); ok {
		info.PledgeName = unit.Name
	}
	return serverpackets.FramePledgeReceiveMemberInfo(info)
}

// mentorName is the name of m's apprentice, else of its sponsor; "Error"
// when that member is gone, "" when m has neither.
func mentorName(cl *clan.Clan, m clan.Member) string {
	id := m.Apprentice
	if id == 0 {
		id = m.Sponsor
	}
	if id == 0 {
		return ""
	}
	if mentor, ok := cl.Member(id); ok {
		return mentor.Name
	}
	return "Error"
}

// requestPledgeSetMemberPowerGrade gives a member a rank, and shows the
// clan its new row and the change.
func (l *GameClientLink) requestPledgeSetMemberPowerGrade(live *livePlayer, req clientpackets.RequestPledgeSetMemberPowerGrade) {
	cl, m, result := l.clanService().SetMemberGrade(live.Character, req.Name, int(req.PowerGrade))
	switch result {
	case clan.GradeIgnored:
		return
	case clan.GradeNotAuthorized:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	row := l.memberUpdateRow(m)
	l.broadcastToClan(cl, 0,
		func() wire.Frame { return serverpackets.FramePledgeShowMemberListUpdate(row) },
		func() wire.Frame {
			return serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageClanMemberS1PrivilegeChangedToS2, m.Name, int32(m.PowerGrade))
		})
}
