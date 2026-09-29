package network

import (
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// festivalResurrectionRefusal is the text a resurrection scroll answers
// for a dead festival participant.
const festivalResurrectionRefusal = "You may not resurrect participants in a festival."

// useResurrectionScroll runs a resurrection scroll: its own checks on the
// selected creature, then each of its skills cast on it as an ordinary
// skill request with no modifiers. The scroll is not consumed as the item
// used; each skill's own consume item pays for it once the cast starts. A
// refused target is answered by its reason alone. It reports whether inst
// is a resurrection scroll.
func (l *GameClientLink) useResurrectionScroll(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok {
		return false
	}
	defs, ok := itemhandler.ResolveResurrectionScrollSkills(tmpl, l.skills)
	if !ok {
		return false
	}
	if refusal := itemhandler.ResurrectionScrollGate(live.ObjectID(), live.Target()); refusal != itemhandler.ResurrectionScrollAllowed {
		sendResurrectionScrollRefusal(live, refusal)
		return true
	}
	if len(defs) == 0 {
		l.log.Warn().Int32("item", inst.TemplateID).Msg("resurrection scroll has no registered skill")
		sendMagicActionFailed(live)
		return true
	}
	l.castItemSkills(live, inv, nil, defs, false, false)
	return true
}

func sendResurrectionScrollRefusal(live *livePlayer, refusal itemhandler.ResurrectionScrollRefusal) {
	switch refusal {
	case itemhandler.ResurrectionScrollInvalidTarget:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
	case itemhandler.ResurrectionScrollSiege:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotBeResurrectedDuringSiege))
	case itemhandler.ResurrectionScrollFestival:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, festivalResurrectionRefusal))
	case itemhandler.ResurrectionScrollPetOfferOpen:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotResMaster))
	case itemhandler.ResurrectionScrollAlreadyProposed:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageResHasAlreadyBeenProposed))
	case itemhandler.ResurrectionScrollOwnerOfferOpen:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotResPet2))
	}
}
