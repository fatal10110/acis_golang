package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

func testInventory(ownerID, itemID int32, count int) *itemcontainer.Inventory {
	templates := item.NewTable([]*item.Template{
		{ID: itemID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := itemcontainer.NewPlayerInventory(ownerID, templates)
	inv.AddNew(itemID, count, 100)
	return inv
}
