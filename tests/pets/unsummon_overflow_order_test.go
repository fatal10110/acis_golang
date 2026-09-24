package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestUnsummonOverflowFollowsContainerOrder pins an outcome, not a packet
// order: when the owner cannot hold everything the pet carries, the order the
// pet's inventory is walked in decides which items the player keeps and which
// land on the floor.
//
// transferPetInventory walks petInventory.Items() and, per item, either
// transfers it to the owner or drops it once ValidateCapacity(1) fails. The
// reference walks _items — container order — and makes the same per-item
// decision (PetInventory.deleteMe), so the pet's most recently received item
// is the one that survives a full owner inventory.
//
// The fixture makes the two orders disagree. Adena is seeded first, so it
// carries the lower object id, and it is handed over first, so it carries the
// older entry time; the food is the reverse on both. Walking ascending by
// object id would keep the adena and drop the food; container order keeps the
// food and drops the adena.
func TestUnsummonOverflowFollowsContainerOrder(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t,
		seedItem{TemplateID: item.AdenaID, Count: 100},
		seedItem{TemplateID: wolfFoodID, Count: 2},
	)
	adenaID := h.seededItem(t, item.AdenaID)
	foodID := h.seededItem(t, wolfFoodID)
	if adenaID >= foodID {
		t.Fatalf("fixture object ids adena=%d food=%d: the adena must sort first by object id for this test to discriminate", adenaID, foodID)
	}

	h.spawnWolf(t)
	// Whole stacks, so each keeps its seeded object id inside the pet
	// inventory rather than splitting off a freshly allocated one.
	h.giveToPet(t, adenaID, 100)
	h.giveToPet(t, foodID, 2)

	// The owner is left holding only the collar. One more slot lets exactly
	// one of the two returning items land, and sends the other to the floor.
	h.srv.SetInventorySlotLimit(t, h.ownerID, 2)

	h.returnPet(t)

	if got := h.ownerItemCount(t, wolfFoodID); got != 2 {
		t.Fatalf("owner food count after unsummon = %d, want 2: the most recently received pet item must be the one kept", got)
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 0 {
		t.Fatalf("owner adena count after unsummon = %d, want 0: it is the older entry and must overflow to the ground", got)
	}

	// The overflowed stack is not destroyed, it is on the floor: the same
	// instance, under its original object id, now a live world object.
	if _, ok := h.srv.State.Object(adenaID); !ok {
		t.Fatalf("adena instance %d is neither held by the owner nor present on the ground after unsummon", adenaID)
	}
}
