package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Texts the sub-unit commands answer with.
const (
	subunitMissingText     = "Pledge doesn't exist."
	removeMentorsFirstText = "Remove previous connections first."
)

// villageMasterSubunit runs one of a village master's sub-unit commands
// for live, reporting whether command is one. A command missing a
// required argument does nothing.
func (l *GameClientLink) villageMasterSubunit(live *livePlayer, verb, arg, arg2 string) bool {
	switch verb {
	case "create_academy":
		if arg != "" {
			l.createSubunit(live, clan.SubunitAcademy, arg, "")
		}
	case "create_royal":
		if arg != "" {
			l.createSubunit(live, clan.SubunitRoyal1, arg, arg2)
		}
	case "create_knight":
		if arg != "" {
			l.createSubunit(live, clan.SubunitKnight1, arg, arg2)
		}
	case "rename_pledge":
		if arg != "" && arg2 != "" {
			l.renameSubunit(live, arg, arg2)
		}
	case "assign_subpl_leader":
		if arg != "" {
			l.assignSubunitCaptain(live, arg, arg2)
		}
	default:
		return false
	}
	return true
}

// createSubunit founds a sub-unit of kind kind named name in live's clan,
// captained by the member named captain.
func (l *GameClientLink) createSubunit(live *livePlayer, kind int, name, captain string) {
	change, result := l.clanService().CreateSubunit(live.Character, kind, name, captain)
	if result != clan.SubunitDone {
		l.sendSubunitRefusal(live, kind, name, result)
		return
	}
	cl, unit := change.Clan, change.Unit
	if change.Paid {
		l.sendReputationChange(cl, change.Reputation, live)
	}
	leaderName := cl.SubunitLeaderName(unit.ID)
	l.broadcastToClan(cl, 0,
		func() wire.Frame { return framePledgeShowInfoUpdate(cl) },
		func() wire.Frame {
			return serverpackets.FramePledgeReceiveSubPledgeCreated(int32(unit.ID), unit.Name, leaderName)
		})
	created := serverpackets.SystemMessageS1ClanAcademyCreated
	switch {
	case clan.IsKnight(unit.ID):
		created = serverpackets.SystemMessageKnightsOfS1Created
	case clan.IsRoyal(unit.ID):
		created = serverpackets.SystemMessageRoyalGuardOfS1Created
	}
	live.SendFrame(serverpackets.FrameSystemMessageString(created, cl.Name()))
	if unit.ID != clan.SubunitAcademy {
		l.refreshCaptain(change.Captain)
	}
}

// refreshCaptain recomputes a new captain's clan rank and shows it its
// status, on its own queue; nothing when it is offline.
func (l *GameClientLink) refreshCaptain(m clan.Member) {
	captain, ok := l.livePlayerByID(m.ObjectID)
	if !ok {
		return
	}
	postLive(captain, func() {
		l.clanService().RefreshPledgeClass(captain.Character)
		captain.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(captain)))
	})
}

// sendSubunitRefusal tells live why its sub-unit command, about a sub-unit
// of pledge type unit named name, was refused.
func (l *GameClientLink) sendSubunitRefusal(live *livePlayer, unit int, name string, result clan.SubunitResult) {
	academy := unit == clan.SubunitAcademy
	message := -1
	switch result {
	case clan.SubunitNotLeader:
		message = serverpackets.SystemMessageNotAuthorizedToDoThat
	case clan.SubunitLevelTooLow:
		message = serverpackets.SystemMessageNotMeetCriteriaForMilitaryUnit
		if academy {
			message = serverpackets.SystemMessageNotMeetCriteriaForAcademy
		}
	case clan.SubunitNameInvalid, clan.SubunitNotMilitary:
		message = serverpackets.SystemMessageClanNameInvalid
	case clan.SubunitNameLength:
		message = serverpackets.SystemMessageClanNameLengthIncorrect
	case clan.SubunitNameTaken:
		if academy {
			live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1AlreadyExists, name))
			return
		}
		message = serverpackets.SystemMessageMilitaryUnitNameTaken
	case clan.SubunitCaptainInvalid:
		switch {
		case unit >= clan.SubunitKnight1:
			message = serverpackets.SystemMessageKnightCaptainCannotBeAppointed
		case unit >= clan.SubunitRoyal1:
			message = serverpackets.SystemMessageRoyalCaptainCannotBeAppointed
		}
	case clan.SubunitNoSlot:
		message = serverpackets.SystemMessageNotMeetCriteriaForMilitaryUnit
		if academy {
			message = serverpackets.SystemMessageClanAlreadyHasAcademy
		}
	case clan.SubunitCaptainIsLeader:
		message = serverpackets.SystemMessageNotMeetCriteriaForMilitaryUnit
	case clan.SubunitReputationTooLow:
		message = serverpackets.SystemMessageClanReputationScoreTooLow
	case clan.SubunitUnknown:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, subunitMissingText))
		return
	case clan.SubunitCaptainNameTooLong:
		message = serverpackets.SystemMessageNamingCharnameUpTo16Chars
	case clan.SubunitCaptainSelf:
		message = serverpackets.SystemMessageRoyalCaptainCannotBeAppointed
	}
	if message >= 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(message))
	}
}

