package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

const (
	acquireSkillTypeUsual   int32 = 0
	acquireSkillTypeFishing int32 = 1
	acquireSkillTypeClan    int32 = 2

	// spellbookRequirementType is the requirement kind tag marking a
	// spellbook item in an AcquireSkillInfo requirement entry.
	spellbookRequirementType = 99
	// spellbookRequirementUnk is the trailing field sent with spellbook
	// requirements; its value is part of the wire contract.
	spellbookRequirementUnk = 50

	// fishingRequirementType marks the item consumed to learn a fishing
	// skill in an AcquireSkillInfo requirement entry.
	fishingRequirementType = 4
)

// RequestAcquireSkillInfo and RequestAcquireSkill carry an int32 skill type
// whose values are the AcquireSkillType the trainer list belongs to. Both
// are made at the civilian NPC the player last selected and can still
// interact with; without one they answer nothing. A skill type outside the
// usual, fishing and clan lists answers nothing either, as specified.

func (l *GameClientLink) sendAcquireSkillInfo(live *livePlayer, req clientpackets.RequestAcquireSkillInfo) {
	if live == nil || !skillstate.ValidAcquireRequest(req.SkillID, req.Level) {
		return
	}
	trainer, ok := l.currentTrainer(live)
	if !ok {
		return
	}
	switch req.SkillType {
	case acquireSkillTypeUsual:
		if !trainer.CanTeach(live.ClassID()) {
			return
		}
		l.sendGeneralAcquireSkillInfo(live, req)
	case acquireSkillTypeFishing:
		l.sendFishingAcquireSkillInfo(live, req)
	case acquireSkillTypeClan:
		if l.skillDefinitionLoaded(int(req.SkillID), int(req.Level)) {
			l.sendClanAcquireSkillInfo(live, req)
		}
	}
}

func (l *GameClientLink) learnAcquireSkill(live *livePlayer, req clientpackets.RequestAcquireSkill) {
	if live == nil || !skillstate.ValidAcquireRequest(req.SkillID, req.Level) {
		return
	}
	trainer, ok := l.currentTrainer(live)
	if !ok {
		return
	}
	switch req.SkillType {
	case acquireSkillTypeUsual:
		l.learnGeneralAcquireSkill(live, trainer, req)
	case acquireSkillTypeFishing:
		l.learnFishingAcquireSkill(live, req)
	case acquireSkillTypeClan:
		if l.skillDefinitionLoaded(int(req.SkillID), int(req.Level)) {
			l.learnClanSkill(live, req)
		}
	}
}

func (l *GameClientLink) sendGeneralAcquireSkillInfo(live *livePlayer, req clientpackets.RequestAcquireSkillInfo) {
	offer, ok := skillstate.GeneralOfferFor(live.Character, live.Template(), l.skills, l.spellbooks, int(req.SkillID), int(req.Level))
	if !ok {
		return
	}
	var reqs []serverpackets.SkillRequirement
	if offer.BookID > 0 {
		reqs = []serverpackets.SkillRequirement{{Type: spellbookRequirementType, ItemID: offer.BookID, Count: 1, Unknown: spellbookRequirementUnk}}
	}
	live.SendFrame(serverpackets.FrameAcquireSkillInfo(req.SkillID, req.Level, int32(offer.Grant.CorrectedCost()), acquireSkillTypeUsual, reqs))
}

// learnGeneralAcquireSkill learns a usual skill at trainer. The learn does
// not ask whether trainer trains live's profession; the list it reopens
// does. A skill that is not the next learnable level answers nothing.
func (l *GameClientLink) learnGeneralAcquireSkill(live *livePlayer, trainer *npc.Folk, req clientpackets.RequestAcquireSkill) {
	_, status, err := skillstate.LearnGeneral(live.Character, live.Template(), l.skills, l.spellbooks, int(req.SkillID), int(req.Level))
	if err != nil {
		// The skill is learned and paid for; only its passive stats failed to
		// build from a malformed definition. The reference cannot fail here
		// (Player.addSkill), so the client gets the full learn reply.
		l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("learn skill: bad passive definition")
	}
	switch status {
	case skillstate.LearnDone:
	case skillstate.LearnNeedsSP:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughSPToLearnSkill))
		l.showSkillList(live, trainer)
		return
	case skillstate.LearnMissingItem:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemMissingToLearnSkill))
		l.showSkillList(live, trainer)
		return
	default:
		return
	}

	live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageLearnedSkill, req.SkillID, req.Level))
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	l.refreshSkillShortcuts(live, req.SkillID, req.Level)
	l.showSkillList(live, trainer)
}

