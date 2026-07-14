package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const acquireSkillTypeUsual int32 = 0

func (l *GameClientLink) sendAcquireSkillInfo(live *livePlayer, req clientpackets.RequestAcquireSkillInfo) {
	if !validAcquireSkillRequest(req.SkillID, req.Level) || req.SkillType != acquireSkillTypeUsual {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		return
	}
	grant, ok := learnableGeneralSkill(live, int(req.SkillID), int(req.Level))
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		return
	}
	if !l.skillDefinitionLoaded(int(req.SkillID), int(req.Level)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		return
	}
	live.SendFrame(serverpackets.FrameAcquireSkillInfo(req.SkillID, req.Level, int32(grant.CorrectedCost()), acquireSkillTypeUsual, nil))
}

func (l *GameClientLink) learnAcquireSkill(ctx context.Context, live *livePlayer, req clientpackets.RequestAcquireSkill) {
	if !validAcquireSkillRequest(req.SkillID, req.Level) || req.SkillType != acquireSkillTypeUsual {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		return
	}
	if !l.skillDefinitionLoaded(int(req.SkillID), int(req.Level)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		return
	}
	grant, status := live.template.CheckSkillLearn(live.Level, live.SP, live.SkillLevels(), int(req.SkillID), int(req.Level))
	switch status {
	case player.LearnAllowed:
	case player.LearnNeedsSP:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughSPToLearnSkill))
		live.SendFrame(l.acquireSkillList(live))
		return
	default:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		live.SendFrame(l.acquireSkillList(live))
		return
	}
	if l.skills != nil {
		if err := l.skills.SetKnownSkill(ctx, live.Character, grant.SkillID, grant.Level); err != nil {
			l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("learn skill")
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNothingHappened))
			return
		}
	} else {
		live.SetSkillLevel(grant.SkillID, grant.Level)
	}
	cost := grant.CorrectedCost()
	if cost > 0 {
		live.RemoveSp(cost)
	}

	live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageLearnedSkill, req.SkillID, req.Level))
	if cost > 0 {
		live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusSP, Value: live.SP},
		}))
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	live.SendFrame(l.acquireSkillList(live))
}

func validAcquireSkillRequest(skillID, level int32) bool {
	return skillID > 0 && level > 0
}

func learnableGeneralSkill(live *livePlayer, skillID, level int) (player.SkillGrant, bool) {
	if live == nil || live.template == nil {
		return player.SkillGrant{}, false
	}
	grant, ok := live.template.FindSkillGrant(skillID, level)
	if !ok || grant.MinLevel > live.Level || grant.Cost == 0 || live.SkillLevel(skillID) != level-1 {
		return player.SkillGrant{}, false
	}
	return grant, true
}

func (l *GameClientLink) skillDefinitionLoaded(skillID, level int) bool {
	return l != nil && l.skills != nil && l.skills.hasDefinition(modelskill.Ref{ID: modelskill.ID(skillID), Level: level})
}

func (l *GameClientLink) acquireSkillList(live *livePlayer) wire.Frame {
	grants := acquireSkillListEntries(live)
	entries := grants[:0]
	for _, grant := range grants {
		if l.skillDefinitionLoaded(int(grant.ID), int(grant.Level)) {
			entries = append(entries, grant)
		}
	}
	return serverpackets.FrameAcquireSkillList(serverpackets.AcquireSkillTypeUsual, entries)
}

func acquireSkillListEntries(live *livePlayer) []serverpackets.AcquireSkillListEntry {
	if live == nil || live.template == nil {
		return nil
	}
	grants := live.template.AvailableSkillGrants(live.Level, live.SkillLevels())
	entries := make([]serverpackets.AcquireSkillListEntry, 0, len(grants))
	for _, grant := range grants {
		entries = append(entries, serverpackets.AcquireSkillListEntry{
			ID:    int32(grant.SkillID),
			Level: int32(grant.Level),
			Cost:  int32(grant.CorrectedCost()),
		})
	}
	return entries
}
