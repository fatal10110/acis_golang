package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// orphanID is an item id the orphan table has no template for: what a
// reload that dropped a held item's template leaves behind.
const orphanID int32 = 65000

// orphanTable holds the templates of the items every list below carries
// besides the orphan: adena and a two-handed sword.
func orphanTable() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Stackable: true, Tradable: true, Duration: -1, ReferencePrice: 1, EtcItem: &item.EtcItemDetail{}},
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: -1, ReferencePrice: 300, Weapon: &item.WeaponDetail{}},
	})
}

// TestListPacketsSkipItemsWithNoTemplate builds every item list packet
// twice, once with an orphan row in front of, between and behind the rows
// whose templates are loaded and once without it: the two frames must be
// byte-identical, so the orphan writes nothing and the count the packet
// carries is the rows it actually wrote.
func TestListPacketsSkipItemsWithNoTemplate(t *testing.T) {
	templates := orphanTable()
	inst := func(objectID, templateID int32, loc item.Location) *item.Instance {
		return &item.Instance{ObjectID: objectID, TemplateID: templateID, Count: 1, Location: loc, ManaLeft: -1}
	}
	carried := func(withOrphan bool) []*item.Instance {
		items := []*item.Instance{inst(1, item.AdenaID, item.LocationInventory), inst(2, 2368, item.LocationPaperdoll)}
		if withOrphan {
			items = []*item.Instance{inst(9, orphanID, item.LocationInventory), items[0], inst(10, orphanID, item.LocationPaperdoll), items[1], inst(11, orphanID, item.LocationInventory)}
		}
		return items
	}
	updates := func(withOrphan bool) []itemcontainer.Update {
		var out []itemcontainer.Update
		for _, it := range carried(withOrphan) {
			out = append(out, itemcontainer.NewUpdate(it.Snapshot(), itemcontainer.UpdateModified))
		}
		return out
	}
	products := func(withOrphan bool) buylist.List {
		list := buylist.List{ID: 7, Products: []buylist.Product{{ItemID: item.AdenaID, Price: 10, MaxCount: -1}, {ItemID: 2368, Price: 20, MaxCount: -1}}}
		if withOrphan {
			list.Products = append([]buylist.Product{{ItemID: orphanID, Price: 5, MaxCount: -1}}, list.Products...)
		}
		return list
	}
	stock := func(buylist.Product) int { return 1 }
	states := func(withOrphan bool) []item.InstanceState {
		var out []item.InstanceState
		for _, it := range carried(withOrphan) {
			out = append(out, it.Snapshot())
		}
		return out
	}

	builders := map[string]func(withOrphan bool) (wire.Frame, error){
		"ItemList": func(o bool) (wire.Frame, error) { return FrameItemList(carried(o), templates, true) },
		"GMViewItemList": func(o bool) (wire.Frame, error) {
			return FrameGMViewItemList("Owner", 80, carried(o), templates)
		},
		"InventoryUpdate":    func(o bool) (wire.Frame, error) { return FrameInventoryUpdate(updates(o), templates) },
		"PetItemList":        func(o bool) (wire.Frame, error) { return FramePetItemList(carried(o), templates) },
		"PetInventoryUpdate": func(o bool) (wire.Frame, error) { return FramePetInventoryUpdate(updates(o), templates) },
		"PackageSendableList": func(o bool) (wire.Frame, error) {
			return FramePackageSendableList(5, 100, carried(o), templates)
		},
		"WarehouseDepositList": func(o bool) (wire.Frame, error) {
			return FrameWarehouseDepositList(WarehousePrivate, 100, carried(o), templates)
		},
		"WarehouseWithdrawList": func(o bool) (wire.Frame, error) {
			return FrameWarehouseWithdrawList(WarehouseClan, 100, carried(o), templates)
		},
		"BuyList": func(o bool) (wire.Frame, error) {
			return FrameBuyList(products(o), stock, 100, 0, 1, templates)
		},
		"ShopPreviewList": func(o bool) (wire.Frame, error) {
			return FrameShopPreviewList(products(o), 100, 5, 10, templates)
		},
		"SellList":   func(o bool) (wire.Frame, error) { return FrameSellList(100, carried(o), templates) },
		"TradeStart": func(o bool) (wire.Frame, error) { return FrameTradeStart(5, carried(o), templates) },
		"TradeItemUpdate": func(o bool) (wire.Frame, error) {
			var entries []TradeItemUpdateEntry
			for _, st := range states(o) {
				entries = append(entries, TradeItemUpdateEntry{Item: TradeItemSnapshot{ObjectID: st.ObjectID, TemplateID: st.TemplateID, Count: st.Count}, AvailableCount: st.Count})
			}
			return FrameTradeItemUpdate(entries, templates)
		},
		"SellListProcure": func(o bool) (wire.Frame, error) {
			var rows []ProcureSale
			for _, it := range carried(o) {
				rows = append(rows, ProcureSale{Item: it, Amount: 3})
			}
			return FrameSellListProcure(100, rows, templates)
		},
		"PrivateStoreManageListSell": func(o bool) (wire.Frame, error) {
			var candidates []privatestore.SellCandidate
			var listed []privatestore.SellItem
			for _, st := range states(o) {
				candidates = append(candidates, privatestore.SellCandidate{Item: st, Count: st.Count})
				listed = append(listed, privatestore.SellItem{ObjectID: st.ObjectID, TemplateID: st.TemplateID, Count: 1, Price: 7})
			}
			return FramePrivateStoreManageListSell(5, false, 100, candidates, listed, templates)
		},
		"PrivateStoreListSell": func(o bool) (wire.Frame, error) {
			var listed []privatestore.SellItem
			for _, st := range states(o) {
				listed = append(listed, privatestore.SellItem{ObjectID: st.ObjectID, TemplateID: st.TemplateID, Count: 1, Price: 7})
			}
			return FramePrivateStoreListSell(5, true, 100, listed, templates)
		},
		"PrivateStoreManageListBuy": func(o bool) (wire.Frame, error) {
			var listed []privatestore.BuyItem
			for _, st := range states(o) {
				listed = append(listed, privatestore.BuyItem{TemplateID: st.TemplateID, Quantity: 2, Price: 7})
			}
			return FramePrivateStoreManageListBuy(5, 100, states(o), listed, templates)
		},
		"PrivateStoreListBuy": func(o bool) (wire.Frame, error) {
			var offers []privatestore.BuyOffer
			for _, st := range states(o) {
				offers = append(offers, privatestore.BuyOffer{BuyItem: privatestore.BuyItem{TemplateID: st.TemplateID, Quantity: 2, Price: 7}, ObjectID: st.ObjectID, Count: 1})
			}
			return FramePrivateStoreListBuy(5, 100, offers, templates)
		},
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			without := builtPayload(t, build, false)
			with := builtPayload(t, build, true)
			if !bytes.Equal(with, without) {
				t.Fatalf("%s with orphan rows = %x\nwant the frame without them %x", name, with, without)
			}
			if bytes.Contains(with, []byte{0xe8, 0xfd, 0x00, 0x00}) { // orphanID, little-endian
				t.Fatalf("%s carries the orphan id: %x", name, with)
			}
			// adena (57) or the sword (2368): the loaded rows are still written.
			if !bytes.Contains(without, []byte{0x39, 0x00, 0x00, 0x00}) && !bytes.Contains(without, []byte{0x40, 0x09, 0x00, 0x00}) {
				t.Fatalf("%s wrote no loaded row: %x", name, without)
			}
		})
	}
}

func builtPayload(t *testing.T, build func(bool) (wire.Frame, error), withOrphan bool) []byte {
	t.Helper()
	frame, err := build(withOrphan)
	if err != nil {
		t.Fatalf("build (orphan %v): %v", withOrphan, err)
	}
	return bytes.Clone(framePayload(t, frame))
}
