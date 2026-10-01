package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestItemListOrdersByEntryTimeThenObjectID pins the order a player's
// inventory reaches the client in: most recently acquired first, descending
// object id between items acquired at the same instant.
//
// The three seeded swords are non-stackable, so each is its own instance, and
// login restores all three at one instant — every entry time ties and the
// object-id tie-break decides the whole list, newest id first.
//
// The second half is what makes the entry time observable on its own. Dropping
// a whole non-stackable item and picking it back up returns the *same*
// instance under its original object id, so the item that carries the lowest
// object id of the three is also the one that entered the inventory last. An
// implementation that ordered on object id alone would keep it last; ordering
// on entry time moves it to the front.
func TestItemListOrdersByEntryTimeThenObjectID(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)

	first := srv.GiveItem(t, objID, 30, 1)
	second := srv.GiveItem(t, objID, 30, 1)
	third := srv.GiveItem(t, objID, 30, 1)
	if !(first < second && second < third) {
		t.Fatalf("fixture object ids %d,%d,%d are not ascending; the tie-break assertion below needs them to be", first, second, third)
	}

	burst := startInWorld(t, c)
	assertItemListOrder(t, burst[enterWorldBurstItemList], []int32{third, second, first}, "EnterWorld burst ItemList")

	// Drop the lowest-id sword. A full drop of a non-stackable item hands the
	// ground the very instance that left the inventory, keeping its id.
	c.Send(encodeRequestDropItem(first, 1, spawnX, spawnY, spawnZ))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // dropper
	groundID := r.ReadInt32()
	if groundID != first {
		t.Fatalf("DropItem ground object id = %d, want the dropped instance %d", groundID, first)
	}

	drainUntilQuiet(t, c)
	// The pickup below must stamp a later entry time than the login restore,
	// or the two tie and the object-id tie-break hides whether the pickup
	// refreshed it. Entry times are wall-clock milliseconds (they persist with
	// the item), while the harness's reads wait on its driven clock and let no
	// wall time pass, so draining alone leaves the pickup inside the restore's
	// millisecond about one run in ten. Wait for the wall clock to leave the
	// millisecond the restore stamped.
	awaitWallClockPast(t, srv.PlayerInventory(t, objID).ItemByObjectID(third).TimeValue())

	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup pending-action release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")
	drainUntilQuiet(t, c)

	c.Send(encodeRequestItemList())
	assertItemListOrder(t, readItemList(t, c), []int32{first, third, second}, "ItemList after re-pickup")
}

// awaitWallClockPast blocks until the wall clock reads a later Unix
// millisecond than ms, the clock containers stamp entry times on.
func awaitWallClockPast(t *testing.T, ms int64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().UnixMilli() <= ms {
		if time.Now().After(deadline) {
			t.Fatalf("wall clock did not pass entry time %d within a second", ms)
		}
		time.Sleep(100 * time.Microsecond)
	}
}

func assertItemListOrder(t *testing.T, frame []byte, want []int32, what string) {
	t.Helper()
	entries := readItemListEntries(t, frame)
	got := make([]int32, len(entries))
	for i, e := range entries {
		got[i] = e.objID
	}
	if len(got) != len(want) {
		t.Fatalf("%s listed %d entries (%v), want %d (%v)", what, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s object ids = %v, want %v", what, got, want)
		}
	}
}
