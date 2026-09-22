package lifecycle

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFailedDetachFlushKeepsInventoryOnRelog logs a character out with the
// item flush refusing every write, which is what a refused or timed-out
// detach flush looks like. Every item the session changed then stays in the
// pending set with its row unwritten, and the relog has to tell two of those
// entries apart.
//
// A partly destroyed stack is still the player's, so it comes back — and at
// the count the failed write was carrying, not the larger one the row still
// holds, because the unflushed change is the freshest description of the
// item. A fully destroyed stack is gone and stays gone, even though its row
// survived the same failure. A pending entry is not by itself a reason to
// restore or to drop; only the state it carries is.
func TestFailedDetachFlushKeepsInventoryOnRelog(t *testing.T) {
	fault := &gameservertest.ItemFlushFault{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithItemFlushFault(fault),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potions := srv.GiveItem(t, objID, 20, 5)
	spirits := srv.GiveItem(t, objID, 21, 3)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)

	// Both destroys land in memory and leave their row to the lazy task, so
	// both stacks are pending when the session ends.
	c.Send(encodeRequestDestroyItem(potions, 2))
	drainUntilQuiet(t, c)
	c.Send(encodeRequestDestroyItem(spirits, 3))
	drainUntilQuiet(t, c)

	// From here every write fails, so the detach flush keeps both pending
	// instead of dropping them.
	fault.Arm()
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, c, serverpackets.OpcodeCharSelectInfo)
	fault.Disarm()

	rows := persistedItemCounts(t, srv, objID)
	if rows[potions] != 5 {
		t.Fatalf("precondition failed: row %d holds count %d, so the failed flush did land and this run never entered the window the test covers", potions, rows[potions])
	}
	if _, ok := rows[spirits]; !ok {
		t.Fatalf("precondition failed: row %d was already deleted, so this run never entered the stale-row window the test covers", spirits)
	}

	entries := readItemListEntries(t, burstFrame(t, startInWorld(t, c), serverpackets.OpcodeItemList))
	e := findItemListEntry(entries, potions)
	if e == nil {
		t.Fatalf("stack %d retained by the failed detach flush is missing from the relog ItemList: %+v", potions, entries)
	}
	if e.count != 3 {
		t.Fatalf("retained stack count = %d, want 3 (the unflushed count, not the row's 5)", e.count)
	}
	if got := findItemListEntry(entries, spirits); got != nil {
		t.Fatalf("destroyed stack returned in the relog ItemList: %+v", *got)
	}
	if got := findItemListEntry(entries, adena); got == nil || got.count != 100 {
		t.Fatalf("untouched stack %d missing or wrong in the relog ItemList: %+v", adena, entries)
	}

	// The restore takes the row's write over; it does not cancel it. A second
	// logout whose flush fails just as the first did must therefore retain the
	// same state again, rather than leaving the stale row as the only record
	// and handing the destroyed potions back on the login after it.
	fault.Arm()
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, c, serverpackets.OpcodeCharSelectInfo)
	fault.Disarm()

	if rows := persistedItemCounts(t, srv, objID); rows[potions] != 5 {
		t.Fatalf("precondition failed: row %d holds count %d, so the second flush did land and this run never entered the repeated-failure window", potions, rows[potions])
	}
	entries = readItemListEntries(t, burstFrame(t, startInWorld(t, c), serverpackets.OpcodeItemList))
	if e := findItemListEntry(entries, potions); e == nil || e.count != 3 {
		t.Fatalf("stack %d after a second failed detach flush = %+v, want count 3 (the destroyed potions must not come back)", potions, e)
	}

	// With the fault cleared the retained state finally reaches the table, so
	// the row converges on the count the session actually left behind.
	if err := srv.ItemInstances.Save(context.Background()); err != nil {
		t.Fatalf("flush retained item state: %v", err)
	}
	if rows := persistedItemCounts(t, srv, objID); rows[potions] != 3 {
		t.Fatalf("row %d count = %d after the flush succeeds, want 3", potions, rows[potions])
	}
}

// persistedItemCounts returns ownerID's persisted item counts by object id.
func persistedItemCounts(t *testing.T, srv *gameservertest.Server, ownerID int32) map[int32]int {
	t.Helper()
	counts := make(map[int32]int)
	for _, inst := range persistedItems(t, srv, ownerID) {
		counts[inst.ObjectID] = inst.Count
	}
	return counts
}
