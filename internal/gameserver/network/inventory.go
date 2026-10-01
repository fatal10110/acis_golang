package network

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// useItem handles UseItem. ctrl is the client's Ctrl modifier, carried into
// an item-carried AI cast as its force-use flag. rollDice consumes the
// client's dice reuse gate; see useWindowItem.
func (l *GameClientLink) useItem(live *livePlayer, objectID int32, ctrl bool, rollDice func() bool) {
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
	if l.useItemAICast(live, inv, inst, ctrl) {
		return
	}
	if l.useResurrectionScroll(live, inv, inst) {
		return
	}
	if l.useSummonItem(live, inv, inst) {
		return
	}
	if l.eatPetFood(live, inv, inst) {
		return
	}
	if l.useShotItem(live, inv, inst) {
		return
	}
	if l.useBeastShotItem(live, inv, inst) {
		return
	}
	if l.useRecipeItem(live, inst, tmpl) {
		return
	}
	if l.useWindowItem(live, tmpl, rollDice) {
		return
	}
	if l.usePaganKey(live, inv, inst, tmpl) {
		return
	}
	if l.useTargetCastItem(live, inv, inst, ctrl) {
		return
	}
	if tmpl.Kind == item.KindEtcItem && tmpl.Slot != item.SlotNone {
		l.useOffHandItem(live, inv, inst, tmpl)
		return
	}
	switch tmpl.Slot {
	case item.SlotLRHand, item.SlotLHand, item.SlotRHand:
		if live.Mounted() {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotEquipItemDueToBadCondition))
			return
		}
		l.tryToUseItem(live, inv, inst, tmpl)
		return
	}
	l.toggleEquipItem(live, inv, inst, tmpl, false)
}

// useOffHandItem answers UseItem on arrows or a lure, the etc items that go
// in the left hand. They are not equipment a player puts on or takes off:
// a bow takes its arrows into the left hand by itself, and a lure only goes
// on over a fishing rod, replacing the lure worn, with no message and no
// toggle off. Anything else answers ActionFailed, as UseItem does for any
// item nothing uses (the specified behavior drops it silently).
func (l *GameClientLink) useOffHandItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template) {
	if tmpl.EtcItem == nil || tmpl.EtcItem.Type != item.EtcItemLure || !wieldsFishingRod(inv) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	inv.SetPaperdollItem(itemcontainer.LHand, inst, tmpl)
	l.broadcastEquipmentChange(live)
}

// wieldsFishingRod reports whether inv's right hand holds a fishing rod.
func wieldsFishingRod(inv *itemcontainer.Inventory) bool {
	worn := inv.ItemAt(itemcontainer.RHand)
	if worn == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(worn.TemplateID)
	return ok && tmpl.Weapon != nil && tmpl.Weapon.Type == item.WeaponFishingRod
}

// toggleEquipItem puts inst on, or takes it off when it is worn, for
// UseItem. A weapon loses its shot charges either way. Taking an item off
// announces it before the paperdoll changes; putting one on announces it
// after, then recharges auto-use shots for a main-hand weapon. With
// abortAttack the attack in progress stops and ActionFailed answers it once
// the refresh is out. An equip change that moved the inventory limit
// resends the storage limits last.
func (l *GameClientLink) toggleEquipItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template, abortAttack bool) {
	st := inst.Snapshot()
	oldInventoryLimit := live.InventoryLimit()
	if tmpl.Kind == item.KindWeapon {
		inst.UnchargeAllShots()
	}
	if st.Equipped() {
		sendUnequippedMessage(live, st.TemplateID, st.EnchantLevel)
	}
	res, failure := l.inventory.ToggleEquipItem(inv, st.ObjectID)
	switch failure {
	case invops.EquipOK:
	case invops.EquipBadCondition:
		// The paperdoll is unchanged, but the refusal still ends in the
		// usual UserInfo/CharInfo refresh.
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotEquipItemDueToBadCondition))
		l.broadcastEquipmentChange(live)
		return
	default:
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.applyEquipItemStats(live, inv, res)
	if !st.Equipped() {
		if st.EnchantLevel > 0 {
			live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageS1S2Equipped, int32(st.EnchantLevel), st.TemplateID))
		} else {
			live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1Equipped, st.TemplateID))
		}
		if tmpl.Slot == item.SlotLRHand || tmpl.Slot == item.SlotRHand {
			l.rechargeShots(live, inv, true, true)
		}
	}
	live.RefreshExpertisePenalty()
	l.broadcastEquipmentChange(live)
	if abortAttack {
		if live.attack != nil {
			live.attack.Stop()
		}
		live.SendFrame(serverpackets.FrameActionFailed())
	}
	if live.InventoryLimit() != oldInventoryLimit {
		live.SendFrame(serverpackets.FrameExStorageMaxCount(live.Character))
	}
}