// renameSubunit renames the sub-unit of pledge type idArg to name.
func (l *GameClientLink) renameSubunit(live *livePlayer, idArg, name string) {
	change, result := l.clanService().RenameSubunit(live.Character, idArg, name)
	if result != clan.SubunitDone {
		l.sendSubunitRefusal(live, 0, name, result)
		return
	}
	cl, id := change.Clan, change.Unit.ID
	l.broadcastToClan(cl, 0, func() wire.Frame { return l.framePledgeUnitList(cl, id) })
}

// assignSubunitCaptain names the member called captain captain of the
// royal guard or knight order named unitName.
func (l *GameClientLink) assignSubunitCaptain(live *livePlayer, unitName, captain string) {
	change, result := l.clanService().AssignSubunitCaptain(live.Character, unitName, captain)
	if result != clan.SubunitDone {
		l.sendSubunitRefusal(live, change.Unit.ID, unitName, result)
		return
	}
	cl, id := change.Clan, change.Unit.ID
	l.refreshCaptain(change.Captain)
	l.broadcastToClan(cl, 0,
		func() wire.Frame { return l.framePledgeUnitList(cl, id) },
		func() wire.Frame {
			return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1SelectedAsCaptainOfS2,
				serverpackets.TextParam(captain), serverpackets.TextParam(unitName))
		})
}

// requestPledgeReorganizeMember moves a member between sub-units, or shows
// its card.
func (l *GameClientLink) requestPledgeReorganizeMember(live *livePlayer, req clientpackets.RequestPledgeReorganizeMember) {
	cl, m, result := l.clanService().Reorganize(live.Character, req.Selected != 0, req.MemberName, int(req.NewPledgeType), req.SelectedMemberName)
	switch result {
	case clan.ReorganizeNotAuthorized:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
	case clan.ReorganizeShowMember:
		live.SendFrame(l.frameMemberInfo(cl, m))
	case clan.Reorganized:
		l.broadcastClanStatus(cl)
	}
}

// requestPledgeSetAcademyMaster links or unlinks a sponsor and its
// apprentice, telling both, the requester and the clan.
func (l *GameClientLink) requestPledgeSetAcademyMaster(live *livePlayer, req clientpackets.RequestPledgeSetAcademyMaster) {
	change, result := l.clanService().SetMentor(live.Character, req.Set != 0, req.CurrentName, req.TargetName)
	var message int
	switch result {
	case clan.MentorIgnored:
		return
	case clan.MentorNotAuthorized:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoRightToDismissApprentice))
		return
	case clan.MentorAlreadyLinked:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, removeMentorsFirstText))
		return
	case clan.MentorLinked:
		message = serverpackets.SystemMessageS2DesignatedApprenticeOfS1
	default:
		message = serverpackets.SystemMessageS2ApprenticeOfS1Removed
	}
	sponsor, apprentice := l.onlineMember(change.Sponsor), l.onlineMember(change.Apprentice)
	notice := func() wire.Frame {
		return serverpackets.FrameSystemMessageParams(message,
			serverpackets.TextParam(change.Sponsor.Name), serverpackets.TextParam(change.Apprentice.Name))
	}
	// The requester is told unless it is the sponsor, or the sponsor and
	// apprentice are the same player or both offline.
	if sponsor != live && sponsor != apprentice {
		live.SendFrame(notice())
	}
	if sponsor != nil {
		sponsor.SendFrame(notice())
	}
	if apprentice != nil {
		apprentice.SendFrame(notice())
	}
	sponsorRow, apprenticeRow := l.memberUpdateRow(change.Sponsor), l.memberUpdateRow(change.Apprentice)
	l.broadcastToClan(change.Clan, 0,
		func() wire.Frame { return serverpackets.FramePledgeShowMemberListUpdate(sponsorRow) },
		func() wire.Frame { return serverpackets.FramePledgeShowMemberListUpdate(apprenticeRow) })
}

// onlineMember returns m's live player, nil when it is offline.
func (l *GameClientLink) onlineMember(m clan.Member) *livePlayer {
	if !m.Online {
		return nil
	}
	live, ok := l.livePlayerByID(m.ObjectID)
	if !ok {
		return nil
	}
	return live
}
