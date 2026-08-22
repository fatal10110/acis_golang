package lifecycle

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestShutdownFlushMatchesFinalInventoryState drops part of an adena stack
// and shuts the server down with the client still connected: no logout
// flush, no lazy-persistence tick. The shutdown drain must land the pending
// mutation so the items row equals the final in-memory state, and the next
// boot must restore that state rather than the pre-drop one.
func TestShutdownFlushMatchesFinalInventoryState(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)

	c.Send(encodeRequestDropItem(adena, 40, spawnX, spawnY, spawnZ))
	groundID := readDropItemGroundID(t, c.Read(), objID, item.AdenaID, 40)

	srv.Shutdown(t)

	if counts := persistedAdena(t, srv, objID); len(counts) != 1 || counts[0] != 60 {
		t.Fatalf("persisted adena stacks after shutdown = %v, want one stack of 60", counts)
	}
	rows := groundRows(t, srv)
	if len(rows) != 1 || rows[0].ObjectID != groundID || rows[0].Count != 40 {
		t.Fatalf("items_on_ground rows after shutdown = %+v, want one adena row object %d count 40", rows, groundID)
	}

	srv2 := gameservertest.Boot(t, gameservertest.WithWantChars(1))
	if got := srv2.SoleObjectID(t); got != objID {
		t.Fatalf("second boot character id = %d, want %d", got, objID)
	}
	frames := startInWorld(t, srv2.Client)
	e := findItemListEntry(readItemListEntries(t, burstFrame(t, frames, serverpackets.OpcodeItemList)), adena)
	if e == nil {
		t.Fatalf("restored ItemList has no adena row %d", adena)
	}
	if e.count != 60 {
		t.Fatalf("restored adena count = %d, want 60 (the final pre-shutdown state)", e.count)
	}
}

// TestShutdownFlushPersistsDestruction destroys a whole stack and shuts the
// server down before any flush tick: the shutdown drain must delete the row,
// not leave a phantom stack behind for the next boot to resurrect.
func TestShutdownFlushPersistsDestruction(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potions := srv.GiveItem(t, objID, 20, 5)
	startInWorld(t, c)

	c.Send(encodeRequestDestroyItem(potions, 5))
	testsupport.SyncBarrier(t, c, func() { c.Send(encodeRequestItemList()) }, serverpackets.OpcodeItemList)

	srv.Shutdown(t)

	for _, inst := range persistedItems(t, srv, objID) {
		if inst.ObjectID == potions {
			t.Fatalf("destroyed potion still persisted after shutdown: %+v", inst)
		}
	}
}

// persistedItems returns every items row persisted for ownerID.
func persistedItems(t *testing.T, srv *gameservertest.Server, ownerID int32) []*item.Instance {
	t.Helper()
	instances, err := srv.Items.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	return instances
}

// persistedAdena returns the count of every adena stack persisted for
// ownerID.
func persistedAdena(t *testing.T, srv *gameservertest.Server, ownerID int32) []int32 {
	t.Helper()
	var counts []int32
	for _, inst := range persistedItems(t, srv, ownerID) {
		if inst.TemplateID == item.AdenaID {
			counts = append(counts, int32(inst.Count))
		}
	}
	return counts
}

// groundRows reads the raw items_on_ground rows through the production
// store.
func groundRows(t *testing.T, srv *gameservertest.Server) []item.GroundSnapshot {
	t.Helper()
	rows, err := gamesql.NewGroundItemStore(srv.DB).Load(context.Background())
	if err != nil {
		t.Fatalf("load ground rows: %v", err)
	}
	return rows
}
