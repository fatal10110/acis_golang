package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// oracleShopCases are the merchant, trade, private store, recipe shop, macro
// and symbol packets (testdata/oracle/ProbeShops.java). The window contents a
// reference packet derives from the inventory (what can be traded, which
// symbols a class may draw) are decided before the encoder here, so each case
// hands the encoder the rows the probe's stand-ins hand the reference.
func oracleShopCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		addMerchantCases(m, r, v, n)
		addTradeCases(m, v, n)
		addPrivateStoreCases(m, r, v, n)
		addRecipeCases(m, r, v, n)
		addMacroCases(m, r, v, n)
		addHennaCases(m, v, n)
	}
	return m
}

func addMerchantCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	table := oracleItemTable(v)
	list := buylist.List{ID: int(rowField(v, 1))}
	stock := map[int32]int{}
	for kind := oracleWeapon; kind <= oracleSiege; kind++ {
		id := [...]int32{2368, 1146, 112, 1000, item.AdenaID, 4000}[kind]
		p := buylist.Product{ItemID: id, Price: int(rowField(v, kind+3)), MaxCount: -1}
		switch kind % 3 {
		case 1:
			p.MaxCount = 5
			stock[id] = int(rowField(v, kind+4))
		case 2:
			p.MaxCount = 9
			stock[id] = 3
		}
		list.Products = append(list.Products, p)
	}
	count := func(p buylist.Product) int { return stock[p.ItemID] }
	tax, siege := oracleDoubles[r%len(oracleDoubles)], oracleDoubles[(r+2)%len(oracleDoubles)]
	m["BuyList"+n] = func() wire.Frame {
		return mustFrame(FrameBuyList(list, count, int(rowField(v, 0)), tax, siege, table))
	}
	m["SellList"+n] = func() wire.Frame {
		return mustFrame(FrameSellList(int(rowField(v, 2)), oracleItems(v), table))
	}
	for _, expertise := range []int{0, 2, 5} {
		name := "ShopPreviewList/exp" + string(rune('0'+expertise)) + n
		m[name] = func() wire.Frame {
			return mustFrame(FrameShopPreviewList(list, int(rowField(v, 0)), expertise, int(rowField(v, 6)), table))
		}
	}
	m["ShopPreviewInfo"+n] = func() wire.Frame {
		var worn [item.PaperdollSlots]int32
		for slot := range worn {
			worn[slot] = rowField(v, slot)
		}
		return FrameShopPreviewInfo(worn)
	}

	m["MultiSellList/first"+n] = func() wire.Frame { return FrameMultiSellList(oracleMultiSell(v, table), 0) }
	m["MultiSellList/last"+n] = func() wire.Frame { return FrameMultiSellList(oracleMultiSell(v, table), 40) }
}

// oracleMultiSell is ProbeShops.multiSell: 45 entries, so two pages.
func oracleMultiSell(v [10]int32, table *item.Table) *multisell.List {
	ingredient := func(kind int, count, enchant int32) multisell.Ingredient {
		id := [...]int32{2368, 1146, 112, 1000, item.AdenaID, 4000}[kind]
		ing := multisell.NewIngredient(id, int(count), false, false, table)
		ing.EnchantLevel = int(enchant)
		return ing
	}
	unknown := func(count int32) multisell.Ingredient {
		return multisell.NewIngredient(4242, int(count), false, false, table)
	}
	list := &multisell.List{ID: rowField(v, 0)}
	for j := range 45 {
		products := []multisell.Ingredient{ingredient(j%6, rowField(v, j), rowField(v, j+1))}
		if j%3 == 0 {
			products = append(products, unknown(rowField(v, j+2)))
		}
		ingredients := []multisell.Ingredient{
			ingredient(oracleAdena, rowField(v, j+3), 0),
			ingredient((j+1)%6, rowField(v, j+4), rowField(v, j+5)),
		}
		if j%4 == 0 {
			ingredients = append(ingredients, unknown(rowField(v, j+6)))
		}
		list.Entries = append(list.Entries, multisell.NewEntry(ingredients, products))
	}
	return list
}

