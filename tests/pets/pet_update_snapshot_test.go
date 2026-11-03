package pets

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: Inventory.addUpdate (Inventory.java:830-865) stores a new
// ItemInfo (ItemInfo.java:24-41) for every queued change, copying the item's
// enchant, custom types and equipped flag at that moment, and
// AbstractInventoryUpdate writes each entry from that copy. Inventory.removeItem
// (Inventory.java:84-105) takes a worn item off before it queues the removal.

// TestTakeBackEnchantedWornWeaponKeepsEnchantInUpdate takes back a worn +3
// wolf weapon: the PetInventoryUpdate lists it modified (taken off) and then
// removed, both entries carrying the +3 it had when they were queued even
// though the weapon has left the pet's inventory by the time they are sent.
func TestTakeBackEnchantedWornWeaponKeepsEnchantInUpdate(t *testing.T) {
	t.Parallel()
	h, wolf, weaponID := bootArmedWolf(t)
	inv := wolf.PetInventory()
	if !inv.SetEnchantLevel(inv.ItemByObjectID(weaponID), 3) {
		t.Fatal("seed pet weapon enchant level refused")
	}
	h.sendAndTick(t, encodeRequestPetUseItem(weaponID))

	frames := h.sendAndTick(t, encodeRequestGetItemFromPet(weaponID, 1))
	var updates [][]byte
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] == serverpackets.OpcodePetInventoryUpdate {
			updates = append(updates, frame)
		}
	}
	if len(updates) != 1 {
		t.Fatalf("take-back frames = opcodes %x, want one PetInventoryUpdate", frameOpcodes(frames))
	}
	want := []petUpdateEntry{
		{state: uint16(itemcontainer.UpdateModified), objectID: weaponID, templateID: fixturePetWeaponID, count: 1, enchant: 3},
		{state: uint16(itemcontainer.UpdateRemoved), objectID: weaponID, templateID: fixturePetWeaponID, count: 1, enchant: 3},
	}
	if got := readPetUpdateEntries(t, updates[0]); !slices.Equal(got, want) {
		t.Fatalf("take-back PetInventoryUpdate = %+v, want %+v", got, want)
	}
}
