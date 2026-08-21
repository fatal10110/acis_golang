package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) useItem(live *livePlayer, objectID int32) {
	if live == nil {
		return
	}
	if live.Operating() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemsUnavailableForStore))
		return
	}
	if l.trades != nil && l.trades.HasActive(live.ObjectID()) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotPickupOrUseItemTrading))
		return
	}
	inv := live.Inventory()
	if inv == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if live.ItemDisabled(objectID) {
		return
	}
	if inst.QuestItem(tmpl) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotUseQuestItems))
		return
	}
	if !liveItemInteractionAllowed(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if itemhandler.ItemBlockedByKarmaTeleport(tmpl, l.skills, live.Karma(), l.playerConfig.KarmaPlayerCanTeleport) {
		return
	}
	if live.Fishing() && tmpl.DefaultAction != item.ActionFishingShot {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDoWhileFishing))
		return
	}
	if !inst.Equipped() && rejectUseItemConditions(live, tmpl) {
		return
	}
	if l.useEnchantScroll(live, inst) {
		return
	}
	if l.useConsumableSkillItem(live, inv, inst) {
		return
	}
	if l.useItemAICast(live, inv, inst) {
		return
	}
	if l.useSummonItem(live, inv, inst) {
		return
	}
	if l.useShotItem(live, inv, inst) {
		return
	}
	if l.useBeastShotItem(live, inv, inst) {
		return
	}
	res, ok := l.inventory.ToggleEquipItem(inv, objectID)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.applyEquipStatChanges(live, inv, res)
	l.broadcastEquipmentChange(live)
}

// applyEquipStatChanges attaches or detaches the stat functions each
// instance in res.Changed contributes while equipped — item-attached
// passive skills and equip modifiers — based on its current equip state.
// Unequip and equip changes for the same paperdoll slot arrive in that
// order (the old occupant first, the new one second), mirroring the
// reference's unequip-before-equip listener sequencing for one slot swap.
func (l *GameClientLink) applyEquipStatChanges(live *livePlayer, inv *itemcontainer.Inventory, res invops.Result) {
	if live == nil || inv == nil {
		return
	}
	if l.skills != nil {
		var skillsChanged, timersChanged bool
		for _, inst := range res.Changed {
			tmpl, ok := inv.Templates().Get(inst.TemplateID)
			if !ok {
				continue
			}
			if inst.Equipped() {
				changed, timers, err := l.skills.EquipItemStats(live.Character, inst, tmpl)
				if err != nil {
					l.log.Error().Err(err).Int32("object_id", inst.ObjectID).Msg("equip item stats")
				}
				skillsChanged = skillsChanged || changed
				timersChanged = timersChanged || timers
				continue
			}
			if l.skills.UnequipItemStats(live.Character, inv, inst, tmpl) {
				skillsChanged = true
			}
		}
		// Mirrors ItemPassiveSkillsListener: SkillList always precedes
		// SkillCoolTime, and SkillCoolTime is only ever sent when an
		// item-granted skill's reuse timer needs to reach the client.
		if skillsChanged {
			live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
		}
		if timersChanged {
			now := time.Now()
			live.SendFrame(serverpackets.FrameSkillCoolTime(skillCoolTimeEntries(live.SkillReuseTimers(now), now)))
		}
	}
	if l.shadowItems != nil {
		for _, inst := range res.Changed {
			tmpl, ok := inv.Templates().Get(inst.TemplateID)
			if !ok {
				continue
			}
			if inst.Equipped() {
				l.shadowItems.Track(live.ObjectID(), inst, tmpl)
			} else {
				l.shadowItems.Untrack(inst)
			}
		}
	}
	live.RefreshExpertisePenalty()
}

// ExpireShadowItem unequips and destroys an exhausted shadow item.
func (l *GameClientLink) ExpireShadowItem(live *livePlayer, inst *item.Instance) {
	if live == nil || inst == nil {
		return
	}
	if l.inventory == nil {
		return
	}
	inv := live.Inventory()
	if inv == nil || inv.ItemByObjectID(inst.ObjectID) != inst {
		return
	}
	count := int(inst.Snapshot().Count)
	if count <= 0 {
		return
	}
	templateID := inst.TemplateID
	res, ok := l.inventory.DestroyItem(inv, inst.ObjectID, count)
	if !ok {
		return
	}
	l.applyEquipStatChanges(live, inv, res)
	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageRemainingManaIsNow0, templateID))
}

