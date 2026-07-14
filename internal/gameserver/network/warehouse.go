package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func (l *GameClientLink) sendPackageSendableItemList(live *livePlayer, objectID int32) {
	if live == nil {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}
	items := packageSendableItems(inv, inv.Templates())
	frame, err := serverpackets.FramePackageSendableList(objectID, int32(inv.Adena()), items, inv.Templates())
	if err != nil {
		l.log.Error().Err(err).Msg("build PackageSendableList")
		return
	}
	live.SendFrame(frame)
}

func packageSendableItems(inv *itemcontainer.Inventory, templates *item.Table) []*item.Instance {
	items := inv.Items()
	out := make([]*item.Instance, 0, len(items))
	for _, inst := range items {
		if inst == nil || inst.Location != item.LocationInventory {
			continue
		}
		tmpl, ok := templates.Get(inst.TemplateID)
		if !ok || inst.QuestItem(tmpl) || !inst.Tradable(tmpl) {
			continue
		}
		out = append(out, inst)
	}
	return out
}
