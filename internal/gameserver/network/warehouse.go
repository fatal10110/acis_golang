package network

import (
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
	items := inv.PackageSendableItems()
	frame, err := serverpackets.FramePackageSendableList(objectID, int32(inv.Adena()), items, inv.Templates())
	if err != nil {
		l.log.Error().Err(err).Msg("build PackageSendableList")
		return
	}
	live.SendFrame(frame)
}
