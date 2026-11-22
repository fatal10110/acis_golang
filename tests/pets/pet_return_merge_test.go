package pets

import "testing"

// TestReturnPetMergesStackIntoFullOwner returns a wolf carrying bones and a
// sword to an owner with no free slot (#5089): the bones merge into the
// owner's bone stack, which needs no slot, and only the sword lands on the
// ground.
func TestReturnPetMergesStackIntoFullOwner(t *testing.T) {
	t.Parallel()
	h := bootTakeBackOwner(t,
		seedItem{TemplateID: animalBoneID, Count: 10},
		seedItem{TemplateID: shortSwordID, Count: 1},
	)
	h.spawnWolf(t)
	sword := h.seededItem(t, shortSwordID)
	h.giveToPet(t, sword, 1)
	h.giveToPet(t, h.seededItem(t, animalBoneID), 4)
	// The owner keeps the collar and six bones, and no free slot.
	h.srv.SetInventorySlotLimit(t, h.ownerID, h.ownerInventory(t).Size())

	h.returnPet(t)

	if got := countTemplate(h.ownerInventory(t), animalBoneID); got != 10 {
		t.Fatalf("owner bones in memory after the return = %d, want 10", got)
	}
	if got := h.ownerItemCount(t, animalBoneID); got != 10 {
		t.Fatalf("owner bone rows after the return = %d, want 10", got)
	}
	if got := h.petRowCount(t, animalBoneID) + h.petRowCount(t, shortSwordID) + h.ownerItemCount(t, shortSwordID); got != 0 {
		t.Fatalf("pet bone, pet sword and owner sword rows after the return = %d units, want none", got)
	}
	if _, ok := h.srv.State.Object(sword); !ok {
		t.Fatalf("sword %d is neither held nor on the ground after the return", sword)
	}
}
