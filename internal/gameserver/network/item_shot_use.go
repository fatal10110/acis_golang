package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Handler names for the direct-from-window shot etc items this path
// covers. Beast (summon-charged) variants and fishing shots are not
// handled here: they need charged-shot state on the summon actor and on
// the (currently unmodeled) fishing state respectively, neither of which
// exists yet.
const (
	soulShotsHandler          = "SoulShots"
	spiritShotsHandler        = "SpiritShots"
	blessedSpiritShotsHandler = "BlessedSpiritShots"
)

// useShotItem charges the player's active weapon with a soulshot or
// spiritshot used directly from the item window: the same ChargedShot
// state the background AutoSoulShot path drives at attack time, so direct
// and auto use stay consistent. It reports whether inst was handled by
// this path.
func (l *GameClientLink) useShotItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.Kind != item.KindEtcItem || tmpl.EtcItem == nil {
		return false
	}

	switch tmpl.EtcItem.Handler {
	case soulShotsHandler:
		consume, result := live.ChargeSoulshot(tmpl.Crystal, rnd.Get(100))
		l.replyShotCharge(live, inv, inst, tmpl, result, consume,
			serverpackets.SystemMessageCannotUseSoulshots,
			serverpackets.SystemMessageSoulshotsGradeMismatch,
			serverpackets.SystemMessageNotEnoughSoulshots,
			serverpackets.SystemMessageEnabledSoulshot,
		)
		return true
	case spiritShotsHandler:
		consume, result := live.ChargeSpiritshot(item.ShotSpirit, tmpl.Crystal)
		l.replyShotCharge(live, inv, inst, tmpl, result, consume,
			serverpackets.SystemMessageCannotUseSpiritshots,
			serverpackets.SystemMessageSpiritshotsGradeMismatch,
			serverpackets.SystemMessageNotEnoughSpiritshots,
			serverpackets.SystemMessageEnabledSpiritshot,
		)
		return true
	case blessedSpiritShotsHandler:
		consume, result := live.ChargeSpiritshot(item.ShotBlessedSpirit, tmpl.Crystal)
		l.replyShotCharge(live, inv, inst, tmpl, result, consume,
			serverpackets.SystemMessageCannotUseSpiritshots,
			serverpackets.SystemMessageSpiritshotsGradeMismatch,
			serverpackets.SystemMessageNotEnoughSpiritshots,
			serverpackets.SystemMessageEnabledSpiritshot,
		)
		return true
	}
	return false
}

// replyShotCharge maps one ChargeSoulshot/ChargeSpiritshot outcome to
// client packets. A rejection message is suppressed when the item is
// enabled for AutoSoulShot, matching the reference's own message
// suppression for that case — but this path still answers ActionFailed
// even then, so the client's pending click always resolves to something,
// per this codebase's no-silent-rejection rule. An already-charged
// result stays fully silent (no message, no ActionFailed): the reference
// treats it as a pure no-op, not a rejection of something that changed.
func (l *GameClientLink) replyShotCharge(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template, result player.ChargeShotResult, consume int32, noCapacityMsg, gradeMismatchMsg, notEnoughMsg, enabledMsg int) {
	autoEnabled := live.AutoSoulShotEnabled(tmpl.ID)

	switch result {
	case player.ChargeShotAlreadyCharged:
		return
	case player.ChargeShotNoCapacity:
		if !autoEnabled {
			live.SendFrame(serverpackets.FrameSystemMessage(noCapacityMsg))
		}
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	case player.ChargeShotGradeMismatch:
		if !autoEnabled {
			live.SendFrame(serverpackets.FrameSystemMessage(gradeMismatchMsg))
		}
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}

	if _, ok := l.inventoryService().DestroyItem(inv, inst.ObjectID, int(consume)); !ok {
		if !autoEnabled {
			live.SendFrame(serverpackets.FrameSystemMessage(notEnoughMsg))
		}
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.sendInventoryUpdate(live, inv)

	live.SendFrame(serverpackets.FrameSystemMessage(enabledMsg))
	if len(tmpl.AttachedSkills) > 0 {
		self := skillCastObject(live)
		skillID := int32(tmpl.AttachedSkills[0].ID)
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameMagicSkillUse(self, self, skillID, 1, 0, 0, false)
		})
	}
}