// sendUnequippedMessage names an item that came off the paperdoll, with
// its enchant level when it has one.
func sendUnequippedMessage(live *livePlayer, templateID int32, enchantLevel int) {
	if enchantLevel > 0 {
		live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageEquipmentS1S2Removed, int32(enchantLevel), templateID))
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1Disarmed, templateID))
}

// applyEquipStatChanges attaches or detaches the stat functions each
// instance in res.Changed contributes while equipped — item-attached
// passive skills and equip modifiers — based on its current equip state.
// Unequip and equip changes for the same paperdoll slot arrive in that
// order (the old occupant first, the new one second): a slot swap runs its
// unequip listeners before its equip listeners.
// Putting on or taking off formal wear always resends SkillList, since
// every entry's greyed-out flag follows it.
func (l *GameClientLink) applyEquipStatChanges(live *livePlayer, inv *itemcontainer.Inventory, res invops.Result) {
	if live == nil || inv == nil {
		return
	}
	l.applyEquipItemStats(live, inv, res)
	live.RefreshExpertisePenalty()
}

// applyEquipItemStats is applyEquipStatChanges without the closing grade
// penalty refresh, for a caller that has to send its own packets between
// the two.
//
// Each instance in res.Changed is one listener step, run in that order
// against the paperdoll as it stood at that step: its formal wear refresh,
// armor set and augmentation stages each answer with their own SkillList
// (and SkillCoolTime), then its item skills with one more, so a request
// that changes several slots sends one per item that changed skills. A set
// piece cleared ahead of the chest is still checked against the old chest,
// and every SkillList greys out its entries only while formal wear is worn
// at that step.
func (l *GameClientLink) applyEquipItemStats(live *livePlayer, inv *itemcontainer.Inventory, res invops.Result) {
	if live == nil || inv == nil {
		return
	}
	steps := equipSteps(inv, res.Changed)
	for i, inst := range res.Changed {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok {
			continue
		}
		l.applyEquipStep(live, inst, tmpl, steps[i])
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
}

// applyEquipStep attaches or detaches the stat functions and skills inst,
// just equipped or unequipped, contributes, answering each stage with its
// own packets. SkillList always precedes SkillCoolTime, and SkillCoolTime is
// only ever sent when an item-granted skill's reuse timer needs to reach
// the client.
func (l *GameClientLink) applyEquipStep(live *livePlayer, inst *item.Instance, tmpl *item.Template, step equipStep) {
	sendSkillList := func() {
		live.SendFrame(serverpackets.FrameSkillList(skillListEntriesGreyed(live.Character, l.skills, step.formalWear)))
	}
	sendCoolTime := func() {
		now := live.Now()
		live.SendFrame(serverpackets.FrameSkillCoolTime(skillCoolTimeEntries(live.SkillReuseTimers(now), now)))
	}
	if l.skills == nil {
		if tmpl.Slot == item.SlotAllDress {
			sendSkillList()
		}
		return
	}
	stage := func(change skillstate.SkillChange) {
		if change.SkillsChanged {
			sendSkillList()
		}
		if change.TimersChanged {
			sendCoolTime()
		}
	}
	if inst.Equipped() {
		skillsChanged, timersChanged, err := l.skills.EquipItemStatsReporting(live.Character, inst, tmpl, stage)
		if err != nil {
			l.log.Error().Err(err).Int32("object_id", inst.ObjectID).Msg("equip item stats")
		}
		stage(skillstate.SkillChange{SkillsChanged: skillsChanged, TimersChanged: timersChanged})
		return
	}
	if l.skills.UnequipItemStatsReporting(live.Character, step.worn, inst, tmpl, stage) {
		sendSkillList()
	}
}

// equipStep is the paperdoll one listener step of an equip change sees:
// what is worn once that step's item went on or came off, and whether the
// chest then holds formal wear.
type equipStep struct {
	worn       skillstate.WornStep
	formalWear bool
}

// equipSteps replays changed, in the order the paperdoll changed them,
// backwards from the paperdoll inv wears now: an item worn now went on at
// its step, so it was not worn before it; one not worn now came off at its
// step, so it was worn before it.
func equipSteps(inv *itemcontainer.Inventory, changed []*item.Instance) []equipStep {
	worn := inv.PaperdollItems()
	steps := make([]equipStep, len(changed))
	for i := len(changed) - 1; i >= 0; i-- {
		steps[i] = paperdollStep(inv.Templates(), slices.Clone(worn))
		inst := changed[i]
		if inst.Equipped() {
			worn = slices.DeleteFunc(worn, func(w *item.Instance) bool { return w == inst })
		} else {
			worn = append(worn, inst)
		}
	}
	return steps
}

// paperdollStep resolves the chest among worn.
func paperdollStep(templates *item.Table, worn []*item.Instance) equipStep {
	step := equipStep{worn: skillstate.WornStep{Items: worn}}
	for _, inst := range worn {
		tmpl, ok := templates.Get(inst.TemplateID)
		if !ok {
			continue
		}
		if slot, ok := tmpl.Slot.PaperdollIndex(); ok && slot == itemcontainer.Chest {
			step.worn.ChestID = inst.TemplateID
			step.formalWear = tmpl.Slot == item.SlotAllDress
		}
	}
	return step
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
	if live == nil || live.AlikeDead() || live.Operating() {
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

// hasActiveSummon reports whether live has a summon in the world, and
// answers no for a summon cast still resolving its pets row: the summon
// slot is filled only after the pet row is restored, so it is empty across
// that read.
// restoringSummon reports whether live has a summon cast that already hit
// and is still resolving its pets row. It stands in for the casting state
// held across that read, for the gates that refuse a player who is casting
// — never for hasActiveSummon, which has to keep answering whether the
// summon slot is filled.
func (l *GameClientLink) restoringSummon(live *livePlayer) bool {
	return live != nil && live.petRestoreInFlight.Load()
}

func (l *GameClientLink) hasActiveSummon(live *livePlayer) bool {
	if l.world == nil || live == nil {
		return false
	}
	_, ok := l.world.Summon(live.ObjectID())
	return ok
}

// activeServitorTarget returns live's active servitor as a
// skilltarget.Actor, or nil if it has none, has a pet instead, or doesn't
// expose that surface. Only a servitor qualifies: a pet alone does not.
func (l *GameClientLink) activeServitorTarget(live *livePlayer) skilltarget.Actor {
	if l.world == nil || live == nil {
		return nil
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return nil
	}
	if pet, ok := obj.(*summon.Actor); ok && pet.IsPet() {
		return nil
	}
	target, ok := obj.(skilltarget.Actor)
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
	for _, unequipped := range res.Changed {
		unequipped.UnchargeAllShots()
	}
	l.applyEquipStatChanges(live, inv, res)
	l.broadcastEquipmentChange(live)

	if len(res.Changed) > 0 {
		unequipped := res.Changed[0].Snapshot()
		sendUnequippedMessage(live, unequipped.TemplateID, unequipped.EnchantLevel)
	}
}

// dropLiveItem answers RequestDropItem. Whether the item can be discarded
// at all is settled first: a pet's collar while the pet is out, the selected
// enchant scroll, a missing item or a bad count, and any drop by a non-GM
// when the server allows no discards. The player's own state comes next:
// an access level without transactions, a trade or a store, fishing. The
// distance is last, so none of these ever reads as too far.
//
// An augmented item and, for a non-GM, a quest item are refused here too;
// neither gets this far, since an augmented item is not
// droppable and a quest item is ignored above.
func (l *GameClientLink) dropLiveItem(live *livePlayer, req clientpackets.RequestDropItem) {
	if !liveItemOpsAllowed(live) || l.groundItems == nil {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	count := int(req.Count)
	refused := l.boundItems(live)(req.ObjectID) || (l.playerConfig.DiscardItemDisabled && !live.access.IsGM)
	switch l.inventory.DropItemFailure(inv, req.ObjectID, count, refused) {
	case invops.DropOK:
	case invops.DropCannotDiscard:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDiscardThisItem))
		return
	default:
		// The specified behavior ignores these requests without an answer;
		// ActionFailed releases the drag without a message.
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if !live.access.AllowTransaction {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAuthorizedToDoThat))
		return
	}
	if l.busyTrading(live) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotTradeDiscardDropInShopMode))
		return
	}
	if live.Fishing() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDoWhileFishing2))
		return
	}
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
	// A worn item leaves the paperdoll before it reaches the ground: its
	// stat functions, item skills and shot charges go with it.
	for _, unequipped := range res.Changed {
		unequipped.UnchargeAllShots()
	}
	l.applyEquipStatChanges(live, inv, res.Result)
	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}

	ground, err := grounditem.New(*res.Dropped, res.Template)
	if err != nil {
		l.log.Error().Err(err).Msg("build dropped ground item")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
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

// destroyLiveItem answers RequestDestroyItem. A player running a private
// store or tied up in a direct trade is refused before anything else; a
// destroy that goes through names what disappeared.
func (l *GameClientLink) destroyLiveItem(live *livePlayer, objectID int32, count int) {
	if live == nil {
		return
	}
	if l.busyTrading(live) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotTradeDiscardDropInShopMode))
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
	if live.ControlItemInUse(objectID) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetSummonedMayNotDestroyed))
		return
	}
	l.unequipDestroyedItem(live, inv, objectID, count)
	templateID := int32(0)
	if inst := inv.ItemByObjectID(objectID); inst != nil {
		templateID = inst.TemplateID
	}
	res, failure := l.inventory.DestroyItemResult(inv, objectID, count)
	if failure != invops.DestroyOK {
		return
	}
	sendDestroyedMessage(live, templateID, count)
	l.applyEquipStatChanges(live, inv, res)
	if res.EquipmentChanged {
		l.broadcastEquipmentChange(live)
	}
}