func (l *GameClientLink) handleAutoSoulShot(live *livePlayer, req clientpackets.RequestAutoSoulShot) {
	if live == nil || live.AlikeDead() {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	hasItem := inv.ItemByTemplateID(req.ItemID) != nil

	enabled := false
	switch req.Type {
	case 1:
		enabled = true
	case 0:
	default:
		return
	}

	switch live.ToggleAutoSoulShot(req.ItemID, enabled, hasItem, l.hasActiveSummon(live)) {
	case player.AutoSoulShotToggled:
	case player.AutoSoulShotNeedsSummon:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoServitorCannotAutomateUse))
		return
	default:
		return
	}
	live.SendFrame(serverpackets.FrameExAutoSoulShot(req.ItemID, enabled))
	if enabled {
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageUseOfItemWillBeAuto, req.ItemID))
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageAutoUseOfItemCancelled, req.ItemID))
}

func (l *GameClientLink) hasActiveSummon(live *livePlayer) bool {
	if l.world == nil || live == nil {
		return false
	}
	_, ok := l.world.Summon(live.ObjectID())
	return ok
}

// activeServitorTarget returns live's active servitor as a
// skilltarget.Creature, or nil if it has none, has a pet instead, or doesn't
// expose that surface. Matches the reference's `player.hasServitor()` gate
// (`Player.java:2986-2990`, checking `_summon instanceof Servitor`): a pet
// alone does not qualify.
func (l *GameClientLink) activeServitorTarget(live *livePlayer) skilltarget.Creature {
	if l.world == nil || live == nil {
		return nil
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return nil
	}
	if pet, ok := obj.(interface{ IsPet() bool }); ok && pet.IsPet() {
		return nil
	}
	target, ok := obj.(skilltarget.Creature)
	if !ok {
		return nil
	}
	return target
}

// unequipItem clears whatever item occupies the paperdoll position that
// bodySlot (a Slot bitmask value from the item's own template) resolves
// to. An empty or unresolvable slot answers FrameActionFailed per the
// packet-impact rule (#829); it is not a silent no-op.
func (l *GameClientLink) unequipItem(live *livePlayer, bodySlot int32) {
	inv := live.Inventory()
	if inv == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	paperdollSlot, ok := item.Slot(bodySlot).PaperdollIndex()
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	worn := inv.ItemAt(paperdollSlot)
	if worn == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if !liveItemInteractionAllowed(live) || (live.cast != nil && live.cast.CastingNow()) {
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1CannotBeUsed, worn.TemplateID))
		return
	}
	res, ok := l.inventory.UnequipBodySlot(inv, bodySlot)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.applyEquipStatChanges(live, inv, res)
	l.broadcastEquipmentChange(live)

	if len(res.Changed) > 0 {
		unequipped := res.Changed[0]
		if unequipped.EnchantLevel > 0 {
			live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageEquipmentS1S2Removed, int32(unequipped.EnchantLevel), unequipped.TemplateID))
		} else {
			live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1Disarmed, unequipped.TemplateID))
		}
	}
}

func (l *GameClientLink) dropLiveItem(live *livePlayer, req clientpackets.RequestDropItem) {
	if !liveItemOpsAllowed(live) || l.groundItems == nil || req.Count < 0 {
		return
	}
	if req.Count == 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDiscardThisItem))
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	if req.ObjectID == live.Character.MountObjectID() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	count := int(req.Count)
	if !dropInRange(live, int(req.X), int(req.Y), int(req.Z)) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDiscardDistanceTooFar))
		return
	}

	res, ok, err := l.inventory.DropItem(inv, req.ObjectID, count)
	if err != nil {
		l.log.Error().Err(err).Msg("allocate dropped item id")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	ground, err := grounditem.New(*res.Dropped, res.Template)
	if err != nil {
		l.log.Error().Err(err).Msg("build dropped ground item")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}

	l.groundItems.Drop(ground, task.DropOptions{
		X:             int(req.X),
		Y:             int(req.Y),
		Z:             int(req.Z),
		Heading:       live.CurrentHeading(),
		PlayerDropped: true,
		DropperID:     live.ObjectID(),
	})
}

