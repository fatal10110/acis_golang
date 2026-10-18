package items

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Inventory.addUpdate (Inventory.java:830-865) stores a new
// ItemInfo (ItemInfo.java:24-41) for every queued change, copying the item's
// enchant, augmentation, count, custom types, equipped flag and mana at that
// moment; AbstractInventoryUpdate writes each entry from that copy, the mana
// field as ItemInfo.getDisplayedManaLeft, _manaLeft / 60, which is 0 for a
// regular item (mana_left -1). Inventory.removeItem (Inventory.java:84-105)
// takes a worn item off before it queues the removal.

// snapshotEntry is every instance field of one InventoryUpdate entry.
type snapshotEntry struct {
	state        uint16
	objID        int32
	count        int32
	customType1  uint16
	equipped     uint16
	enchant      uint16
	customType2  uint16
	augmentation int32
	mana         int32
}

func readSnapshotEntries(t *testing.T, frame []byte) []snapshotEntry {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeInventoryUpdate, "InventoryUpdate")
	r := wire.NewReader(frame[1:])
	n := r.ReadUint16()
	entries := make([]snapshotEntry, 0, n)
	for range n {
		var e snapshotEntry
		e.state = r.ReadUint16()
		r.ReadUint16() // category
		e.objID = r.ReadInt32()
		r.ReadInt32() // template id
		e.count = r.ReadInt32()
		r.ReadUint16() // sub-category
		e.customType1 = r.ReadUint16()
		e.equipped = r.ReadUint16()
		r.ReadInt32() // body slot
		e.enchant = r.ReadUint16()
		e.customType2 = r.ReadUint16()
		e.augmentation = r.ReadInt32()
		e.mana = r.ReadInt32()
		entries = append(entries, e)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read InventoryUpdate: %v", err)
	}
	return entries
}

// TestDestroyEnchantedWornSwordUpdateKeepsQueuedState destroys a worn +4
// sword with custom types set. The InventoryUpdate lists it taken off and
// then removed; although the sword is gone when the update is sent, both
// entries carry the enchant and custom types it had when they were queued,
// not worn, with the regular item's mana field 0.
func TestDestroyEnchantedWornSwordUpdateKeepsQueuedState(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, 30, 1)
	inst := mustFindItem(t, srv, objID, sword)
	inst.EnchantLevel = 4
	inst.CustomType1 = 2
	inst.CustomType2 = 3
	if err := srv.Items.Update(context.Background(), inst); err != nil {
		t.Fatalf("seed sword state: %v", err)
	}
	startInWorld(t, c)
	wearAndSettle(t, srv, c, sword)

	c.Send(encodeRequestDestroyItem(sword, 1))
	drainUntilQuiet(t, c)
	srv.InventoryUpdates.Tick()
	var got []snapshotEntry
	for _, f := range collectUntilQuiet(t, c) {
		if f[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		for _, e := range readSnapshotEntries(t, f) {
			if e.objID == sword {
				got = append(got, e)
			}
		}
	}
	want := []snapshotEntry{
		{state: uint16(itemcontainer.UpdateModified), objID: sword, count: 1, customType1: 2, enchant: 4, customType2: 3},
		{state: uint16(itemcontainer.UpdateRemoved), objID: sword, count: 1, customType1: 2, enchant: 4, customType2: 3},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("destroy InventoryUpdate entries for the sword = %+v, want %+v", got, want)
	}
}