// sendDestroyedMessage names count units of templateID the player just
// destroyed: a shadow item reads as its mana running out, several units
// carry their count.
func sendDestroyedMessage(live *livePlayer, templateID int32, count int) {
	switch {
	case shadowTemplate(live, templateID):
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageRemainingManaIsNow0, templateID))
	case count > 1:
		live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageS2S1Disappeared, templateID, int32(count)))
	default:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1Disappeared, templateID))
	}
}

// unequipDestroyedItem takes a worn item off before a destroy consumes all
// of it, the way UseItem takes it off: the removal message ahead of the
// paperdoll change, the weapon's shot charges, the grade penalty refresh,
// UserInfo/CharInfo and a moved storage limit. A destroy that leaves part
// of a worn stack behind keeps it worn.
func (l *GameClientLink) unequipDestroyedItem(live *livePlayer, inv *itemcontainer.Inventory, objectID int32, count int) {
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return
	}
	st := inst.Snapshot()
	if !st.Equipped() || st.Count > count {
		return
	}
	tmpl, ok := inv.Templates().Get(st.TemplateID)
	if !ok {
		return
	}
	l.toggleEquipItem(live, inv, inst, tmpl, false)
}

// crystallizeLiveItem answers RequestCrystallizeItem. A player running a
// private store is refused first. A player already crystallizing is
// refused too, by a flag that only guards this same handler against
// itself: requests run one at a time on the player's queue, so no second
// crystallize can start while one is under way here.
func (l *GameClientLink) crystallizeLiveItem(live *livePlayer, req clientpackets.RequestCrystallizeItem) {
	if live == nil || req.Count <= 0 {
		return
	}
	if live.Operating() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotTradeDiscardDropInShopMode))
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

	l.applyEquipStatChanges(live, inv, res.Result)
	if res.EquipmentChanged {
		sendUnequippedMessage(live, res.SourceItemID, res.SourceEnchantLevel)
	}
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageItemCrystallized, res.SourceItemID))
	live.ItemAdded(event.ItemObtained{ItemID: res.CrystalItemID, Count: res.CrystalCount, Notice: event.ObtainCreated})
	l.broadcastEquipmentChange(live)
}

