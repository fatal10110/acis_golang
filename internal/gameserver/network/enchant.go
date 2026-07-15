package network

import (
	"context"
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const (
	enchantChanceMagicWeapon       = 0.4
	enchantChanceMagicWeapon15Plus = 0.2
	enchantChanceWeapon            = 0.7
	enchantChanceWeapon15Plus      = 0.35
	enchantChanceArmor             = 0.66
	enchantSafeMax                 = 3
	enchantSafeMaxFull             = 4
	enchantMaxWeapon               = 0
	enchantMaxArmor                = 0
)

type enchantScroll struct {
	weapon  bool
	blessed bool
	grade   item.CrystalType
}

var enchantScrolls = map[int32]enchantScroll{
	729:  {weapon: true, grade: item.CrystalA},
	947:  {weapon: true, grade: item.CrystalB},
	951:  {weapon: true, grade: item.CrystalC},
	955:  {weapon: true, grade: item.CrystalD},
	959:  {weapon: true, grade: item.CrystalS},
	730:  {grade: item.CrystalA},
	948:  {grade: item.CrystalB},
	952:  {grade: item.CrystalC},
	956:  {grade: item.CrystalD},
	960:  {grade: item.CrystalS},
	6569: {weapon: true, blessed: true, grade: item.CrystalA},
	6571: {weapon: true, blessed: true, grade: item.CrystalB},
	6573: {weapon: true, blessed: true, grade: item.CrystalC},
	6575: {weapon: true, blessed: true, grade: item.CrystalD},
	6577: {weapon: true, blessed: true, grade: item.CrystalS},
	6570: {blessed: true, grade: item.CrystalA},
	6572: {blessed: true, grade: item.CrystalB},
	6574: {blessed: true, grade: item.CrystalC},
	6576: {blessed: true, grade: item.CrystalD},
	6578: {blessed: true, grade: item.CrystalS},
	731:  {weapon: true, grade: item.CrystalA},
	949:  {weapon: true, grade: item.CrystalB},
	953:  {weapon: true, grade: item.CrystalC},
	957:  {weapon: true, grade: item.CrystalD},
	961:  {weapon: true, grade: item.CrystalS},
	732:  {grade: item.CrystalA},
	950:  {grade: item.CrystalB},
	954:  {grade: item.CrystalC},
	958:  {grade: item.CrystalD},
	962:  {grade: item.CrystalS},
}

func (l *GameClientLink) useEnchantScroll(live *livePlayer, scroll *item.Instance) bool {
	if live == nil || scroll == nil {
		return false
	}
	if _, ok := enchantScrolls[scroll.TemplateID]; !ok {
		return false
	}
	if live.activeEnchantScrollObjectID == 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSelectItemToEnchant))
	}
	live.activeEnchantScrollObjectID = scroll.ObjectID
	live.SendFrame(serverpackets.FrameChooseInventoryItem(scroll.TemplateID))
	return true
}

func (l *GameClientLink) enchantLiveItem(ctx context.Context, live *livePlayer, req clientpackets.RequestEnchantItem) {
	if !liveItemOpsAllowed(live) || req.ObjectID == 0 {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}

	target := inv.ItemByObjectID(req.ObjectID)
	scroll := inv.ItemByObjectID(live.activeEnchantScrollObjectID)
	if target == nil || scroll == nil {
		cancelActiveEnchant(live)
		return
	}
	scrollTemplate, ok := enchantScrolls[scroll.TemplateID]
	if !ok {
		return
	}

	targetTemplate, ok := inv.Templates().Get(target.TemplateID)
	if !ok || !scrollTemplate.valid(target, targetTemplate) || !enchantable(target, targetTemplate) {
		failEnchantCondition(live)
		return
	}

	destroyedScroll := inv.DestroyItem(scroll, 1)
	if destroyedScroll == nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		live.activeEnchantScrollObjectID = 0
		live.SendFrame(serverpackets.FrameEnchantResult(serverpackets.EnchantResultCancelled))
		return
	}
	persistDestroyedOrUpdated(ctx, l, destroyedScroll)

	chance := scrollTemplate.chance(target, targetTemplate)
	if target.OwnerID != live.ObjectID() || !enchantable(target, targetTemplate) || chance < 0 {
		failEnchantCondition(live)
		l.sendInventoryUpdate(live, inv)
		return
	}

	if l.rollEnchant() < chance {
		l.enchantSuccess(ctx, live, target)
	} else if scrollTemplate.blessed {
		l.blessedEnchantFailure(ctx, live, target)
	} else {
		l.normalEnchantFailure(ctx, live, target, targetTemplate)
	}

	l.broadcastEquipmentChange(live)
	live.activeEnchantScrollObjectID = 0
}

