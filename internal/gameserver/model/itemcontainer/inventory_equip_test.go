package itemcontainer

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// equipFixture bundles an inventory with its template table and a helper to
// equip by template id, allocating sequential object ids.
type equipFixture struct {
	inv       *Inventory
	templates *item.Table
	nextID    int32
}

func newEquipFixture() *equipFixture {
	templates := equipTestTemplates()
	return &equipFixture{
		inv:       NewPlayerInventory(0x10000001, templates),
		templates: templates,
		nextID:    0x20000001,
	}
}

func (f *equipFixture) equip(templateID int32) (*item.Instance, []*item.Instance) {
	tmpl, ok := f.templates.Get(templateID)
	if !ok {
		panic("unknown template id")
	}
	inst := f.inv.AddNew(templateID, 1, f.nextID)
	f.nextID++
	altered := f.inv.EquipItem(inst, tmpl)
	return inst, altered
}

// TestInventory_EquipItem_SlotRules walks the paperdoll slot bookkeeping
// table: paired weapons keep their offhand slot, unpaired equips clear
// conflicting slots, paired accessory slots fill left then right before
// replacing left, and whole-body pieces clear every slot they subsume.
func TestInventory_EquipItem_SlotRules(t *testing.T) {
	t.Run("two-handed clears offhand", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(shieldID)
		if f.inv.ItemAt(LHand) == nil {
			t.Fatalf("shield should occupy LHand")
		}
		twoHand, altered := f.equip(twoHandID)
		if f.inv.ItemAt(RHand) != twoHand {
			t.Errorf("two-handed weapon should occupy RHand")
		}
		if f.inv.ItemAt(LHand) != nil {
			t.Errorf("equipping a two-handed weapon should clear LHand")
		}
		if len(altered) != 2 {
			t.Errorf("altered = %v, want shield unequipped + weapon equipped (2 entries)", altered)
		}
	})

	t.Run("one-handed clears existing two-handed", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(twoHandID)
		sword, _ := f.equip(swordID)
		if f.inv.ItemAt(RHand) != sword {
			t.Errorf("one-handed sword should now occupy RHand")
		}
	})

	t.Run("bow arrow pairing keeps offhand", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(bowID)
		arrow, _ := f.equip(arrowID)
		if f.inv.ItemAt(LHand) != arrow {
			t.Fatalf("arrow should occupy LHand")
		}
		if f.inv.ItemAt(RHand) == nil {
			t.Errorf("equipping an arrow while a bow is worn must not clear the bow")
		}
	})

	t.Run("fishing rod lure pairing keeps offhand", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(rodID)
		f.equip(lureID)
		if f.inv.ItemAt(RHand) == nil {
			t.Errorf("equipping a lure while a fishing rod is worn must not clear the rod")
		}
	})

	t.Run("unpaired offhand clears two-handed", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(twoHandID)
		f.equip(shieldID)
		if f.inv.ItemAt(RHand) != nil {
			t.Errorf("equipping a shield (unpaired LHand item) while two-handed should clear RHand")
		}
	})

	t.Run("ears fill first empty then replace left", func(t *testing.T) {
		f := newEquipFixture()
		first, _ := f.equip(earringID)
		if f.inv.ItemAt(LEar) != first {
			t.Fatalf("first earring should fill LEar")
		}
		second, _ := f.equip(earringID)
		if f.inv.ItemAt(REar) != second {
			t.Fatalf("second earring should fill REar")
		}
		// Both slots full: a third of the *same* template id replaces LEar
		// (matches the reference's "same id as REar -> replace LEar" rule).
		third, _ := f.equip(earringID)
		if f.inv.ItemAt(LEar) != third {
			t.Errorf("third earring of the same template should replace LEar")
		}
	})

	t.Run("fingers same shape", func(t *testing.T) {
		f := newEquipFixture()
		first, _ := f.equip(ringID)
		if f.inv.ItemAt(LFinger) != first {
			t.Fatalf("first ring should fill LFinger")
		}
		second, _ := f.equip(ringID)
		if f.inv.ItemAt(RFinger) != second {
			t.Fatalf("second ring should fill RFinger")
		}
	})

	t.Run("full armor clears legs", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(legsLightID)
		full, _ := f.equip(fullArmorID)
		if f.inv.ItemAt(Chest) != full {
			t.Fatalf("full armor should occupy Chest")
		}
		if f.inv.ItemAt(Legs) != nil {
			t.Errorf("equipping full armor should clear Legs")
		}
	})

	t.Run("legs clears full armor", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(fullArmorID)
		legs, _ := f.equip(legsLightID)
		if f.inv.ItemAt(Legs) != legs {
			t.Fatalf("legs should occupy Legs")
		}
		if f.inv.ItemAt(Chest) != nil {
			t.Errorf("equipping legs while full armor is worn should clear Chest")
		}
	})

	t.Run("all dress clears six slots", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(legsLightID)
		f.equip(shieldID)
		f.equip(swordID)
		dress, _ := f.equip(allDressID)
		if f.inv.ItemAt(Chest) != dress {
			t.Fatalf("all-dress should occupy Chest")
		}
		for _, slot := range []int{Legs, LHand, RHand, Head, Feet, Gloves} {
			if f.inv.ItemAt(slot) != nil {
				t.Errorf("all-dress should clear paperdoll slot %d", slot)
			}
		}
	})

	t.Run("hairall clears face and vice versa", func(t *testing.T) {
		f := newEquipFixture()
		f.equip(faceID)
		hairAll, _ := f.equip(hairAllID)
		if f.inv.ItemAt(Hair) != hairAll {
			t.Fatalf("hairall should occupy Hair")
		}
		if f.inv.ItemAt(Face) != nil {
			t.Errorf("equipping hairall should clear Face")
		}
		hair, _ := f.equip(hairID)
		if f.inv.ItemAt(Hair) != hair {
			t.Fatalf("hair should occupy Hair")
		}
	})
}

