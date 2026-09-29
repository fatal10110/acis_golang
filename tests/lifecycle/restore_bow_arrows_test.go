package lifecycle

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRestoreWornBowPullsItsArrows: Inventory.restore (Inventory.java:108-154)
// re-equips each worn row through equipItem, so the bow listener runs on
// login too. Rows come back in loc_data order, the unequipped arrows (slot 0)
// ahead of the bow's right hand, and the bow takes them into the left hand:
// the login ItemList shows them worn.
func TestRestoreWornBowPullsItsArrows(t *testing.T) {
	const (
		bowID       = 14
		woodenArrow = 17
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	rightHand, ok := item.SlotRHand.PaperdollIndex()
	if !ok {
		t.Fatal("no paperdoll index for the right hand")
	}
	bow := srv.NewObjectID()
	if err := srv.Items.Create(context.Background(), objID, item.Instance{
		ObjectID: bow, TemplateID: bowID, OwnerID: objID, Count: 1,
		Location: item.LocationPaperdoll, LocationData: rightHand,
	}); err != nil {
		t.Fatalf("seed worn bow: %v", err)
	}
	arrows := srv.GiveItem(t, objID, woodenArrow, 10)

	entries := readItemListEntries(t, burstFrame(t, startInWorld(t, c), serverpackets.OpcodeItemList))
	for _, id := range []int32{bow, arrows} {
		e := findItemListEntry(entries, id)
		if e == nil || e.equipped != 1 {
			t.Fatalf("ItemList entry for %d = %+v, want worn", id, e)
		}
	}
}
