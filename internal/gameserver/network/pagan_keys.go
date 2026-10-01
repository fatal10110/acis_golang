package network

import (
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// usePaganKey answers UseItem on a Pagan Temple key: it opens the selected
// door directly, with no cast. A target that is not a door, or a door out of
// interaction range, is refused with its message and ActionFailed and the key
// is kept. Otherwise one key is spent before the door is matched, so a key
// used on a door it does not fit is still gone, answered by S1_CANNOT_BE_USED
// naming it. It reports whether inst is a Pagan Temple key.
func (l *GameClientLink) usePaganKey(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template) bool {
	if tmpl.EtcItem == nil || tmpl.EtcItem.Handler != itemhandler.PaganKeysHandler {
		return false
	}
	gate, ok := live.Target().(*door.Object)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	if !interactInRange(live, gate, interactionDistance) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageDistTooFarCastingStopped))
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	if _, ok := l.inventory.DestroyItem(inv, inst.ObjectID, 1); !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		return true
	}
	sendDestroyedMessage(live, inst.TemplateID, 1)
	doors, wrongDoor := itemhandler.PaganKeyDoors(inst.TemplateID, gate.DoorID())
	if wrongDoor {
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1CannotBeUsed, inst.TemplateID))
		return true
	}
	if l.doors == nil {
		return true
	}
	for _, id := range doors {
		l.doors.SetDoorOpen(id, true)
	}
	return true
}
