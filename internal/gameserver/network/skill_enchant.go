package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// enchantSkillRequirementType marks the item consumed to attempt a skill
// enchant in an ExEnchantSkillInfo requirement entry.
const enchantSkillRequirementType = 4

func (l *GameClientLink) sendEnchantSkillInfo(live *livePlayer, req clientpackets.RequestExEnchantSkillInfo) {
	if live == nil {
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
	if l.skillEnchantSPBookNeeded && node.ItemID != 0 {
		info.Requirements = []serverpackets.EnchantSkillRequirement{
			{Type: enchantSkillRequirementType, ItemID: node.ItemID, Count: int32(node.ItemCount)},
		}
	}
	live.SendFrame(serverpackets.FrameExEnchantSkillInfo(info))
}

func (l *GameClientLink) applyEnchantSkill(ctx context.Context, live *livePlayer, req clientpackets.RequestExEnchantSkill) {
	if live == nil {
		return
	}
	_, status, err := skillstate.Enchant(ctx, live.Character, l.levels, live.template, l.skillTrees, l.skills, l.skillEnchantSPBookNeeded, l.rollEnchantSkill, int(req.SkillID), int(req.SkillLevel))
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

	if status == skillstate.EnchantSucceeded {
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageSucceededEnchantingSkillS1, req.SkillID, req.SkillLevel))
	} else {
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageFailedEnchantingSkillS1, req.SkillID, req.SkillLevel))
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	live.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{
		Character: live.Character,
		Template:  live.template,
		Items:     live.inventoryItems(),
	}))
}
