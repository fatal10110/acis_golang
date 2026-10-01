package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/augment"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The augmentation window's requests answer only with their own packets
// and system messages. A request naming an item the player does not hold
// gets nothing at all, as specified: the window registers no
// pending client action, and only asks again on the player's next drop.

// newAugmentService builds the refine service from cfg's augmentation
// data; without data there is nothing to roll from, the confirmation steps
// answer nothing and every refine fails.
func newAugmentService(cfg GameClientLinkConfig) *augment.Service {
	if cfg.Augmentations == nil {
		return nil
	}
	roll := cfg.AugmentRoll
	if roll == nil {
		roll = rnd.GetRange
	}
	skills := cfg.Skills
	return augment.NewService(cfg.Augmentations, cfg.AugmentationChances, roll, func(id int32, level int) bool {
		return skills != nil && skills.HasDefinition(modelskill.Ref{ID: modelskill.ID(id), Level: level})
	})
}

// augmentState reads the player state augmentation is gated on.
func (l *GameClientLink) augmentState(live *livePlayer) augment.State {
	return augment.State{
		Level:        live.Level(),
		Operating:    live.Operating(),
		Trading:      l.trades != nil && l.trades.ProcessingTransaction(live.ObjectID()),
		Dead:         live.Dead(),
		Paralyzed:    live.Paralyzed(),
		Fishing:      live.Fishing(),
		Sitting:      !live.Standing(),
		CursedWeapon: live.Character.CursedWeaponEquipped(),
	}
}

func sendAugmentMessages(live *livePlayer, msgs []augment.Message) {
	for _, m := range msgs {
		live.SendFrame(serverpackets.FrameSystemMessage(augmentMessageID(m)))
	}
}

func augmentMessageID(m augment.Message) int {
	switch m {
	case augment.MessageWhileOperating:
		return serverpackets.SystemMessageCannotAugmentWhileOperating
	case augment.MessageWhileTrading:
		return serverpackets.SystemMessageCannotAugmentWhileTrading
	case augment.MessageWhileDead:
		return serverpackets.SystemMessageCannotAugmentWhileDead
	case augment.MessageWhileParalyzed:
		return serverpackets.SystemMessageCannotAugmentWhileParalyzed
	case augment.MessageWhileFishing:
		return serverpackets.SystemMessageCannotAugmentWhileFishing
	case augment.MessageWhileSitting:
		return serverpackets.SystemMessageCannotAugmentWhileSitting
	case augment.MessageLifeStoneLevelTooHigh:
		return serverpackets.SystemMessageLifeStoneLevelTooHigh
	case augment.MessageAlreadyAugmented:
		return serverpackets.SystemMessageAlreadyAugmented
	case augment.MessageGemstoneQuantityIncorrect:
		return serverpackets.SystemMessageGemstoneQuantityIncorrect
	case augment.MessageRemovalNeedsAugmentedItem:
		return serverpackets.SystemMessageAugmentationRemovalNeedsAugmentedItem
	default:
		return serverpackets.SystemMessageNotSuitableItem
	}
}

// confirmAugmentTarget answers RequestConfirmTargetItem: the weapon dropped
// into the augmentation window is accepted, or refused with its reason.
func (l *GameClientLink) confirmAugmentTarget(live *livePlayer, req clientpackets.RequestConfirmTargetItem) {
	inv := live.Inventory()
	if inv == nil || l.augment == nil {
		return
	}
	check := l.augment.ConfirmTarget(l.augmentState(live), live.ObjectID(), inv, req.ObjectID)
	sendAugmentMessages(live, check.Messages)
	if check.OK {
		live.SendFrame(serverpackets.FrameExConfirmVariationItem(req.ObjectID))
	}
}

// confirmAugmentRefiner answers RequestConfirmRefinerItem: the life stone
// is accepted together with the gemstones the weapon's grade asks for, or
// refused.
func (l *GameClientLink) confirmAugmentRefiner(live *livePlayer, req clientpackets.RequestConfirmRefinerItem) {
	inv := live.Inventory()
	if inv == nil || l.augment == nil {
		return
	}
	check := l.augment.ConfirmRefiner(l.augmentState(live), live.ObjectID(), inv, req.TargetObjectID, req.RefinerObjectID)
	sendAugmentMessages(live, check.Messages)
	if check.OK {
		live.SendFrame(serverpackets.FrameExConfirmVariationRefiner(req.RefinerObjectID, check.LifeStoneItemID, check.GemstoneItemID, int32(check.GemstoneCount)))
	}
}

