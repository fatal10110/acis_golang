package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Extractable.java hands each product to Player.addItem(itemId,
// count, true), which names the whole count (Player.java:1830-1840) and
// then calls ItemContainer.addItem(itemId, count): its per-unit loop for a
// non-stackable stops after the first instance when MultipleItemDrop is off
// (ItemContainer.java:224-253).

// TestCapsuleKeepsOneInstanceWithoutMultipleItemDrop opens a capsule
// yielding two weapons with MultipleItemDrop off: one weapon instance is
// added, shown and persisted, and the line still names both.
func TestCapsuleKeepsOneInstanceWithoutMultipleItemDrop(t *testing.T) {
	t.Parallel()
	srv, objID, frames := openCapsule(t, "30,2,100", 5, gameservertest.WithSingleItemDrop())
	assertOneCapsuleSpent(t, srv, objID)

	messages := systemMessages(frames)
	if len(messages) != 1 {
		t.Fatalf("system messages = %d, want the product line", len(messages))
	}
	assertGrantMessage(t, messages[0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(capsuleWeaponID), itemNumberParam(2))

	held := srv.PlayerInventory(t, objID).ItemsByTemplateID(capsuleWeaponID)
	if len(held) != 2 {
		t.Fatalf("held weapon instances = %d, want the held one plus one created", len(held))
	}
	for _, inst := range held {
		if c := inst.CountValue(); c != 1 {
			t.Fatalf("weapon %d holds %d, want 1", inst.ObjectID, c)
		}
	}

	added := 0
	for _, e := range inventoryUpdateAfterTick(t, srv, srv.Client) {
		if e.itemID == capsuleWeaponID && e.state == 1 {
			added++
		}
	}
	if added != 1 {
		t.Fatalf("InventoryUpdate added weapon rows = %d, want 1", added)
	}

	srv.FlushItems(t)
	rows := 0
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.TemplateID == capsuleWeaponID {
			rows++
		}
	}
	if rows != 2 {
		t.Fatalf("persisted weapon rows = %d, want 2", rows)
	}
}