func (l *GameClientLink) enchantSuccess(ctx context.Context, live *livePlayer, target *item.Instance) {
	oldLevel := target.EnchantLevel
	if oldLevel == 0 {
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1SuccessfullyEnchanted, target.TemplateID))
	} else {
		live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageS1S2SuccessfullyEnchanted, int32(oldLevel), target.TemplateID))
	}
	if live.Inventory().SetEnchantLevel(target, oldLevel+1) {
		persistUpdate(ctx, l, target)
	}
	l.sendInventoryUpdate(live, live.Inventory())
	live.SendFrame(serverpackets.FrameEnchantResult(serverpackets.EnchantResultSuccess))
}

func (l *GameClientLink) blessedEnchantFailure(ctx context.Context, live *livePlayer, target *item.Instance) {
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageBlessedEnchantFailed))
	if live.Inventory().SetEnchantLevel(target, 0) {
		persistUpdate(ctx, l, target)
	}
	l.sendInventoryUpdate(live, live.Inventory())
	live.SendFrame(serverpackets.FrameEnchantResult(serverpackets.EnchantResultUnsuccess))
}

func (l *GameClientLink) normalEnchantFailure(ctx context.Context, live *livePlayer, target *item.Instance, tmpl *item.Template) {
	crystalID := tmpl.Crystal.ItemID()
	crystalCount := breakCrystalCount(tmpl, target.EnchantLevel)
	if crystalCount < 1 {
		crystalCount = 1
	}

	if inv := live.Inventory(); inv.DestroyItem(target, target.Count) == nil {
		live.activeEnchantScrollObjectID = 0
		live.SendFrame(serverpackets.FrameEnchantResult(serverpackets.EnchantResultCancelled))
		return
	}
	persistDelete(ctx, l, target.ObjectID)

	if crystalID != 0 {
		crystal := l.addCrystalReward(ctx, live, crystalID, crystalCount)
		if crystal != nil {
			live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageEarnedS2S1S, crystalID, int32(crystalCount)))
		}
	}

	if target.EnchantLevel > 0 {
		live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageEnchantmentFailedS1S2Evaporated, int32(target.EnchantLevel), target.TemplateID))
	} else {
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageEnchantmentFailedS1Evaporated, target.TemplateID))
	}
	l.sendInventoryUpdate(live, live.Inventory())
	if crystalID == 0 {
		live.SendFrame(serverpackets.FrameEnchantResult(serverpackets.EnchantResultBrokenNoCrystals))
		return
	}
	live.SendFrame(serverpackets.FrameEnchantResult(serverpackets.EnchantResultBrokenWithCrystals))
}

func (l *GameClientLink) addCrystalReward(ctx context.Context, live *livePlayer, crystalID int32, count int) *item.Instance {
	if l.ids == nil {
		return nil
	}
	if _, ok := live.Inventory().Templates().Get(crystalID); !ok {
		return nil
	}
	objectID, err := l.ids.NextID()
	if err != nil {
		l.log.Error().Err(err).Msg("allocate enchant crystal item id")
		return nil
	}
	crystal := live.Inventory().AddNew(crystalID, count, objectID)
	if crystal != nil {
		persistSave(ctx, l, crystal)
	}
	return crystal
}

func failEnchantCondition(live *livePlayer) {
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInappropriateEnchantCondition))
	live.activeEnchantScrollObjectID = 0
	live.SendFrame(serverpackets.FrameEnchantResult(serverpackets.EnchantResultCancelled))
}

