package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

const (
	acquireSkillTypeUsual   int32 = 0
	acquireSkillTypeFishing int32 = 1

	// spellbookRequirementType is the requirement kind tag marking a
	// spellbook item in an AcquireSkillInfo requirement entry.
	spellbookRequirementType = 99
	// spellbookRequirementUnk is the trailing field Java sends with
	// spellbook requirements; its value is part of the wire contract.
	spellbookRequirementUnk = 50

	// fishingRequirementType marks the item consumed to learn a fishing
	// skill in an AcquireSkillInfo requirement entry.
	fishingRequirementType = 4
)

// RequestAcquireSkillInfo and RequestAcquireSkill carry an int32 skill type
// whose values are the AcquireSkillType the trainer list belongs to. The
// pledge (clan) type is recognized but handled as unavailable: the clan
// runtime the pledge flow requires is not ported yet.

func (l *GameClientLink) sendAcquireSkillInfo(live *livePlayer, req clientpackets.RequestAcquireSkillInfo) {
	if !skillstate.ValidAcquireRequest(req.SkillID, req.Level) {
		return
	}
	switch req.SkillType {
	case acquireSkillTypeUsual:
		l.sendGeneralAcquireSkillInfo(live, req)
	case acquireSkillTypeFishing:
		l.sendFishingAcquireSkillInfo(live, req)
	default:
		// Pledge (clan) and other types are not handled here: the pledge
		// runtime that owns clan skills, reputation, and leader authority
		// is not ported yet, so pledge-skill info is left unanswered until
		// that lands.
	}
}

func (l *GameClientLink) learnAcquireSkill(ctx context.Context, live *livePlayer, req clientpackets.RequestAcquireSkill) {
	if !skillstate.ValidAcquireRequest(req.SkillID, req.Level) {
		return
	}
	switch req.SkillType {
	case acquireSkillTypeUsual:
		l.learnGeneralAcquireSkill(ctx, live, req)
	case acquireSkillTypeFishing:
		l.learnFishingAcquireSkill(ctx, live, req)
	default:
		// Pledge-skill learning is deferred for the same reason as the
		// info path: it needs the unported pledge runtime.
	}
}

func (l *GameClientLink) sendGeneralAcquireSkillInfo(live *livePlayer, req clientpackets.RequestAcquireSkillInfo) {
	if live == nil {
		return
	}
	grant, ok := skillstate.LearnableGeneral(live.template, live.Level, live.SkillLevels(), int(req.SkillID), int(req.Level))
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		return
	}
	if !l.skillDefinitionLoaded(int(req.SkillID), int(req.Level)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		return
	}
	var reqs []serverpackets.SkillRequirement
	if bookID := l.spellbooks.BookForSkill(modelskill.ID(req.SkillID), int(req.Level)); bookID > 0 {
		reqs = []serverpackets.SkillRequirement{{Type: spellbookRequirementType, ItemID: bookID, Count: 1, Unknown: spellbookRequirementUnk}}
	}
	live.SendFrame(serverpackets.FrameAcquireSkillInfo(req.SkillID, req.Level, int32(grant.CorrectedCost()), acquireSkillTypeUsual, reqs))
}

func (l *GameClientLink) learnGeneralAcquireSkill(ctx context.Context, live *livePlayer, req clientpackets.RequestAcquireSkill) {
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
	if !l.skillDefinitionLoaded(int(req.SkillID), int(req.Level)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoMoreSkillsToLearn))
		live.SendFrame(l.acquireSkillList(live))
		return
	}
	if bookID := l.spellbooks.BookForSkill(modelskill.ID(req.SkillID), int(req.Level)); bookID > 0 {
		if live.Inventory() == nil || live.Inventory().DestroyByTemplateID(bookID, 1) == nil {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemMissingToLearnSkill))
			live.SendFrame(l.acquireSkillList(live))
			return
		}
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

func (l *GameClientLink) sendFishingAcquireSkillInfo(live *livePlayer, req clientpackets.RequestAcquireSkillInfo) {
	if live == nil {
		return
	}
	node, ok := skillstate.LearnableFishing(l.skillTrees, live.Level, live.HasDwarvenCraft(), live.SkillLevels(), l.skillDefinitionLoaded, int(req.SkillID), int(req.Level))
	if !ok {
		return
	}
	reqs := []serverpackets.SkillRequirement{{Type: fishingRequirementType, ItemID: node.ItemID, Count: int32(node.ItemCount)}}
	live.SendFrame(serverpackets.FrameAcquireSkillInfo(req.SkillID, req.Level, 0, acquireSkillTypeFishing, reqs))
}

func (l *GameClientLink) learnFishingAcquireSkill(ctx context.Context, live *livePlayer, req clientpackets.RequestAcquireSkill) {
	if live == nil {
		return
	}
	node, ok := skillstate.LearnableFishing(l.skillTrees, live.Level, live.HasDwarvenCraft(), live.SkillLevels(), l.skillDefinitionLoaded, int(req.SkillID), int(req.Level))
	if !ok {
		return
	}
	if live.Inventory() == nil || live.Inventory().DestroyByTemplateID(node.ItemID, node.ItemCount) == nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemMissingToLearnSkill))
		live.SendFrame(l.fishingAcquireSkillList(live))
		return
	}
	if l.skills != nil {
		if err := l.skills.SetKnownSkill(ctx, live.Character, int(req.SkillID), int(req.Level)); err != nil {
			l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("learn fishing skill")
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNothingHappened))
			return
		}
	} else {
		live.SetSkillLevel(int(req.SkillID), int(req.Level))
	}

	live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageLearnedSkill, req.SkillID, req.Level))
	if skillstate.NeedsStorageSync(req.SkillID) {
		live.SendFrame(serverpackets.FrameExStorageMaxCount(live.Character))
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	live.SendFrame(l.fishingAcquireSkillList(live))
}

func (l *GameClientLink) skillDefinitionLoaded(skillID, level int) bool {
	return l != nil && l.skills != nil && l.skills.HasDefinition(modelskill.Ref{ID: modelskill.ID(skillID), Level: level})
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

// fishingAcquireSkillList builds the fishing-type trainer list of skills the
// character can learn now; each entry's displayed cost is 0 and its row tag
// is 1 (the fishing marker), matching the oracle's FishingSkillNode layout.
func (l *GameClientLink) fishingAcquireSkillList(live *livePlayer) wire.Frame {
	if l.skillTrees == nil || live == nil {
		return serverpackets.FrameAcquireSkillList(serverpackets.AcquireSkillTypeFishing, nil)
	}
	nodes := l.skillTrees.FishingSkillsFor(live.Level, live.HasDwarvenCraft(), skillstate.TreeSkillLevels(live.SkillLevels()))
	entries := make([]serverpackets.AcquireSkillListEntry, 0, len(nodes))
	for _, node := range nodes {
		entries = append(entries, serverpackets.AcquireSkillListEntry{
			ID:      int32(node.ID),
			Level:   int32(node.Level),
			Unknown: 1,
		})
	}
	return serverpackets.FrameAcquireSkillList(serverpackets.AcquireSkillTypeFishing, entries)
}
