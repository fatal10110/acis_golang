package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
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
		Destroyer: l.inventoryService(),
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
		l.replyShotRejection(live, res.AutoEnabled, msgs.notEnough)
	case itemhandler.ShotApplied:
		l.sendInventoryUpdate(live, inv)
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
