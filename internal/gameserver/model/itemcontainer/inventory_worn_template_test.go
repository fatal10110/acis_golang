package itemcontainer

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// dropTemplate replaces f's template table, as a reload does, with one that
// no longer defines id.
func (f *equipFixture) dropTemplate(id int32) {
	var kept []*item.Template
	for _, tmpl := range f.templates.All() {
		if tmpl.ID != id {
			kept = append(kept, tmpl)
		}
	}
	f.templates.Replace(item.NewTable(kept))
}

// TestInventory_WornMask_ReloadedAwayItemLeavesIt takes off worn items whose
// template a reload removed: the worn-type bits they set go with them.
func TestInventory_WornMask_ReloadedAwayItemLeavesIt(t *testing.T) {
	t.Run("weapon", func(t *testing.T) {
		f := newEquipFixture()
		swordTmpl, _ := f.templates.Get(swordID)
		f.equip(swordID)
		f.dropTemplate(swordID)

		f.inv.UnequipSlot(RHand)
		if f.inv.IsWearingType(swordTmpl.Mask()) {
			t.Fatal("the reloaded-away sword still counts in the worn mask after coming off")
		}
	})

	t.Run("two-piece chest", func(t *testing.T) {
		f := newEquipFixture()
		chestTmpl, _ := f.templates.Get(chestLightID)
		f.equip(chestLightID)
		f.equip(legsLightID)
		f.dropTemplate(chestLightID)

		f.inv.UnequipSlot(Chest)
		if f.inv.IsWearingType(chestTmpl.Mask()) {
			t.Fatal("the light armor bit survived the reloaded-away chest coming off")
		}
	})

	t.Run("legs against a reloaded-away chest", func(t *testing.T) {
		f := newEquipFixture()
		chestTmpl, _ := f.templates.Get(chestLightID)
		f.equip(chestLightID)
		f.dropTemplate(chestLightID)

		f.equip(legsLightID)
		if !f.inv.IsWearingType(chestTmpl.Mask()) {
			t.Fatal("light legs going on over a reloaded-away light chest did not set the light armor bit")
		}
	})
}

// TestInventory_ReloadedAwayBowReleasesItsArrows takes off a worn bow whose
// template a reload removed: like any bow leaving the right hand, it takes
// its arrows out of the left hand.
func TestInventory_ReloadedAwayBowReleasesItsArrows(t *testing.T) {
	f := newEquipFixture()
	bow, _ := f.equip(bowID)
	arrowTmpl, _ := f.templates.Get(arrowID)
	arrows := f.inv.AddNew(arrowID, 10, f.nextID)
	f.inv.SetPaperdollItem(LHand, arrows, arrowTmpl)
	f.dropTemplate(bowID)

	altered := f.inv.UnequipItem(bow)
	if f.inv.ItemAt(LHand) != nil {
		t.Fatal("the arrows stayed in the left hand after the reloaded-away bow came off")
	}
	if len(altered) != 2 || altered[0] != arrows || altered[1] != bow {
		t.Fatalf("UnequipItem altered = %v, want the arrows then the bow", altered)
	}
}
