package admin

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// wornHPBonus is the max HP the worn sword of the reload tests below adds.
const wornHPBonus = 100

// wornSwordTable is the shared catalog with the sword adding wornHPBonus max
// HP, plus a copy of it as orphanWeaponID when withOrphan is set. Each call
// builds new templates, as a reload does.
func wornSwordTable(withOrphan bool) *item.Table {
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == swordID {
			rigged := *tmpl
			rigged.Modifiers = []item.StatModifier{{Op: item.FuncAdd, Stat: "maxHp", Value: wornHPBonus}}
			tmpl = &rigged
			if withOrphan {
				orphan := rigged
				orphan.ID = orphanWeaponID
				templates = append(templates, &orphan)
			}
		}
		templates = append(templates, tmpl)
	}
	return item.NewTable(templates)
}

// bootWornSword boots the GM wearing a sword of template id in the right
// hand, reloading the item table to reloaded, and returns the server, the
// sword's object id and the max HP the GM entered with.
func bootWornSword(t *testing.T, id int32, reloaded func() *item.Table) (*gameservertest.Server, int32, int32) {
	t.Helper()
	items := wornSwordTable(true)
	srv, objID := bootAdmin(t, adminLevel,
		gameservertest.WithItemTemplates(items),
		gameservertest.WithDataReloads(network.DataReloads{Items: func() error {
			items.Replace(reloaded())
			return nil
		}}),
	)
	sword := srv.GiveItem(t, objID, id, 1)
	row := itemRow(t, srv, objID, sword)
	row.Location, row.LocationData = item.LocationPaperdoll, itemcontainer.RHand
	if err := srv.Items.Update(context.Background(), row); err != nil {
		t.Fatalf("wear the sword: %v", err)
	}
	hp := lastMaxHP(t, enterWorldBurst(t, srv.Client))
	return srv, sword, hp
}

// TestReloadThenUnequipDropsWornItemStats wears the sword, reloads the item
// table with the sword unchanged, and takes the sword off: its max HP goes
// with it, and wearing it again adds it once, not twice.
func TestReloadThenUnequipDropsWornItemStats(t *testing.T) {
	t.Parallel()
	srv, sword, worn := bootWornSword(t, swordID, func() *item.Table { return wornSwordTable(true) })
	gm := srv.Client
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload item")), "Items' templates have been reloaded.")

	if got := lastMaxHP(t, exchange(t, gm, encodeUnequip(int32(item.SlotRHand)))); got != worn-wornHPBonus {
		t.Fatalf("max HP after taking the sword off = %d, want %d", got, worn-wornHPBonus)
	}
	if got := lastMaxHP(t, exchange(t, gm, encodeUseItem(sword))); got != worn {
		t.Fatalf("max HP after wearing the sword again = %d, want %d", got, worn)
	}
}

// TestReloadedAwayWornItemUnequipDropsItsStats wears a sword whose template
// the reload removes, then takes it off: the max HP it added goes with it.
func TestReloadedAwayWornItemUnequipDropsItsStats(t *testing.T) {
	t.Parallel()
	srv, _, worn := bootWornSword(t, orphanWeaponID, func() *item.Table { return wornSwordTable(false) })
	gm := srv.Client
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload item")), "Items' templates have been reloaded.")

	if got := lastMaxHP(t, exchange(t, gm, encodeUnequip(int32(item.SlotRHand)))); got != worn-wornHPBonus {
		t.Fatalf("max HP after taking the reloaded-away sword off = %d, want %d", got, worn-wornHPBonus)
	}
}

// orphanShadowID is a copy of the shared shadow sword (7884) that only the
// booted item table defines.
const orphanShadowID int32 = 65012

// TestReloadedAwayShadowItemStopsDecayingOnUnequip wears a shadow weapon
// whose template the reload removes, then takes it off: like any shadow
// weapon taken off, it stops losing mana, so the decay never runs it down
// to nothing and destroys its row.
func TestReloadedAwayShadowItemStopsDecayingOnUnequip(t *testing.T) {
	t.Parallel()
	table := func(withOrphan bool) *item.Table {
		templates := gameservertest.ItemTemplates().All()
		if withOrphan {
			shadow, _ := gameservertest.ItemTemplates().Get(7884)
			orphan := *shadow
			orphan.ID = orphanShadowID
			templates = append(templates, &orphan)
		}
		return item.NewTable(templates)
	}
	items := table(true)
	srv, objID := bootAdmin(t, adminLevel,
		gameservertest.WithItemTemplates(items),
		gameservertest.WithDataReloads(network.DataReloads{Items: func() error {
			items.Replace(table(false))
			return nil
		}}),
	)
	gm := srv.Client
	shadow := srv.NewObjectID()
	worn := item.Instance{ObjectID: shadow, TemplateID: orphanShadowID, OwnerID: objID, Count: 1, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand, ManaLeft: 300}
	if err := srv.Items.Create(context.Background(), objID, worn); err != nil {
		t.Fatalf("seed the shadow sword: %v", err)
	}
	enterWorldBurst(t, gm)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload item")), "Items' templates have been reloaded.")
	exchange(t, gm, encodeUnequip(int32(item.SlotRHand)))

	srv.FlushItems(t)
	before := itemRow(t, srv, objID, shadow).ManaLeft
	for range 3 {
		srv.ShadowItems.Tick()
	}
	srv.FlushItems(t)
	if row := itemRow(t, srv, objID, shadow); row.ManaLeft != before || row.Location != item.LocationInventory {
		t.Fatalf("shadow sword after three ticks off = mana %d at %s, want mana %d in the inventory", row.ManaLeft, row.Location, before)
	}
}

func encodeUnequip(bodySlot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUnEquipItem)
	w.WriteInt32(bodySlot)
	return w.Bytes()
}

func encodeUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	return w.Bytes()
}

// lastMaxHP decodes MaxHP out of the last UserInfo among frames.
func lastMaxHP(t *testing.T, frames [][]byte) int32 {
	t.Helper()
	var last []byte
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeUserInfo {
			last = frame
		}
	}
	if last == nil {
		t.Fatal("no UserInfo among the frames")
	}
	r := wire.NewReader(last[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	for range 4 { // race, sex, class, level
		r.ReadInt32()
	}
	r.ReadInt64()
	for range 6 { // STR..MEN
		r.ReadInt32()
	}
	return r.ReadInt32()
}
