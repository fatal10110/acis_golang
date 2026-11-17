package pets

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// soleFrameOf returns the one frame of the given opcode, failing on none or
// several.
func soleFrameOf(t *testing.T, frames [][]byte, opcode byte, name string) []byte {
	t.Helper()
	var found [][]byte
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] == opcode {
			found = append(found, frame)
		}
	}
	if len(found) != 1 {
		t.Fatalf("frames %x hold %d %s, want 1", frameOpcodes(frames), len(found), name)
	}
	return found[0]
}

// TestPartialPetStackMoveListsModifiedEntries moves part of a stack between
// the owner and a pet already holding that stack, both ways. The stack only
// changes count on both sides, so the owner's InventoryUpdate and the pet's
// PetInventoryUpdate each list the one stack modified with its new count.
func TestPartialPetStackMoveListsModifiedEntries(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 50})
	wolf, _ := h.spawnWolf(t)
	carried := h.seedPetStack(t, item.AdenaID, 20)
	owned := h.seededItem(t, item.AdenaID)

	frames := h.sendAndTick(t, encodeRequestGiveItemToPet(owned, 10))
	wantPet := []petUpdateEntry{{state: uint16(itemcontainer.UpdateModified), objectID: carried, templateID: item.AdenaID, count: 30}}
	if got := readPetUpdateEntries(t, soleFrameOf(t, frames, serverpackets.OpcodePetInventoryUpdate, "PetInventoryUpdate")); !slices.Equal(got, wantPet) {
		t.Fatalf("give PetInventoryUpdate = %+v, want %+v", got, wantPet)
	}
	wantOwner := []collarUpdateEntry{{state: uint16(itemcontainer.UpdateModified), objectID: owned}}
	if got := readInventoryUpdateCustomTypes(t, soleFrameOf(t, frames, serverpackets.OpcodeInventoryUpdate, "InventoryUpdate")); !slices.Equal(got, wantOwner) {
		t.Fatalf("give InventoryUpdate = %+v, want %+v", got, wantOwner)
	}
	h.requireAdena(t, wolf, 40, 30)

	frames = h.sendAndTick(t, encodeRequestGetItemFromPet(carried, 5))
	wantPet[0].count = 25
	if got := readPetUpdateEntries(t, soleFrameOf(t, frames, serverpackets.OpcodePetInventoryUpdate, "PetInventoryUpdate")); !slices.Equal(got, wantPet) {
		t.Fatalf("take-back PetInventoryUpdate = %+v, want %+v", got, wantPet)
	}
	if got := readInventoryUpdateCustomTypes(t, soleFrameOf(t, frames, serverpackets.OpcodeInventoryUpdate, "InventoryUpdate")); !slices.Equal(got, wantOwner) {
		t.Fatalf("take-back InventoryUpdate = %+v, want %+v", got, wantOwner)
	}
	h.requireAdena(t, wolf, 45, 25)
}
