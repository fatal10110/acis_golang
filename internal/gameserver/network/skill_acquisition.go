package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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

	// storageSyncSkill{Lo,Hi} bound the skill ids whose learning prompts an
	// ExStorageMaxCount refresh ( Expanded the player's inventory limits).
	storageSyncSkillLo int32 = 1368
	storageSyncSkillHi int32 = 1372
)

// RequestAcquireSkillInfo and RequestAcquireSkill carry an int32 skill type
// whose values are the AcquireSkillType the trainer list belongs to. The
// pledge (clan) type is recognized but handled as unavailable: the clan
// runtime the pledge flow requires is not ported yet.

func (l *GameClientLink) sendAcquireSkillInfo(live *livePlayer, req clientpackets.RequestAcquireSkillInfo) {
	if !validAcquireSkillRequest(req.SkillID, req.Level) {
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
	if !validAcquireSkillRequest(req.SkillID, req.Level) {
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
	grant, ok := learnableGeneralSkill(live, int(req.SkillID), int(req.Level))
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
	node, ok := l.learnableFishingSkill(live, int(req.SkillID), int(req.Level))
	if !ok {
		return
	}
	reqs := []serverpackets.SkillRequirement{{Type: fishingRequirementType, ItemID: node.ItemID, Count: int32(node.ItemCount)}}
	live.SendFrame(serverpackets.FrameAcquireSkillInfo(req.SkillID, req.Level, 0, acquireSkillTypeFishing, reqs))
}

func (l *GameClientLink) learnFishingAcquireSkill(ctx context.Context, live *livePlayer, req clientpackets.RequestAcquireSkill) {
	node, ok := l.learnableFishingSkill(live, int(req.SkillID), int(req.Level))
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
	if req.SkillID >= storageSyncSkillLo && req.SkillID <= storageSyncSkillHi {
		live.SendFrame(serverpackets.FrameExStorageMaxCount(live.Character))
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	live.SendFrame(l.fishingAcquireSkillList(live))
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

// learnableFishingSkill returns the fishing-skill node for the requested id and
// level when it is the next learnable level for this character. Returns false
// when no skill trees are configured or the node is not learnable now.
func (l *GameClientLink) learnableFishingSkill(live *livePlayer, skillID, level int) (modelskill.FishingSkill, bool) {
	if l.skillTrees == nil || live == nil {
		return modelskill.FishingSkill{}, false
	}
	if live.SkillLevel(skillID) >= level || live.SkillLevel(skillID) != level-1 {
		return modelskill.FishingSkill{}, false
	}
	node, ok := l.skillTrees.FishingSkillFor(live.Level, live.HasDwarvenCraft(), treeSkillLevels(live), modelskill.ID(skillID), level)
	if !ok || !l.skillDefinitionLoaded(skillID, level) {
		return modelskill.FishingSkill{}, false
	}
	return node, true
}

// treeSkillLevels returns the character's known skill levels keyed by skill
// id as the skill-tree model expects them.
func treeSkillLevels(live *livePlayer) modelskill.SkillLevels {
	src := live.SkillLevels()
	out := make(modelskill.SkillLevels, len(src))
	for id, lvl := range src {
		out[modelskill.ID(id)] = lvl
	}
	return out
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

// fishingAcquireSkillList builds the fishing-type trainer list of skills the
// character can learn now; each entry's displayed cost is 0 and its row tag
// is 1 (the fishing marker), matching the oracle's FishingSkillNode layout.
func (l *GameClientLink) fishingAcquireSkillList(live *livePlayer) wire.Frame {
	if l.skillTrees == nil {
		return serverpackets.FrameAcquireSkillList(serverpackets.AcquireSkillTypeFishing, nil)
	}
	nodes := l.skillTrees.FishingSkillsFor(live.Level, live.HasDwarvenCraft(), treeSkillLevels(live))
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