// itemObtainedFrame is the chat line naming items that reached a player's
// inventory. Adena names only its amount. A picked-up item names a stack's
// count as a plain number and a single enchanted item's enchant level; an
// item created by id names a stack's count as an item number. An earned
// item reads as earned rather than picked up.
func itemObtainedFrame(e event.ItemObtained) wire.Frame {
	switch {
	case e.Notice == event.ObtainEarned && e.Count > 1:
		return serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageEarnedS2S1S, e.ItemID, int32(e.Count))
	case e.Notice == event.ObtainEarned:
		return serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageEarnedItemS1, e.ItemID)
	case e.Notice == event.ObtainAdena:
		return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageEarnedS1Adena, int32(e.Count))
	case e.Count > 1 && e.Notice == event.ObtainPickup:
		return serverpackets.FrameSystemMessageItemNameNumber(serverpackets.SystemMessageYouPickedUpS2S1, e.ItemID, int32(e.Count))
	case e.Count > 1:
		return serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageYouPickedUpS2S1, e.ItemID, int32(e.Count))
	case e.EnchantLevel > 0 && e.Notice == event.ObtainPickup:
		return serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageYouPickedUpAS1S2, int32(e.EnchantLevel), e.ItemID)
	default:
		return serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageYouPickedUpS1, e.ItemID)
	}
}

