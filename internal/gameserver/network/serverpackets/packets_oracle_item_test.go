package serverpackets

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Item kinds of testdata/oracle/ProbeItems.java; each is also the position that
// rotates the shared row for the item.
const (
	oracleWeapon = iota
	oracleArmor
	oracleJewel
	oracleQuest
	oracleAdena
)

// oracleSiege is ProbeItems.SIEGE: a stackable etc item whose id lies in the
// siege guard ticket range.
const oracleSiege = 5

// oracleItemTable holds the templates of ProbeItems.template for row v: a
// two-handed weapon, a shadow chest armor, a neck jewel, a quest stack, adena
// and a siege guard ticket. Every reference price is a field of the row.
func oracleItemTable(v [10]int32) *item.Table {
	price := func(kind int) int32 { return rowField(v, kind+9) }
	templates := []*item.Template{
		{
			ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: -1, Crystal: item.CrystalC, ReferencePrice: price(oracleWeapon),
			Weapon: &item.WeaponDetail{},
		},
		{
			ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest, Duration: 60, Crystal: item.CrystalB, ReferencePrice: price(oracleArmor),
			Armor: &item.ArmorDetail{},
		},
		{
			ID: 112, Kind: item.KindArmor, Slot: item.SlotNeck, Duration: -1, Crystal: item.CrystalD, ReferencePrice: price(oracleJewel),
			Armor: &item.ArmorDetail{},
		},
		{
			ID: 1000, Kind: item.KindEtcItem, Slot: item.SlotNone, Duration: -1, Stackable: true, ReferencePrice: price(oracleQuest),
			EtcItem: &item.EtcItemDetail{Type: item.EtcItemQuest},
		},
		{
			ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone, Duration: -1, Stackable: true, ReferencePrice: price(oracleAdena),
			EtcItem: &item.EtcItemDetail{},
		},
		{
			ID: 4000, Kind: item.KindEtcItem, Slot: item.SlotNone, Duration: -1, Stackable: true, ReferencePrice: price(oracleSiege),
			EtcItem: &item.EtcItemDetail{},
		},
	}
	for _, t := range templates {
		t.Tradable, t.Depositable = true, true
	}
	return item.NewTable(templates)
}

// oracleItem is ProbeItems.item: the item of the given kind that row v
// describes, carried in the inventory or worn.
func oracleItem(v [10]int32, kind int, equipped, augmented bool) *item.Instance {
	templateIDs := [...]int32{2368, 1146, 112, 1000, item.AdenaID, 4000}
	inst := &item.Instance{
		ObjectID:     rowField(v, kind),
		TemplateID:   templateIDs[kind],
		Count:        int(rowField(v, kind+1)),
		EnchantLevel: int(rowField(v, kind+2)),
		CustomType1:  int(rowField(v, kind+3)),
		CustomType2:  int(rowField(v, kind+4)),
		ManaLeft:     int(rowField(v, kind+5)),
		Location:     item.LocationInventory,
	}
	if equipped {
		inst.Location = item.LocationPaperdoll
	}
	if augmented {
		inst.Augmentation = &item.Augmentation{Attributes: rowField(v, kind+6)}
	}
	return inst
}

// oracleItems is ProbeItems.items.
func oracleItems(v [10]int32) []*item.Instance {
	return []*item.Instance{
		oracleItem(v, oracleWeapon, true, true),
		oracleItem(v, oracleArmor, true, false),
		oracleItem(v, oracleJewel, false, true),
		oracleItem(v, oracleQuest, false, false),
		oracleItem(v, oracleAdena, false, false),
	}
}

// oracleDepositable is what the reference deposit window selects from the
// inventory (the inventory's available items, then the item's own deposit
// rule): nothing worn and no quest item; a public warehouse also refuses an
// augmented or shadow item. The server makes this selection before it builds
// the packet, so the encoder takes the result.
func oracleDepositable(v [10]int32, private bool) []*item.Instance {
	var out []*item.Instance
	for _, inst := range oracleItems(v) {
		switch {
		case inst.Location == item.LocationPaperdoll, inst.TemplateID == 1000:
		case !private && (inst.Augmentation != nil || inst.TemplateID == 1146):
		default:
			out = append(out, inst)
		}
	}
	return out
}

// oracleItemUpdates are the entries ProbeItems.updates queues, with the same
// states: added, modified, removed, modified, added.
func oracleItemUpdates(v [10]int32) []itemcontainer.Update {
	states := [...]itemcontainer.UpdateState{
		itemcontainer.UpdateAdded, itemcontainer.UpdateModified, itemcontainer.UpdateRemoved,
		itemcontainer.UpdateModified, itemcontainer.UpdateAdded,
	}
	items := oracleItems(v)
	updates := make([]itemcontainer.Update, len(items))
	for i, inst := range items {
		updates[i] = itemcontainer.NewUpdate(inst.Snapshot(), states[i])
	}
	return updates
}