func TestInventory_UnequipSlot(t *testing.T) {
	f := newEquipFixture()
	sword, _ := f.equip(swordID)

	old := f.inv.UnequipSlot(RHand)
	if old != sword {
		t.Fatalf("UnequipSlot() = %v, want the equipped sword", old)
	}
	if f.inv.ItemAt(RHand) != nil {
		t.Errorf("RHand should be empty after unequip")
	}
	if sword.Location != f.inv.Location() {
		t.Errorf("unequipped item should move to the inventory's base location, got %v", sword.Location)
	}
}

func TestInventory_WornMask_TwoPieceArmorRequiresMatchingType(t *testing.T) {
	f := newEquipFixture()
	chestTmpl, _ := f.templates.Get(chestLightID)

	f.equip(chestLightID)
	f.equip(legsLightID)
	if !f.inv.IsWearingType(chestTmpl.Mask()) {
		t.Errorf("matching light chest+legs should register the light-armor worn mask")
	}

	f2 := newEquipFixture()
	f2.equip(chestHeavyID)
	f2.equip(legsLightID)
	heavyTmpl, _ := f2.templates.Get(chestHeavyID)
	lightTmpl, _ := f2.templates.Get(legsLightID)
	if f2.inv.IsWearingType(heavyTmpl.Mask()) || f2.inv.IsWearingType(lightTmpl.Mask()) {
		t.Errorf("mismatched chest/legs armor types should not register either worn-type bit")
	}
}

func TestInventory_FindArrowForBow(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1341, Kind: item.KindEtcItem, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
	})
	inv := NewPlayerInventory(0x10000001, templates)
	inv.AddNew(1341, 40, 0x20000001)

	if got := inv.FindArrowForBow(item.CrystalD); got == nil || got.TemplateID != 1341 {
		t.Errorf("FindArrowForBow(CrystalD) = %v, want bone arrow instance", got)
	}
	if got := inv.FindArrowForBow(item.CrystalS); got != nil {
		t.Errorf("FindArrowForBow(CrystalS) = %v, want nil (no shining arrows held)", got)
	}
}
