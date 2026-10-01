package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// enchantSkillRequirementType marks the item consumed to attempt a skill
// enchant in an ExEnchantSkillInfo requirement entry.
const enchantSkillRequirementType = 4

// sendEnchantSkillInfo quotes one skill enchant at the civilian NPC the
// player last selected, can still interact with, and is trained by. Every
// refusal answers nothing.
func (l *GameClientLink) sendEnchantSkillInfo(live *livePlayer, req clientpackets.RequestExEnchantSkillInfo) {
	if live == nil {
		return
	}
	trainer, ok := l.currentTrainer(live)
	if !ok || !trainer.CanTeach(live.ClassID) {
		return
	}
	offer, ok := skillstate.EnchantOfferFor(live.Character, l.skillTrees, l.skills, int(req.SkillID), int(req.SkillLevel))
	if !ok {
		return
	}
	node := offer.Skill
	info := serverpackets.EnchantSkillInfo{
		ID:     req.SkillID,
		Level:  req.SkillLevel,
		SPCost: int32(node.SP),
		XPCost: int64(node.Exp),
		Rate:   int32(offer.Rate),
	}
	if l.playerConfig.SkillEnchantSPBookNeeded && node.ItemID != 0 {
		info.Requirements = []serverpackets.EnchantSkillRequirement{
			{Type: enchantSkillRequirementType, ItemID: node.ItemID, Count: int32(node.ItemCount)},
		}
	}
	live.SendFrame(serverpackets.FrameExEnchantSkillInfo(info))
}

// applyEnchantSkill attempts one skill enchant at the civilian NPC the
// player last selected and can still interact with; without one it answers
// nothing. The attempt does not ask whether that NPC trains the player's
// profession; the enchant list it reopens after a roll does.
func (l *GameClientLink) applyEnchantSkill(live *livePlayer, req clientpackets.RequestExEnchantSkill) {
	if live == nil {
		return
	}
	trainer, ok := l.currentTrainer(live)
	if !ok {
		return
	}
	result, status, err := skillstate.Enchant(live.Character, l.levels, live.template, l.skillTrees, l.skills, l.playerConfig.SkillEnchantSPBookNeeded, l.rollEnchantSkill, int(req.SkillID), int(req.SkillLevel))
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("enchant skill")
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNothingHappened))
		return
	}
	switch status {
	case skillstate.EnchantSucceeded, skillstate.EnchantFailed:
	case skillstate.EnchantNeedsSP:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughSPToEnchantSkill))
		return
	case skillstate.EnchantNeedsExp:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughExpToEnchantSkill))
		return
	case skillstate.EnchantMissingItem:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMissingItemsToEnchantSkill))
		return
	default:
		return
	}

	l.refreshSkillShortcuts(live, int32(result.SkillID), int32(result.AppliedLevel))
	if status == skillstate.EnchantSucceeded {
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageSucceededEnchantingSkillS1, req.SkillID, req.SkillLevel))
	} else {
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageFailedEnchantingSkillS1, req.SkillID, req.SkillLevel))
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	l.showEnchantSkillList(live, trainer)
}
