package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// shotMessageSet names the client messages one shot handler's rejections
// and success map to.
type shotMessageSet struct {
	noCapacity    int
	gradeMismatch int
	notEnough     int
	enabled       int
}

// shotMessages maps each shot handler name itemhandler.UseShot covers to
// its client messages. Spirit and blessed-spirit share the same message
// ids in the reference.
var shotMessages = map[string]shotMessageSet{
	itemhandler.SoulShotsHandler: {
		noCapacity:    serverpackets.SystemMessageCannotUseSoulshots,
		gradeMismatch: serverpackets.SystemMessageSoulshotsGradeMismatch,
		notEnough:     serverpackets.SystemMessageNotEnoughSoulshots,
		enabled:       serverpackets.SystemMessageEnabledSoulshot,
	},
	itemhandler.SpiritShotsHandler: {
		noCapacity:    serverpackets.SystemMessageCannotUseSpiritshots,
		gradeMismatch: serverpackets.SystemMessageSpiritshotsGradeMismatch,
		notEnough:     serverpackets.SystemMessageNotEnoughSpiritshots,
		enabled:       serverpackets.SystemMessageEnabledSpiritshot,
	},
	itemhandler.BlessedSpiritShotsHandler: {
		noCapacity:    serverpackets.SystemMessageCannotUseSpiritshots,
		gradeMismatch: serverpackets.SystemMessageSpiritshotsGradeMismatch,
		notEnough:     serverpackets.SystemMessageNotEnoughSpiritshots,
		enabled:       serverpackets.SystemMessageEnabledSpiritshot,
	},
}

// useShotItem charges the player's active weapon with a soulshot or
// spiritshot used directly from the item window: the same ChargedShot
// state the background AutoSoulShot path drives at attack time, so direct
// and auto use stay consistent. Which shot kind to charge, the capacity/
// grade/already-charged decision, and the item consumption are
// itemhandler.UseShot's job; this method only builds and sends the
// packets its result produces. It reports whether inst was handled by
// this path.
func (l *GameClientLink) useShotItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.Kind != item.KindEtcItem || tmpl.EtcItem == nil {
		return false
	}
	msgs, ok := shotMessages[tmpl.EtcItem.Handler]
	if !ok {
		return false
	}

	res := itemhandler.UseShot(itemhandler.ShotUseRequest{
		Caster:    live.Character,
		Inventory: inv,
		Item:      inst,
		Template:  tmpl,
		Destroyer: l.inventory,
	})

	switch res.Outcome {
	case itemhandler.ShotAlreadyCharged:
		// The reference treats this as a pure no-op, not a rejection of
		// something that changed: fully silent, no message, no
		// ActionFailed.
	case itemhandler.ShotNoCapacity:
		l.replyShotRejection(live, res.AutoEnabled, msgs.noCapacity)
	case itemhandler.ShotGradeMismatch:
		l.replyShotRejection(live, res.AutoEnabled, msgs.gradeMismatch)
	case itemhandler.ShotNotEnoughItems:
		if res.AutoEnabled {
			l.disableAutoShot(live, tmpl.ID)
		}
		l.replyShotRejection(live, res.AutoEnabled, msgs.notEnough)
	case itemhandler.ShotApplied:
		live.SendFrame(serverpackets.FrameSystemMessage(msgs.enabled))
		if res.SkillID != 0 {
			self := skillCastObject(live)
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameMagicSkillUse(self, self, res.SkillID, 1, 0, 0, false)
			})
		}
	}
	return true
}

// disableAutoShot turns off itemID's auto-shot flag and notifies the
// client, matching Player.disableAutoShot's active-list branch
// (Player.java:5106-5117: removeAutoSoulShot, ExAutoSoulShot(itemId, 0),
// then AUTO_USE_OF_S1_CANCELLED). Called when a direct-use shot charge
// hits ShotNotEnoughItems while its item is auto-enabled, so a depleted
// stack turns auto-shot off instead of leaving the client's auto icon lit
// against a caster that can no longer charge (SoulShots.java:52-54,
// SpiritShots.java:46-48, BlessedSpiritShots.java:48-50).
func (l *GameClientLink) disableAutoShot(live *livePlayer, itemID int32) {
	live.SetAutoSoulShot(itemID, false)
	live.SendFrame(serverpackets.FrameExAutoSoulShot(itemID, false))
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageAutoUseOfItemCancelled, itemID))
}

// replyShotRejection answers a shot-charge rejection: msg unless
// autoEnabled suppresses it (matching the reference's own suppression for
// an AutoSoulShot-enabled item), and always ActionFailed so the client's
// pending click resolves to something, per this codebase's
// no-silent-rejection rule.
func (l *GameClientLink) replyShotRejection(live *livePlayer, autoEnabled bool, msg int) {
	if !autoEnabled {
		live.SendFrame(serverpackets.FrameSystemMessage(msg))
	}
	live.SendFrame(serverpackets.FrameActionFailed())
}

// beastShotNotEnoughMessage maps each beast shot handler name to the
// client message shown when the caster lacks enough of the item to charge
// the summon's per-hit count; soulshot and spiritshot use distinct ids.
var beastShotNotEnoughMessage = map[string]int{
	itemhandler.BeastSoulShotsHandler:   serverpackets.SystemMessageNotEnoughSoulshotsForPet,
	itemhandler.BeastSpiritShotsHandler: serverpackets.SystemMessageNotEnoughSpiritshotsForPet,
}

// useBeastShotItem charges live's active pet or servitor with a beast
// soulshot or spiritshot used directly from the item window. It reports
// whether inst was handled by this path.
func (l *GameClientLink) useBeastShotItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.Kind != item.KindEtcItem || tmpl.EtcItem == nil {
		return false
	}
	notEnoughMsg, ok := beastShotNotEnoughMessage[tmpl.EtcItem.Handler]
	if !ok {
		return false
	}

	summon, summonTarget := l.activeBeastShotSummon(live)

	res := itemhandler.UseBeastShot(itemhandler.BeastShotUseRequest{
		Caster:    live.Character,
		Summon:    summon,
		Inventory: inv,
		Item:      inst,
		Template:  tmpl,
		Destroyer: l.inventory,
	})

	switch res.Outcome {
	case itemhandler.BeastShotAlreadyCharged:
		// The reference treats this as a pure no-op: fully silent, no
		// message, no ActionFailed.
	case itemhandler.BeastShotCallerIsSummon:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetCannotUseItem))
	case itemhandler.BeastShotNoSummon:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePetsNotAvailableAtThisTime))
	case itemhandler.BeastShotSummonDead:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageShotsNotAvailableForDeadPet))
	case itemhandler.BeastShotNotEnoughItems:
		l.replyShotRejection(live, res.AutoEnabled, notEnoughMsg)
	case itemhandler.BeastShotApplied:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessagePetUsesS1, tmpl.ID))
		if res.SkillID != 0 && summonTarget != nil {
			self := skillCastObject(summonTarget)
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameMagicSkillUse(self, self, res.SkillID, 1, 0, 0, false)
			})
		}
	}
	return true
}

// activeBeastShotSummon returns live's active pet or servitor as both an
// itemhandler.BeastShotCharger (for UseBeastShot) and an actorcast.Target
// (for the visual charge-skill broadcast), or nil, nil if it has none.
func (l *GameClientLink) activeBeastShotSummon(live *livePlayer) (itemhandler.BeastShotCharger, actorcast.Target) {
	if l.world == nil || live == nil {
		return nil, nil
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return nil, nil
	}
	charger, _ := obj.(itemhandler.BeastShotCharger)
	target, _ := obj.(actorcast.Target)
	return charger, target
}