// confirmAugmentGemstone answers RequestConfirmGemStone: the gemstone stack
// and count are accepted, or refused.
func (l *GameClientLink) confirmAugmentGemstone(live *livePlayer, req clientpackets.RequestConfirmGemStone) {
	inv := live.Inventory()
	if inv == nil || l.augment == nil {
		return
	}
	check := l.augment.ConfirmGemstone(l.augmentState(live), live.ObjectID(), inv, req.TargetObjectID, req.RefinerObjectID, req.GemstoneObjectID, int(req.GemstoneCount))
	sendAugmentMessages(live, check.Messages)
	if check.OK {
		live.SendFrame(serverpackets.FrameExConfirmVariationGemstone(req.GemstoneObjectID, req.GemstoneCount))
	}
}

// refineAugment answers RequestRefine: the weapon takes a new augmentation
// for one life stone and its gemstones, coming off the paperdoll first when
// worn. Every refusal answers with the failed result and its message.
func (l *GameClientLink) refineAugment(live *livePlayer, req clientpackets.RequestRefine) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	if l.augment == nil {
		sendRefineFailed(live)
		return
	}
	out := l.augment.Refine(augment.RefineRequest{
		State:      l.augmentState(live),
		PlayerID:   live.ObjectID(),
		Inv:        inv,
		TargetID:   req.TargetObjectID,
		RefinerID:  req.RefinerObjectID,
		GemstoneID: req.GemstoneObjectID,
		Count:      int(req.GemstoneCount),
	})
	l.applyPersistActions(out.Persist)
	sendAugmentMessages(live, out.Messages)
	if len(out.Unequipped) > 0 {
		l.applyEquipStatChanges(live, inv, invops.Result{EquipmentChanged: true, Changed: out.Unequipped})
		l.broadcastCharacterInfo(live)
	}
	if out.Failed {
		sendRefineFailed(live)
		return
	}
	id := out.Augmentation.Attributes
	live.SendFrame(serverpackets.FrameExVariationResult(id&0xffff, id>>16, 1))
	l.refreshItemShortcuts(live, out.Target.ObjectID)
}

func sendRefineFailed(live *livePlayer) {
	live.SendFrame(serverpackets.FrameExVariationResultFailed())
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAugmentationFailedInappropriate))
}

// confirmAugmentCancel answers RequestConfirmCancelItem with the price of
// removing the augmentation of the item dropped into the removal window.
func (l *GameClientLink) confirmAugmentCancel(live *livePlayer, req clientpackets.RequestConfirmCancelItem) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	check := augment.ConfirmCancel(live.ObjectID(), inv, req.ObjectID)
	sendAugmentMessages(live, check.Messages)
	if !check.OK {
		return
	}
	aug, _ := check.Item.AugmentationValue()
	live.SendFrame(serverpackets.FrameExConfirmCancelItem(check.Item.ObjectID, check.Item.TemplateID, aug.Attributes, int64(check.Price)))
}

// cancelAugment answers RequestRefineCancel: for its price in adena the
// item loses its augmentation, coming off the paperdoll first when worn.
func (l *GameClientLink) cancelAugment(live *livePlayer, req clientpackets.RequestRefineCancel) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(req.ObjectID)
	if inst == nil {
		live.SendFrame(serverpackets.FrameExVariationCancelResult(0))
		return
	}
	check := augment.ConfirmCancel(live.ObjectID(), inv, req.ObjectID)
	if !check.OK {
		// An item someone else owns answers nothing, as specified.
		if inst.Snapshot().OwnerID != live.ObjectID() {
			return
		}
		sendAugmentMessages(live, check.Messages)
		live.SendFrame(serverpackets.FrameExVariationCancelResult(0))
		return
	}
	if check.Price > inv.Adena() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		live.SendFrame(serverpackets.FrameExVariationCancelResult(0))
		return
	}
	adenaOwner := live.ObjectID()
	var persist []invops.Persist
	if check.Price > 0 {
		paid := inv.DestroyByTemplateID(item.AdenaID, check.Price)
		if paid == nil {
			live.SendFrame(serverpackets.FrameExVariationCancelResult(0))
			return
		}
		persist = append(persist, invops.DestroyedOrUpdated(adenaOwner, paid))
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DisappearedAdena, int32(check.Price)))
	}
	if inst.Equipped() {
		l.disarm(live, false)
	}
	inv.RemoveAugmentation(inst)
	l.applyPersistActions(append(persist, invops.Update(inst)))
	live.SendFrame(serverpackets.FrameExVariationCancelResult(1))
	l.refreshItemShortcuts(live, inst.ObjectID)
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageAugmentationRemovedFromS1, inst.TemplateID))
}

// refreshItemShortcuts resends every shortcut pointing at the item
// objectID, so the bar shows its current augmentation.
func (l *GameClientLink) refreshItemShortcuts(live *livePlayer, objectID int32) {
	if live.shortcuts == nil {
		return
	}
	for _, sc := range live.shortcuts.All() {
		if sc.Type == shortcut.Item && sc.ID == objectID {
			live.SendFrame(serverpackets.FrameShortCutRegister(serverShortcut(live.Inventory(), sc)))
		}
	}
}
