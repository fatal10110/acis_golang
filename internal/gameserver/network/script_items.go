package network

import (
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// sendTakenMessage names the items a script took from live: adena by its
// amount, anything else the way a destroy names it. A take that found too
// few units names the shortage instead.
func sendTakenMessage(live *livePlayer, e event.ItemsTaken) {
	switch {
	case e.Short && e.ItemID == item.AdenaID:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
	case e.Short:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
	case e.ItemID == item.AdenaID:
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DisappearedAdena, int32(e.Count)))
	default:
		sendDestroyedMessage(live, e.ItemID, e.Count)
	}
}

// unequipTakenItem takes the worn item objectID off live before a script
// takes it: the slots it held are cleared with no chat line, the
// equipment's effects follow, and live's look is resent to it and to the
// players who see it.
func (l *GameClientLink) unequipTakenItem(live *livePlayer, objectID int32) {
	inv := live.Inventory()
	if inv == nil {
		return
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil || !inst.Equipped() {
		return
	}
	if changed := inv.UnequipItem(inst); len(changed) > 0 {
		l.applyEquipStatChanges(live, inv, invops.Result{EquipmentChanged: true, Changed: changed})
	}
	l.broadcastCharacterInfo(live)
}