func tradeSnapshot(v [10]int32, kind int, count int32) TradeItemSnapshot {
	inst := oracleItem(v, kind, false, false)
	return TradeItemSnapshot{ObjectID: inst.ObjectID, TemplateID: inst.TemplateID, Count: int(count), EnchantLevel: inst.EnchantLevel}
}

func addTradeCases(m map[string]func() wire.Frame, v [10]int32, n string) {
	table := oracleItemTable(v)
	m["TradeStart"+n] = func() wire.Frame {
		return mustFrame(FrameTradeStart(rowField(v, 3), oracleItems(v), table))
	}
	qty := int(rowField(v, 3))
	for _, c := range []struct {
		name string
		kind int
	}{{"/single", oracleWeapon}, {"/stackable", oracleAdena}} {
		snap := tradeSnapshot(v, c.kind, rowField(v, c.kind+1))
		m["TradeOwnAdd"+c.name+n] = func() wire.Frame { return mustFrame(FrameTradeOwnAdd(snap, qty, table)) }
		m["TradeOtherAdd"+c.name+n] = func() wire.Frame { return mustFrame(FrameTradeOtherAdd(snap, qty, table)) }
		m["TradeUpdate"+c.name+n] = func() wire.Frame { return mustFrame(FrameTradeUpdate(snap, qty, table)) }
		m["TradeUpdate"+c.name+"/zero"+n] = func() wire.Frame { return mustFrame(FrameTradeUpdate(snap, 0, table)) }
	}
	m["TradeItemUpdate"+n] = func() wire.Frame {
		inv := oracleItems(v)
		offered := []struct {
			kind  int
			count int32
		}{{oracleWeapon, int32(inv[oracleWeapon].Count)}, {oracleQuest, rowField(v, 5)}, {oracleAdena, rowField(v, 6)}}
		entries := make([]TradeItemUpdateEntry, len(offered))
		for i, o := range offered {
			snap := tradeSnapshot(v, o.kind, o.count)
			// The first inventory item with the object id, less what is offered.
			var have int32
			for _, it := range inv {
				if it.ObjectID == snap.ObjectID {
					have = int32(it.Count)
					break
				}
			}
			entries[i] = TradeItemUpdateEntry{Item: snap, AvailableCount: int(have - o.count)}
		}
		return mustFrame(FrameTradeItemUpdate(entries, table))
	}
}

func sellRow(v [10]int32, kind int, count, price int32) privatestore.SellItem {
	inst := oracleItem(v, kind, false, false)
	return privatestore.SellItem{
		ObjectID: inst.ObjectID, TemplateID: inst.TemplateID, Enchant: inst.EnchantLevel,
		Count: int(count), Quantity: int(count), Price: int(price),
	}
}

func addPrivateStoreCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	table := oracleItemTable(v)
	owner := rowField(v, 0)
	listed := []privatestore.SellItem{
		sellRow(v, oracleWeapon, rowField(v, 1), rowField(v, 3)),
		sellRow(v, oracleQuest, rowField(v, 2), rowField(v, 4)),
	}
	for _, packaged := range []bool{true, false} {
		p := "/plain"
		if packaged {
			p = "/package"
		}
		candidates := []privatestore.SellCandidate{
			{Item: oracleItem(v, oracleArmor, false, false).Snapshot(), Count: int(rowField(v, 4))},
			{Item: oracleItem(v, oracleJewel, false, false).Snapshot(), Count: int(rowField(v, 5))},
		}
		m["PrivateStoreListSell"+p+n] = func() wire.Frame {
			return mustFrame(FramePrivateStoreListSell(owner, packaged, int(rowField(v, 2)), listed, table))
		}
		m["PrivateStoreManageListSell"+p+n] = func() wire.Frame {
			return mustFrame(FramePrivateStoreManageListSell(owner, packaged, int(rowField(v, 2)), candidates, listed, table))
		}
	}

	wanted := func(kind int, count, price, quantity int32) privatestore.BuyOffer {
		inst := oracleItem(v, kind, false, false)
		return privatestore.BuyOffer{
			BuyItem:  privatestore.BuyItem{TemplateID: inst.TemplateID, Enchant: inst.EnchantLevel, Quantity: int(quantity), Price: int(price)},
			ObjectID: inst.ObjectID, Count: int(count),
		}
	}
	offers := []privatestore.BuyOffer{
		wanted(oracleWeapon, rowField(v, 1), rowField(v, 3), rowField(v, 5)),
		wanted(oracleAdena, rowField(v, 2), rowField(v, 4), rowField(v, 6)),
	}
	m["PrivateStoreListBuy"+n] = func() wire.Frame {
		return mustFrame(FramePrivateStoreListBuy(owner, int(rowField(v, 2)), offers, table))
	}
	candidates := []item.InstanceState{
		oracleItem(v, oracleArmor, false, false).Snapshot(),
		oracleItem(v, oracleJewel, false, false).Snapshot(),
	}
	wantedRows := []privatestore.BuyItem{
		{TemplateID: 2368, Enchant: int(rowField(v, 2)), Quantity: int(rowField(v, 1)), Price: int(rowField(v, 3))},
		{TemplateID: 1000, Enchant: int(rowField(v, 6)), Quantity: int(rowField(v, 4)), Price: int(rowField(v, 5))},
	}
	m["PrivateStoreManageListBuy"+n] = func() wire.Frame {
		return mustFrame(FramePrivateStoreManageListBuy(owner, int(rowField(v, 2)), candidates, wantedRows, table))
	}
	sellTitle, buyTitle := oracleStrings[r].s, oracleStrings[(r+1)%4].s
	m["PrivateStoreMsgSell"+n] = func() wire.Frame { return FramePrivateStoreMsgSell(owner, sellTitle) }
	m["PrivateStoreMsgBuy"+n] = func() wire.Frame { return FramePrivateStoreMsgBuy(owner, buyTitle) }
}

func addRecipeCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	crafter, adena, mp, maxMP := rowField(v, 0), int(rowField(v, 1)), rowField(v, 2), rowField(v, 3)
	dwarvenBook := []recipe.Recipe{{ID: int(rowField(v, 4)), Dwarven: true}, {ID: int(rowField(v, 5)), Dwarven: true}}
	commonBook := []recipe.Recipe{{ID: int(rowField(v, 6))}}
	all := []privatestore.ManufactureItem{
		{RecipeID: int(rowField(v, 4)), Cost: int(rowField(v, 7)), Dwarven: true},
		{RecipeID: int(rowField(v, 6)), Cost: int(rowField(v, 8))},
		{RecipeID: int(rowField(v, 5)), Cost: int(rowField(v, 9)), Dwarven: true},
		{RecipeID: 999, Cost: int(rowField(v, 1)), Dwarven: true},
	}
	// The window lists the crafts of its page whose recipe is in the book.
	offered := func(dwarven bool) []privatestore.ManufactureItem {
		var out []privatestore.ManufactureItem
		for _, it := range all {
			known := false
			for _, book := range [][]recipe.Recipe{dwarvenBook, commonBook} {
				for _, rec := range book {
					known = known || rec.ID == it.RecipeID
				}
			}
			if it.Dwarven == dwarven && known {
				out = append(out, it)
			}
		}
		return out
	}
	m["RecipeShopManageList/dwarven"+n] = func() wire.Frame {
		return FrameRecipeShopManageList(crafter, adena, true, dwarvenBook, offered(true))
	}
	m["RecipeShopManageList/common"+n] = func() wire.Frame {
		return FrameRecipeShopManageList(crafter, adena, false, commonBook, offered(false))
	}
	m["RecipeShopManageList/dwarven-without-skill"+n] = func() wire.Frame {
		return FrameRecipeShopManageList(crafter, adena, true, commonBook, offered(true))
	}
	m["RecipeShopSellList"+n] = func() wire.Frame {
		return FrameRecipeShopSellList(crafter, mp, maxMP, int(rowField(v, 2)), all)
	}
	m["RecipeShopItemInfo"+n] = func() wire.Frame { return FrameRecipeShopItemInfo(crafter, rowField(v, 4), mp, maxMP) }
	m["RecipeShopMsg"+n] = func() wire.Frame { return FrameRecipeShopMsg(crafter, oracleStrings[r].s) }
	m["RecipeBookItemList/dwarven"+n] = func() wire.Frame { return FrameRecipeBookItemList(true, maxMP, dwarvenBook) }
	m["RecipeBookItemList/common"+n] = func() wire.Frame { return FrameRecipeBookItemList(false, maxMP, commonBook) }
	m["RecipeItemMakeInfo/dwarven"+n] = func() wire.Frame {
		return FrameRecipeItemMakeInfo(dwarvenBook[0], mp, maxMP, -1)
	}
	m["RecipeItemMakeInfo/common"+n] = func() wire.Frame {
		return FrameRecipeItemMakeInfo(recipe.Recipe{ID: int(rowField(v, 6))}, mp, maxMP, -1)
	}
	m["RecipeItemMakeInfo/status"+n] = func() wire.Frame {
		return FrameRecipeItemMakeInfo(dwarvenBook[0], mp, maxMP, rowField(v, 7))
	}
}

func addMacroCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	full := &macro.Macro{
		ID: rowField(v, 0), Icon: rowField(v, 1),
		Name: oracleStrings[r].s, Description: oracleStrings[(r+1)%4].s, Acronym: oracleStrings[(r+2)%4].s,
		Commands: []macro.Command{
			{Type: rowField(v, 4), D1: rowField(v, 5), D2: rowField(v, 6), Text: oracleStrings[(r+3)%4].s},
			{Type: rowField(v, 7), D1: rowField(v, 8), D2: rowField(v, 9), Text: oracleStrings[r].s},
		},
	}
	empty := &macro.Macro{ID: rowField(v, 0), Icon: rowField(v, 1)}
	rev, count := rowField(v, 2), int(rowField(v, 3))
	m["SendMacroList/macro"+n] = func() wire.Frame { return FrameSendMacroList(rev, count, full) }
	m["SendMacroList/none"+n] = func() wire.Frame { return FrameSendMacroList(rev, count, nil) }
	m["SendMacroList/empty"+n] = func() wire.Frame { return FrameSendMacroList(rev, count, empty) }
}

func oracleHenna(v [10]int32, k int) henna.Henna {
	return henna.Henna{
		SymbolID: int(rowField(v, k)), DyeID: rowField(v, k+1), DrawPrice: int(rowField(v, k+2)),
		INT: int(rowField(v, k+3)), STR: int(rowField(v, k+4)), CON: int(rowField(v, k+5)),
		MEN: int(rowField(v, k+6)), DEX: int(rowField(v, k+7)), WIT: int(rowField(v, k+8)),
		Classes: []int{0},
	}
}

func addHennaCases(m map[string]func() wire.Frame, v [10]int32, n string) {
	hennas := []henna.Henna{oracleHenna(v, 0), oracleHenna(v, 1), oracleHenna(v, 2)}
	adena := int(rowField(v, 2))
	now := HennaStats{
		INT: int(rowField(v, 0)), STR: int(rowField(v, 1)), CON: int(rowField(v, 2)),
		MEN: int(rowField(v, 3)), DEX: int(rowField(v, 4)), WIT: int(rowField(v, 5)),
	}
	m["HennaEquipList"+n] = func() wire.Frame { return FrameHennaEquipList(adena, int(rowField(v, 3)), hennas) }
	m["HennaUnequipList"+n] = func() wire.Frame { return FrameHennaUnequipList(adena, int(rowField(v, 4)), hennas) }
	m["HennaItemInfo"+n] = func() wire.Frame { return FrameHennaItemInfo(hennas[0], adena, now) }
	m["HennaItemUnequipInfo"+n] = func() wire.Frame { return FrameHennaItemUnequipInfo(hennas[1], adena, now) }
	m["HennaInfo"+n] = func() wire.Frame {
		snap := henna.Snapshot{
			INT: now.INT, STR: now.STR, CON: now.CON, MEN: now.MEN, DEX: now.DEX, WIT: now.WIT,
			MaxSlots: int(rowField(v, 3)),
		}
		for i, h := range hennas {
			e := henna.Equipped{SymbolID: h.SymbolID, ActiveSymbolID: h.SymbolID}
			if i == 1 { // the probe marks the second symbol unusable by the class
				e.ActiveSymbolID = 0
			}
			snap.Equipped = append(snap.Equipped, e)
		}
		return FrameHennaInfo(snap)
	}
}
