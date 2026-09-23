package lifecycle

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRestoreMergedStackWritesBothRows covers two persisted rows of one
// stackable template, which is what a live merge leaves behind when only one
// of its two writes lands. The login restores them as one stack, and that
// merge has to reach the table: the grown stack is written and the absorbed
// row is deleted. Otherwise the absorbed row survives and is added to the
// stack again on every later login.
func TestRestoreMergedStackWritesBothRows(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	first := srv.GiveItem(t, objID, item.AdenaID, 100)
	second := srv.GiveItem(t, objID, item.AdenaID, 50)
	potions := srv.GiveItem(t, objID, 20, 5)

	entries := readItemListEntries(t, burstFrame(t, startInWorld(t, c), serverpackets.OpcodeItemList))
	assertSingleAdena(t, entries, 150)

	// Only the merge is a change; a row restored as it was read is not.
	if srv.ItemInstances.ContainsID(potions) {
		t.Fatalf("unmerged row %d scheduled a write on restore", potions)
	}
	if !srv.ItemInstances.ContainsID(first) || !srv.ItemInstances.ContainsID(second) {
		t.Fatalf("merged rows pending = %v/%v, want both scheduled",
			srv.ItemInstances.ContainsID(first), srv.ItemInstances.ContainsID(second))
	}
	// The rows are read back in no particular order, so either one may be
	// the survivor.
	survivor, absorbed, absorbedCount := first, second, 50
	if findItemListEntry(entries, first) == nil {
		survivor, absorbed, absorbedCount = second, first, 100
	}

	// A player can relog before the lazy task runs. The detach flush then
	// writes the grown stack while the absorbed row's delete is still only
	// pending, so the next restore reads both rows back and must not merge
	// the absorbed one in again.
	for login := 2; login <= 4; login++ {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
		readUntilOpcode(t, c, serverpackets.OpcodeCharSelectInfo)
		if rows := persistedItemCounts(t, srv, objID); rows[absorbed] != absorbedCount || rows[survivor] != 150 {
			t.Fatalf("login %d: precondition failed: rows %v, want survivor %d at 150 and absorbed %d still at %d, so this run never entered the relog-before-flush window", login, rows, survivor, absorbed, absorbedCount)
		}
		entries = readItemListEntries(t, burstFrame(t, startInWorld(t, c), serverpackets.OpcodeItemList))
		assertSingleAdena(t, entries, 150)
		if !srv.ItemInstances.ContainsID(absorbed) {
			t.Fatalf("login %d: absorbed row's delete is no longer pending", login)
		}
	}

	if err := srv.ItemInstances.Save(context.Background()); err != nil {
		t.Fatalf("flush merged stack: %v", err)
	}
	assertPersistedAdena(t, srv, objID, 150)

	// The count must hold across relogs rather than grow by the absorbed
	// row each time.
	for login := 5; login <= 7; login++ {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
		readUntilOpcode(t, c, serverpackets.OpcodeCharSelectInfo)
		entries = readItemListEntries(t, burstFrame(t, startInWorld(t, c), serverpackets.OpcodeItemList))
		assertSingleAdena(t, entries, 150)
		if err := srv.ItemInstances.Save(context.Background()); err != nil {
			t.Fatalf("login %d: flush: %v", login, err)
		}
		assertPersistedAdena(t, srv, objID, 150)
	}
}

func assertSingleAdena(t *testing.T, entries []itemListEntry, want int32) {
	t.Helper()
	var counts []int32
	for _, e := range entries {
		if e.itemID == item.AdenaID {
			counts = append(counts, e.count)
		}
	}
	if len(counts) != 1 || counts[0] != want {
		t.Fatalf("ItemList adena stacks = %v, want [%d]", counts, want)
	}
}

func assertPersistedAdena(t *testing.T, srv *gameservertest.Server, ownerID int32, want int32) {
	t.Helper()
	if got := persistedAdena(t, srv, ownerID); len(got) != 1 || got[0] != want {
		t.Fatalf("persisted adena rows = %v, want [%d]", got, want)
	}
}

// TestRestoreMergedStackKeepsInventoryRowOverEquippedRow covers a merge whose
// two rows sit at different locations: one equipped in the arrow slot, one in
// the inventory. Rows are restored in slot order, so the inventory row (slot
// 0) is read first and survives; the equipped row is absorbed into it and the
// merged stack comes back unequipped. The equipped row is inserted first so
// that a restore reading rows in insertion order would keep it instead.
func TestRestoreMergedStackKeepsInventoryRowOverEquippedRow(t *testing.T) {
	const woodenArrow = 17
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	arrowSlot, ok := item.SlotLHand.PaperdollIndex()
	if !ok {
		t.Fatal("no paperdoll index for the left hand")
	}
	equipped := srv.NewObjectID()
	if err := srv.Items.Create(context.Background(), objID, item.Instance{
		ObjectID: equipped, TemplateID: woodenArrow, OwnerID: objID, Count: 10,
		Location: item.LocationPaperdoll, LocationData: arrowSlot,
	}); err != nil {
		t.Fatalf("seed equipped arrows: %v", err)
	}
	inventory := srv.GiveItem(t, objID, woodenArrow, 5)

	entries := readItemListEntries(t, burstFrame(t, startInWorld(t, c), serverpackets.OpcodeItemList))
	var arrows []itemListEntry
	for _, e := range entries {
		if e.itemID == woodenArrow {
			arrows = append(arrows, e)
		}
	}
	if len(arrows) != 1 || arrows[0].objID != inventory || arrows[0].count != 15 || arrows[0].equipped != 0 {
		t.Fatalf("ItemList arrows = %+v, want one unequipped stack of 15 under object %d", arrows, inventory)
	}

	if err := srv.ItemInstances.Save(context.Background()); err != nil {
		t.Fatalf("flush merged stack: %v", err)
	}
	var rows []*item.Instance
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.TemplateID == woodenArrow {
			rows = append(rows, inst)
		}
	}
	if len(rows) != 1 || rows[0].ObjectID != inventory || rows[0].Count != 15 || rows[0].Location != item.LocationInventory {
		t.Fatalf("persisted arrow rows = %+v, want only object %d at 15 in INVENTORY", rows, inventory)
	}
}
