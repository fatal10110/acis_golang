package network

import (
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// useTargetCastItem runs a chest key, soul crystal or beast spice: the
// item's own check of the selected target, then its skills cast on that
// target as ordinary skill requests. The item is not spent as the item used;
// a key's or spice's skill spends it through its own consume item once the
// cast starts, and a crystal is never spent. It reports whether inst is one
// of these items.
func (l *GameClientLink) useTargetCastItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, ctrl bool) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok {
		return false
	}
	decision := itemhandler.ResolveTargetCast(tmpl, live.Character, l.skills)
	if !decision.Handled {
		return false
	}
	switch decision.Refusal {
	case itemhandler.TargetCastAllowed:
		l.castItemSkills(live, inv, nil, decision.Skills, ctrl && decision.CtrlForces, false)
	case itemhandler.TargetCastSitting:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotMoveWhileSitting))
	case itemhandler.TargetCastInvalidTarget:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
	case itemhandler.TargetCastNoSkills:
		// Only logged, as the specified handler does; a use-item request
		// leaves no client action pending.
		l.log.Warn().Int32("item", inst.TemplateID).Msg("key has no registered skill")
	case itemhandler.TargetCastSilent:
		// An immobile key user, and a crystal or spice whose skill does not
		// resolve, are refused with no packet, as specified; a use-item
		// request leaves no client action pending.
	}
	return true
}
