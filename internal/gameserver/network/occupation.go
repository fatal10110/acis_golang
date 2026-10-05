package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// occupationChangeSkillID is the visual an occupation change shows around
// the player.
const occupationChangeSkillID = 5103

// changeOccupation moves live to classID, played with tmpl, as every
// occupation change does: the visual shown around live, the new class in
// place of the old one (an active subclass's slot included), the transfer
// announced, live's row refreshed in its party's and its clan's windows,
// and, when the server grants skills automatically, every skill now
// available granted. An academy member taking its second occupation first
// graduates from its clan (graduateFromAcademy). It reports false, having
// done nothing, while another class change of live is in progress.
func (l *GameClientLink) changeOccupation(live *livePlayer, classID int, tmpl *player.Template) bool {
	if !live.TryLockClassChange() {
		return false
	}
	defer live.UnlockClassChange()
	l.graduateFromAcademy(live, classID)
	self := skillCastObject(live)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(self, self, occupationChangeSkillID, 1, 1000, 0, false)
	})
	live.ChangeOccupation(classID, tmpl)
	msg := serverpackets.SystemMessageClassTransfer
	if tier, _ := player.ClassLevel(classID); tier == 3 {
		msg = serverpackets.SystemMessageThirdClassTransfer
	}
	live.SendFrame(serverpackets.FrameSystemMessage(msg))
	if l.parties != nil {
		if view, ok := l.parties.View(live.ObjectID()); ok {
			row := partyMemberRow(live)
			l.broadcastToMembers(view.Members, func() wire.Frame { return serverpackets.FramePartySmallWindowUpdate(row) })
		}
	}
	if cl, ok := l.clanService().ClanOf(live.Character); ok {
		m, _ := cl.Member(live.ObjectID())
		row := liveMemberRow(live.Character, m)
		l.broadcastToClan(cl, 0, func() wire.Frame { return serverpackets.FramePledgeShowMemberListUpdate(row) })
	}
	if l.playerConfig.AutoLearnSkills {
		l.rewardLiveSkills(live)
	}
	return true
}

// rewardLiveSkills grants live every skill its class and level make
// available, through the shared reward operation: bought grants stored,
// free ones level-derived, Lucky taken away past its level and skills the
// level no longer supports corrected. The raised skills' shortcuts follow,
// then the new skill list.
func (l *GameClientLink) rewardLiveSkills(live *livePlayer) {
	if l.skills == nil {
		return
	}
	before := live.SkillLevels()
	if err := l.skills.RewardSkills(live.Character, live.Template()); err != nil {
		l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("reward skills")
	}
	l.sendSkillChanges(live, before, raisedSkill)
}
