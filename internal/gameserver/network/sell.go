package network

import (
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// merchantListOffset is added to an npc id to make the list id a buy or
// sell request names its merchant by.
const merchantListOffset = 1000000

// sendSellList answers a merchant's Sell command: the sell window on live's
// sellable items and current adena, or, with nothing to sell, emptyPage when
// the merchant has one.
func (l *GameClientLink) sendSellList(live *livePlayer, f *npc.Folk, emptyPage string) {
	inv := live.Inventory()
	items := l.inventory.SellableItems(inv, func(objectID int32) bool {
		// The window leaves out a summoned pet's collar only; a ridden
		// mount's collar is listed and its sale refused.
		return objectID != live.MountObjectID() && live.ControlItemInUse(objectID)
	})
	if len(items) == 0 && emptyPage != "" {
		sendFilledHTML(live, f.ObjectID(), emptyPage, 0)
		return
	}
	adena := 0
	if inv != nil {
		adena = inv.Adena()
	}
	frame, err := serverpackets.FrameSellList(adena, items, l.itemTemplates)
	if err != nil {
		l.log.Error().Err(err).Msg("build sell list")
		return
	}
	live.SendFrame(frame)
}

// requestSellItem sells the requested rows to the merchant live targets and
// pays for them in adena, then shows the merchant's sold page when it has
// one.
//
// Every refusal is silent, the sold rows' InventoryUpdate being the only
// answer a sale owes: the target is not a merchant or out of reach, the
// list id names another merchant's list, or the rows would pay more than
// the largest int32. The client closes its sell window itself when it sends
// the request, so no click is left pending; rows naming items that cannot
// be sold are skipped the same way.
func (l *GameClientLink) requestSellItem(live *livePlayer, req clientpackets.RequestSellItem) {
	if live == nil {
		return
	}
	f, ok := live.Target().(*npc.Folk)
	if !ok || !f.BuysItems() || !l.playerCanDoInteract(live, f) {
		return
	}
	if req.ListID > merchantListOffset && int(req.ListID-merchantListOffset) != f.NpcID() {
		return
	}
	rows := make([]invops.SaleRow, len(req.Items))
	for i, row := range req.Items {
		rows[i] = invops.SaleRow{ObjectID: row.ObjectID, Count: int(row.Count)}
	}
	inv := live.Inventory()
	enchantScroll := l.enchantStateStore().Active(live.ObjectID())
	res, sold, err := l.inventory.Sell(inv, rows, func(objectID int32) bool {
		return objectID == enchantScroll || live.ControlItemInUse(objectID)
	})
	if err != nil {
		l.log.Error().Err(err).Msg("sell items")
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if !sold {
		return
	}
	if len(res.Changed) > 0 {
		l.applyEquipStatChanges(live, inv, res.Result)
	}
	if page, ok := f.SoldPage(setPages{l.html}); ok {
		sendFilledHTML(live, f.ObjectID(), page, 0)
	}
}