func (l *GameClientLink) sendFishingAcquireSkillInfo(live *livePlayer, req clientpackets.RequestAcquireSkillInfo) {
	offer, ok := skillstate.FishingOfferFor(live.Character, l.skillTrees, l.skills, int(req.SkillID), int(req.Level))
	if !ok {
		return
	}
	node := offer.Node
	reqs := []serverpackets.SkillRequirement{{Type: fishingRequirementType, ItemID: node.ItemID, Count: int32(node.ItemCount)}}
	live.SendFrame(serverpackets.FrameAcquireSkillInfo(req.SkillID, req.Level, 0, acquireSkillTypeFishing, reqs))
}

func (l *GameClientLink) learnFishingAcquireSkill(live *livePlayer, req clientpackets.RequestAcquireSkill) {
	result, status, err := skillstate.LearnFishing(live.Character, l.skillTrees, l.skills, int(req.SkillID), int(req.Level))
	if err != nil {
		// As in learnGeneralAcquireSkill: the learn stands, the definition is bad.
		l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("learn fishing skill: bad passive definition")
	}
	switch status {
	case skillstate.LearnDone:
	case skillstate.LearnMissingItem:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemMissingToLearnSkill))
		l.showFishSkillList(live)
		return
	default:
		return
	}

	live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageLearnedSkill, req.SkillID, req.Level))
	if result.StorageSync {
		live.SendFrame(serverpackets.FrameExStorageMaxCount(live.Character))
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	l.refreshSkillShortcuts(live, req.SkillID, req.Level)
	l.showFishSkillList(live)
}

func (l *GameClientLink) skillDefinitionLoaded(skillID, level int) bool {
	return l != nil && l.skills != nil && l.skills.HasDefinition(modelskill.Ref{ID: modelskill.ID(skillID), Level: level})
}

// acquireSkillList builds the usual trainer list of skills live can learn
// now; ok is false when there is none.
func (l *GameClientLink) acquireSkillList(live *livePlayer) (list wire.Frame, ok bool) {
	grants := acquireSkillListEntries(live)
	entries := grants[:0]
	for _, grant := range grants {
		if l.skillDefinitionLoaded(int(grant.ID), int(grant.Level)) {
			entries = append(entries, grant)
		}
	}
	if len(entries) == 0 {
		return wire.Frame{}, false
	}
	return serverpackets.FrameAcquireSkillList(serverpackets.AcquireSkillTypeUsual, entries), true
}

func acquireSkillListEntries(live *livePlayer) []serverpackets.AcquireSkillListEntry {
	if live == nil || live.Template() == nil {
		return nil
	}
	grants := live.Template().AvailableSkillGrants(live.Level(), live.SkillLevels())
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
// is 1 (the fishing marker), the fishing skill-node row layout.
// ok is false when there is none.
func (l *GameClientLink) fishingAcquireSkillList(live *livePlayer) (list wire.Frame, ok bool) {
	nodes := l.skillTrees.FishingSkillsFor(live.Level(), live.HasDwarvenCraft(), skillstate.TreeSkillLevels(live.SkillLevels()))
	if len(nodes) == 0 {
		return wire.Frame{}, false
	}
	entries := make([]serverpackets.AcquireSkillListEntry, 0, len(nodes))
	for _, node := range nodes {
		entries = append(entries, serverpackets.AcquireSkillListEntry{
			ID:      int32(node.ID),
			Level:   int32(node.Level),
			Unknown: 1,
		})
	}
	return serverpackets.FrameAcquireSkillList(serverpackets.AcquireSkillTypeFishing, entries), true
}
