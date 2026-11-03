package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// reduceAdena takes count adena from live with its notices: not enough adena
// when live holds less, else the amount spent. A count of 0 or less takes
// nothing and says nothing.
func reduceAdena(live *livePlayer, count int) bool {
	_, ok := spendAdena(live, count)
	return ok
}

// spendAdena is reduceAdena that also returns the adena stack the count
// came out of, nil when it took nothing.
func spendAdena(live *livePlayer, count int) (*item.Instance, bool) {
	inv := live.Inventory()
	if inv == nil || count > inv.Adena() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		return nil, false
	}
	if count <= 0 {
		return nil, true
	}
	paid := inv.DestroyByTemplateID(item.AdenaID, count)
	if paid == nil {
		return nil, false
	}
	live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DisappearedAdena, int32(count)))
	return paid, true
}

// takeAdena takes count adena from inv without a notice, reporting false,
// taking none, when inv holds less. A count of 0 or less takes nothing.
func takeAdena(inv *itemcontainer.Inventory, count int) bool {
	if count > inv.Adena() {
		return false
	}
	return count <= 0 || inv.DestroyByTemplateID(item.AdenaID, count) != nil
}
