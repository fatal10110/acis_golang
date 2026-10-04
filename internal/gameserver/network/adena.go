package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// reduceAdena takes count adena from live with its notices: not enough adena
// when live holds less, else the amount spent. A count of 0 or less takes
// nothing and says nothing.
func reduceAdena(live *livePlayer, count int) bool {
	inv := live.Inventory()
	if inv == nil || count > inv.Adena() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		return false
	}
	if count <= 0 {
		return true
	}
	if inv.DestroyByTemplateID(item.AdenaID, count) == nil {
		return false
	}
	live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DisappearedAdena, int32(count)))
	return true
}
