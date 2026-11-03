package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: MultiSellChoose.java:307-313 adds each product of a list
// maintaining enchantment, then gives it the taken item's augmentation
// through ItemInstance.setAugmentation (ItemInstance.java:615-629), which
// queues the product MODIFIED with the augmentation in its ItemInfo copy,
// and only then sets the enchant level, which queues nothing when the level
// does not change.

// TestMultisellKeptAugmentationReachesInventoryUpdate exchanges an
// unenchanted augmented sword on a list maintaining enchantment. The product
// is added unaugmented, so the InventoryUpdate lists it added without the
// augmentation and then modified with it.
func TestMultisellKeptAugmentationReachesInventoryUpdate(t *testing.T) {
	t.Parallel()
	w := bootMultisell(t, msTalker(), [][2]int32{{msSwordID, 1}}, nil)
	aug := item.Augmentation{Attributes: 0x12345, SkillID: 3203, SkillLevel: 2}
	w.srv.PlayerInventory(t, w.player).ItemByObjectID(w.items[msSwordID]).SetAugmentation(&aug)
	w.open(t, w.merchant, "Exc_Multisell 9003")

	w.choose(t, "9003", 1, 1)
	w.srv.InventoryUpdates.Tick()
	blade := w.srv.PlayerInventory(t, w.player).ItemByTemplateID(msBladeID)
	if blade == nil {
		t.Fatal("no blade after the exchange")
	}
	type entry struct {
		state        uint16
		augmentation int32
	}
	var got []entry
	for _, f := range drainFrames(t, w.c) {
		if f[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		r := wire.NewReader(f[1:])
		n := r.ReadUint16()
		for range n {
			state := r.ReadUint16()
			r.ReadUint16() // category
			objectID := r.ReadInt32()
			r.ReadInt32()  // template id
			r.ReadInt32()  // count
			r.ReadUint16() // sub-category
			r.ReadUint16() // custom type 1
			r.ReadUint16() // equipped
			r.ReadInt32()  // body slot
			r.ReadUint16() // enchant
			r.ReadUint16() // custom type 2
			augmentation := r.ReadInt32()
			r.ReadInt32() // mana
			if objectID == blade.ObjectID {
				got = append(got, entry{state, augmentation})
			}
		}
		if err := r.Err(); err != nil {
			t.Fatalf("read InventoryUpdate: %v", err)
		}
	}
	want := []entry{
		{uint16(itemcontainer.UpdateAdded), 0},
		{uint16(itemcontainer.UpdateModified), aug.Attributes},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("blade InventoryUpdate entries = %+v, want %+v", got, want)
	}
}