// broadcastEquipmentChange resends UserInfo to live (refreshing its own
// paperdoll/stats) and CharInfo to every client that already knows about
// it (refreshing the worn-item visuals on their screen).
func (l *GameClientLink) broadcastEquipmentChange(live *livePlayer) {
	l.broadcastCharacterInfo(live)
}

// broadcastCharacterInfo resends UserInfo to live (refreshing its own
// visible state) and CharInfo to every client that already knows about it.
func (l *GameClientLink) broadcastCharacterInfo(live *livePlayer) {
	items := live.inventoryItems()
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	if l.world == nil {
		return
	}
	info := serverpackets.CharInfoSnapshot{Character: live.Character, Template: live.Template(), Items: items}
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

// busyTrading reports whether live runs or is setting up a private store or
// workshop, or is tied up in a direct trade or a pending trade request.
func (l *GameClientLink) busyTrading(live *livePlayer) bool {
	return live.Operating() || l.processingTransaction(live)
}

// processingTransaction reports whether live is tied up in a direct trade
// or a pending trade request.
func (l *GameClientLink) processingTransaction(live *livePlayer) bool {
	return l.trades != nil && l.trades.ProcessingTransaction(live.ObjectID())
}

// liveItemOpsAllowed reports whether live may currently manipulate items at
// all: not gone and not dead. Drop/destroy/crystallize/enchant/pet-use gate
// on this alone — a drop checks only death, destroy and enchant check
// nothing, and pet item use checks the owner's death-like state or the
// pet's death. Pickup does
// not gate on this alone; see liveItemInteractionAllowed and
// livePickupBlockedDeferrable's comment (pickup.go).
func liveItemOpsAllowed(live *livePlayer) bool {
	return live != nil && !live.AlikeDead()
}

// liveItemInteractionAllowed reports whether live may currently use or
// equip/unequip an item, or pick one up off the ground: not gone, not dead,
// and free of the crowd-control quartet that locks item interaction
// (stunned, sleeping, paralyzed, or afraid). This is the union applied to
// UseItem and RequestUnEquipItem directly, and to pickup indirectly via the
// AI's deny-action gate — which also folds
// in teleporting/immobile-until-attacked/dead, which this port doesn't model
// for pickup any more than it does for use/unequip (documented deferred
// gaps).
func liveItemInteractionAllowed(live *livePlayer) bool {
	return liveItemOpsAllowed(live) && !live.Stunned() && !live.Sleeping() && !live.Paralyzed() && !live.Afraid()
}

func dropInRange(live *livePlayer, x, y, z int) bool {
	sx, sy, sz := live.Position()
	return location.In3DRadius(sx, sy, sz, x, y, z, dropInteractionDistance)
}
