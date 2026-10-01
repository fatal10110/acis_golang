package network

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// The war requests a clanless player sends, or that name a clan that does
// not exist (a stop or surrender), are not answered, as specified: none
// leaves a client action pending.

// requestStartPledgeWar has live's clan declare war on the clan it names.
func (l *GameClientLink) requestStartPledgeWar(live *livePlayer, req clientpackets.RequestPledgeWarName) {
	war, result := l.clanService().DeclareWar(live.Character, req.PledgeName, time.Now())
	if result != clan.WarDone {
		l.sendWarRefusal(live, war, result)
		return
	}
	l.broadcastWarDeclared(war)
	l.broadcastClanUserInfo(live, war.Target)
	l.broadcastClanUserInfo(live, war.Clan)
}

// requestStopPledgeWar has live's clan stop its war on the clan it names.
func (l *GameClientLink) requestStopPledgeWar(live *livePlayer, req clientpackets.RequestPledgeWarName) {
	inCombat := func(id int32) bool {
		member, ok := l.livePlayerByID(id)
		return ok && member.Character.InCombat()
	}
	war, result := l.clanService().StopWar(live.Character, req.PledgeName, inCombat, time.Now())
	if result != clan.WarDone {
		l.sendWarRefusal(live, war, result)
		return
	}
	l.broadcastWarEnded(war)
	l.broadcastClanUserInfo(live, war.Target)
	l.broadcastClanUserInfo(live, war.Clan)
}

// requestSurrenderPledgeWar has live's clan give up its war on the clan
// it names: live takes a full death's experience loss, then the war ends.
func (l *GameClientLink) requestSurrenderPledgeWar(live *livePlayer, req clientpackets.RequestPledgeWarName) {
	war, result := l.clanService().CheckSurrender(live.Character, req.PledgeName)
	if result != clan.WarDone {
		l.sendWarRefusal(live, war, result)
		return
	}
	live.Character.ApplyDeathPenalty()
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageYouHaveSurrenderedToS1Clan, req.PledgeName))
	if l.clanService().EndWar(war.Clan, war.Target, time.Now()) {
		l.broadcastWarEnded(war)
	}
}

// requestReplyPledgeWar answers a war, stop or surrender proposal. No
// request ever proposes one to another player, so there is never one to
// answer: each reply is ignored, as one with no proposal pending is. A
// reply to an unrelated pending request (a trade, a duel) must not start,
// stop or surrender a war between the two players' clans.
func (l *GameClientLink) requestReplyPledgeWar(live *livePlayer, opcode byte) {
	l.log.Debug().Int32("object_id", live.ObjectID()).Uint8("opcode", opcode).Msg("clan war reply with no proposal pending")
}

// sendWarRefusal tells live why its war request was refused.
func (l *GameClientLink) sendWarRefusal(live *livePlayer, war clan.War, result clan.WarResult) {
	var frame wire.Frame
	switch result {
	case clan.WarNotAuthorized:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat)
	case clan.WarNoSuchClan:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageWarClanDoesNotExist)
	case clan.WarOwnClan:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDeclareAgainstOwnClan)
	case clan.WarTooMany:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageTooManyClanWars)
	case clan.WarTooWeak:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageWarNeedsLevel3Or15Members)
	case clan.WarTargetTooWeak:
		frame = serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1ClanCannotDeclareWarTooWeak, war.Target.Name())
	case clan.WarAllied:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageWarAgainstAlliedClan)
	case clan.WarTargetDissolving:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoWarAgainstDissolvingClan)
	case clan.WarAlreadyDeclared:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageWarAlreadyDeclared)
	case clan.WarPenalty:
		frame = serverpackets.FrameSystemMessageString(serverpackets.SystemMessageAlreadyAtWarWithS1Wait5Days, war.Target.Name())
	case clan.WarNotInvolved:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotInvolvedInWar)
	case clan.WarMemberInCombat:
		frame = serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotStopWarInCombat)
	default:
		return
	}
	live.SendFrame(frame)
}

// broadcastWarDeclared shows both clans the new war: each its header and
// the declaration notice.
func (l *GameClientLink) broadcastWarDeclared(war clan.War) {
	clanName, targetName := war.Clan.Name(), war.Target.Name()
	l.broadcastToClan(war.Clan, 0,
		func() wire.Frame { return framePledgeShowInfoUpdate(war.Clan) },
		func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageWarDeclaredAgainstS1, targetName)
		})
	l.broadcastToClan(war.Target, 0,
		func() wire.Frame { return framePledgeShowInfoUpdate(war.Target) },
		func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageClanS1DeclaredWar, clanName)
		})
}

// broadcastWarEnded shows both clans the war is over: each its header and
// the stop notice.
func (l *GameClientLink) broadcastWarEnded(war clan.War) {
	clanName, targetName := war.Clan.Name(), war.Target.Name()
	l.broadcastToClan(war.Clan, 0,
		func() wire.Frame { return framePledgeShowInfoUpdate(war.Clan) },
		func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageWarAgainstS1Stopped, targetName)
		})
	l.broadcastToClan(war.Target, 0,
		func() wire.Frame { return framePledgeShowInfoUpdate(war.Target) },
		func() wire.Frame {
			return serverpackets.FrameSystemMessageString(serverpackets.SystemMessageClanS1DecidedToStopWar, clanName)
		})
}

// broadcastClanUserInfo refreshes every online member of cl, itself and
// for the players around it. live, the requester, is refreshed in place;
// every other member on its own queue.
func (l *GameClientLink) broadcastClanUserInfo(live *livePlayer, cl *clan.Clan) {
	for _, member := range l.onlineClanMembers(cl, 0) {
		l.broadcastUserInfoOf(live, member)
	}
}

// broadcastUserInfoOf refreshes member for itself and the players around
// it: in place when it is live, the player whose request runs, else on its
// own queue.
func (l *GameClientLink) broadcastUserInfoOf(live, member *livePlayer) {
	if member == live {
		l.broadcastCharacterInfo(member)
		return
	}
	postLive(member, func() { l.broadcastCharacterInfo(member) })
}

// refreshWarTags refreshes, around live, every player of a clan cl is at
// war with, so their clan-war name tags follow live's clan change.
func (l *GameClientLink) refreshWarTags(live *livePlayer, cl *clan.Clan) {
	if l.world == nil {
		return
	}
	wars := cl.WarList()
	if len(wars) == 0 {
		return
	}
	var enemies []*livePlayer
	l.world.ForEachKnown(live, func(o world.Tracked) {
		if other, ok := o.(*livePlayer); ok && slices.Contains(wars, other.Character.ClanID()) {
			enemies = append(enemies, other)
		}
	})
	for _, enemy := range enemies {
		l.broadcastUserInfoOf(live, enemy)
	}
}

// requestPledgeWarList sends live one page of a tab of its clan's war
// window. The attacker tab's page falls back to 0 past its last page.
func (l *GameClientLink) requestPledgeWarList(live *livePlayer, req clientpackets.RequestPledgeWarList) {
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	page := req.Page
	var ids []int32
	if req.Tab == 0 {
		ids = cl.WarList()
	} else {
		ids = cl.AttackerList()
		if int(page) > len(ids)/13 {
			page = 0
		}
		page = max(0, page)
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		if other, ok := l.clanService().Table().Get(id); ok {
			names = append(names, other.Name())
		}
	}
	live.SendFrame(serverpackets.FramePledgeReceiveWarList(req.Tab, page, len(ids), names))
}