func cancelActiveEnchant(live *livePlayer) {
	if live == nil || live.activeEnchantScrollObjectID == 0 {
		return
	}
	live.activeEnchantScrollObjectID = 0
	live.SendFrame(serverpackets.FrameEnchantResult(serverpackets.EnchantResultCancelled))
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageEnchantScrollCancelled))
}

func persistDestroyedOrUpdated(ctx context.Context, l *GameClientLink, inst *item.Instance) {
	if inst.Count == 0 {
		persistDelete(ctx, l, inst.ObjectID)
		return
	}
	persistUpdate(ctx, l, inst)
}

func persistSave(ctx context.Context, l *GameClientLink, inst *item.Instance) {
	if l.items == nil {
		return
	}
	if err := l.items.Save(ctx, inst); err != nil {
		l.log.Error().Err(err).Int32("object_id", inst.ObjectID).Msg("save item")
	}
}

func persistUpdate(ctx context.Context, l *GameClientLink, inst *item.Instance) {
	if l.items == nil {
		return
	}
	if err := l.items.Update(ctx, inst); err != nil {
		l.log.Error().Err(err).Int32("object_id", inst.ObjectID).Msg("update item")
	}
}

func persistDelete(ctx context.Context, l *GameClientLink, objectID int32) {
	if l.items == nil {
		return
	}
	if err := l.items.Delete(ctx, objectID); err != nil {
		l.log.Error().Err(err).Int32("object_id", objectID).Msg("delete item")
	}
}

func (l *GameClientLink) rollEnchant() float64 {
	if l.enchantRoll != nil {
		return l.enchantRoll()
	}
	return rnd.GetFloat(1)
}

func (s enchantScroll) valid(inst *item.Instance, tmpl *item.Template) bool {
	if inst == nil || tmpl == nil {
		return false
	}
	switch tmpl.Kind {
	case item.KindWeapon:
		if !s.weapon || (enchantMaxWeapon > 0 && inst.EnchantLevel >= enchantMaxWeapon) {
			return false
		}
	case item.KindArmor:
		if s.weapon || (enchantMaxArmor > 0 && inst.EnchantLevel >= enchantMaxArmor) {
			return false
		}
	default:
		return false
	}
	return s.grade == tmpl.Crystal
}

func (s enchantScroll) chance(inst *item.Instance, tmpl *item.Template) float64 {
	if !s.valid(inst, tmpl) {
		return -1
	}
	fullBody := tmpl.Slot == item.SlotFullArmor
	if inst.EnchantLevel < enchantSafeMax || (fullBody && inst.EnchantLevel < enchantSafeMaxFull) {
		return 1
	}
	switch tmpl.Kind {
	case item.KindArmor:
		return math.Pow(enchantChanceArmor, float64(inst.EnchantLevel-2))
	case item.KindWeapon:
		if tmpl.Weapon != nil && tmpl.Weapon.Magical {
			if inst.EnchantLevel > 14 {
				return enchantChanceMagicWeapon15Plus
			}
			return enchantChanceMagicWeapon
		}
		if inst.EnchantLevel > 14 {
			return enchantChanceWeapon15Plus
		}
		return enchantChanceWeapon
	default:
		return 0
	}
}

func enchantable(inst *item.Instance, tmpl *item.Template) bool {
	if inst == nil || tmpl == nil {
		return false
	}
	if tmpl.HeroItem() || inst.ShadowItem(tmpl) || tmpl.Kind == item.KindEtcItem {
		return false
	}
	if tmpl.Weapon != nil && tmpl.Weapon.Type == item.WeaponFishingRod {
		return false
	}
	if inst.Location != item.LocationInventory && inst.Location != item.LocationPaperdoll {
		return false
	}
	if tmpl.Kind == item.KindWeapon {
		return tmpl.ID < 7822 || tmpl.ID > 7831
	}
	return true
}

func breakCrystalCount(tmpl *item.Template, enchantLevel int) int {
	count := int(tmpl.CrystalCountAt(enchantLevel) - (tmpl.CrystalCount+1)/2)
	if count < 1 {
		return 1
	}
	return count
}