func (l *GameClientLink) destroyLiveItem(live *livePlayer, objectID int32, count int) {
	if !liveItemOpsAllowed(live) {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	failure := l.inventory.DestroyItemFailure(inv, objectID, count)
	switch failure {
	case invops.DestroyOK:
	case invops.DestroyInvalidCount:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDestroyNumberIncorrect))
		return
	case invops.DestroyNotDestroyable:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDiscardThisItem))
		return
	case invops.DestroyHeroItem:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageHeroWeaponsCantDestroyed))
		return
	default:
		return
	}
	if objectID == live.Character.MountObjectID() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	res, failure := l.inventory.DestroyItemResult(inv, objectID, count)
	if failure != invops.DestroyOK {
		return
	}
	l.applyEquipStatChanges(live, inv, res)
	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}
}

func (l *GameClientLink) crystallizeLiveItem(live *livePlayer, req clientpackets.RequestCrystallizeItem) {
	if !liveItemOpsAllowed(live) || req.Count <= 0 {
		return
	}
	inv := live.Inventory()
	res, failure, err := l.inventory.CrystallizeItem(inv, req.ObjectID, int(req.Count), live.SkillLevel(crystallizeSkillID))
	if err != nil {
		l.log.Error().Err(err).Msg("allocate crystal item id")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	switch failure {
	case invops.CrystallizeOK:
	case invops.CrystallizeNoSkill:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCrystallizeLevelTooLow))
		return
	case invops.CrystallizeGradeTooHigh:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCrystallizeLevelTooLow))
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	default:
		return
	}

	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageItemCrystallized, res.SourceItemID))
	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}
}

// broadcastEquipmentChange resends UserInfo to live (refreshing its own
// paperdoll/stats) and CharInfo to every client that already knows about
// it (refreshing the worn-item visuals on their screen).
func (l *GameClientLink) broadcastEquipmentChange(live *livePlayer) {
	l.broadcastCharacterInfo(live)
}

// broadcastCharacterInfo resends UserInfo to live (refreshing its own
// visible state) and CharInfo to every client that already knows about it,
// matching the reference's broadcastUserInfo().
func (l *GameClientLink) broadcastCharacterInfo(live *livePlayer) {
	items := live.inventoryItems()
	live.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{
		Character: live.Character, Template: live.template, Items: items, IsGM: live.isGM,
	}))
	if l.world == nil {
		return
	}
	info := serverpackets.CharInfoSnapshot{Character: live.Character, Template: live.template, Items: items}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameCharInfo(info)
	}, func(send func(frameReceiver)) {
		l.world.ForEachKnown(live, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}

// liveItemOpsAllowed reports whether live may currently manipulate items at
// all: not gone and not dead. Drop/destroy/crystallize/enchant/pet-use gate
// on this alone — RequestDropItem.java:36 checks isDead() only,
// RequestDestroyItem.java/RequestEnchantItem.java check nothing, and
// RequestPetUseItem.java:34 checks isAlikeDead()||pet.isDead(). Pickup does
// not gate on this alone; see liveItemInteractionAllowed and
// livePickupBlockedDeferrable's comment (pickup.go).
func liveItemOpsAllowed(live *livePlayer) bool {
	return live != nil && !live.AlikeDead()
}

// liveItemInteractionAllowed reports whether live may currently use or
// equip/unequip an item, or pick one up off the ground: not gone, not dead,
// and free of the crowd-control quartet that locks item interaction
// (stunned, sleeping, paralyzed, or afraid). This is the union the reference
// applies to UseItem.java:66 and RequestUnEquipItem.java:37 directly, and to
// pickup indirectly via PlayableAI.tryToPickUp's denyAiAction() gate
// (PlayableAI.java:411-417, Creature.java:636-639) — denyAiAction also folds
// in teleporting/immobile-until-attacked/dead, which this port doesn't model
// for pickup any more than it does for use/unequip (documented deferred
// gaps).
func liveItemInteractionAllowed(live *livePlayer) bool {
	return liveItemOpsAllowed(live) && !live.Stunned() && !live.Sleeping() && !live.Paralyzed() && !live.Afraid()
}

func dropInRange(live *livePlayer, x, y, z int) bool {
	sx, sy, sz := live.Position()
	return location.In3DRange(sx, sy, sz, x, y, z, dropInteractionDistance)
}