// oracleGroundItem is the dropped item ProbeItems.ground builds: the item of
// the given kind lying at the position its row gives.
type oracleGroundItem struct {
	inst    *item.Instance
	x, y, z int32
}

func oracleGround(v [10]int32, kind int) oracleGroundItem {
	return oracleGroundItem{
		inst: oracleItem(v, kind, false, false),
		x:    rowField(v, kind+7), y: rowField(v, kind+8), z: rowField(v, kind+9),
	}
}

func (g oracleGroundItem) ObjectID() int32 { return g.inst.ObjectID }
func (g oracleGroundItem) ItemID() int32   { return g.inst.TemplateID }
func (g oracleGroundItem) Count() int      { return g.inst.Count }
func (g oracleGroundItem) Stackable() bool { return g.inst.TemplateID != 2368 }
func (g oracleGroundItem) Position() (int, int, int) {
	return int(g.x), int(g.y), int(g.z)
}

// mustFrame unwraps an encoder that can fail only on a missing template, which
// the probe's table always holds.
func mustFrame(f wire.Frame, err error) wire.Frame {
	if err != nil {
		panic(err)
	}
	return f
}

// oracleItemCases are the item, ground-item, freight and warehouse packets
// (testdata/oracle/ProbeItems.java).
func oracleItemCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	table := oracleItemTable([10]int32{})
	m["ItemList/empty"] = func() wire.Frame { return mustFrame(FrameItemList(nil, table, true)) }
	m["PetItemList/empty"] = func() wire.Frame { return mustFrame(FramePetItemList(nil, table)) }
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		table := oracleItemTable(v)
		m["ItemList/show"+n] = func() wire.Frame { return mustFrame(FrameItemList(oracleItems(v), table, true)) }
		m["ItemList/hide"+n] = func() wire.Frame { return mustFrame(FrameItemList(oracleItems(v), table, false)) }
		m["PetItemList"+n] = func() wire.Frame { return mustFrame(FramePetItemList(oracleItems(v), table)) }
		m["GMViewItemList/player"+n] = func() wire.Frame {
			return mustFrame(FrameGMViewItemList(oracleStrings[r].s, int(rowField(v, 7)), oracleItems(v), table))
		}
		m["GMViewItemList/pet"+n] = func() wire.Frame {
			return mustFrame(FrameGMViewItemList(oracleStrings[(r+1)%4].s, int(rowField(v, 8)), oracleItems(v), table))
		}
		m["InventoryUpdate"+n] = func() wire.Frame { return mustFrame(FrameInventoryUpdate(oracleItemUpdates(v), table)) }
		m["PetInventoryUpdate"+n] = func() wire.Frame {
			return mustFrame(FramePetInventoryUpdate(oracleItemUpdates(v), table))
		}

		stack, single := oracleGround(v, oracleAdena), oracleGround(v, oracleWeapon)
		m["SpawnItem/stackable"+n] = func() wire.Frame { return FrameSpawnItem(stack) }
		m["SpawnItem/single"+n] = func() wire.Frame { return FrameSpawnItem(single) }
		m["DropItem/stackable"+n] = func() wire.Frame { return FrameDropItem(stack, rowField(v, 3)) }
		m["DropItem/single"+n] = func() wire.Frame { return FrameDropItem(single, rowField(v, 3)) }
		m["GetItem"+n] = func() wire.Frame { return FrameGetItem(stack, rowField(v, 3)) }

		m["PackageToList"+n] = func() wire.Frame {
			recipients := make([]PackageRecipient, 3)
			for i := range recipients {
				recipients[i] = PackageRecipient{ObjectID: rowField(v, i)*3 + int32(i), Name: oracleStrings[i+1].s}
			}
			return FramePackageToList(recipients)
		}
		m["PackageSendableList"+n] = func() wire.Frame {
			return mustFrame(FramePackageSendableList(rowField(v, 1), rowField(v, 2), oracleItems(v), table))
		}
		for typ := WarehousePrivate; typ <= WarehouseFreight; typ++ {
			m["WarehouseDepositList/"+strconv.Itoa(int(typ))+n] = func() wire.Frame {
				return mustFrame(FrameWarehouseDepositList(typ, rowField(v, 2), oracleDepositable(v, typ == WarehousePrivate), table))
			}
			m["WarehouseWithdrawList/"+strconv.Itoa(int(typ))+n] = func() wire.Frame {
				return mustFrame(FrameWarehouseWithdrawList(typ, rowField(v, 2), oracleItems(v), table))
			}
		}
	}
	return m
}
