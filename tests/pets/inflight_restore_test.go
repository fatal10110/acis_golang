package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestDestroyedCollarIsNotRestoredWhileItsDeleteIsQueued drives the one
// interleaving the restore guard has to survive. The item save has already
// taken the destroyed collar out of the pending set, but its row delete is
// still queued on the collar's own persistence lane — the lane a destroyed
// collar's write is routed to, and the one lane the login does not wait on.
// Restoring the row there would hand the collar back, and the next detach
// flush would re-insert it, so the guard has to keep answering for an item
// until its write has run rather than until a save merely picked it up.
func TestDestroyedCollarIsNotRestoredWhileItsDeleteIsQueued(t *testing.T) {
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithReuseDelays(0, 0)})
	// Holding the collar's lane must not also hold the owner's, or it would
	// stall the logout instead of the delete the test is about.
	if uint32(h.collarID)%persist.Lanes == uint32(h.ownerID)%persist.Lanes {
		t.Fatalf("collar %d and owner %d share a persistence lane; this test needs them apart", h.collarID, h.ownerID)
	}

	h.client.Send(encodeRequestDestroyItem(h.collarID, 1))
	waitFor(t, "collar destroyed", func() bool {
		return liveInventory(t, h).ItemByObjectID(h.collarID) == nil
	})
	drainUntilQuiet(t, h.client)

	release := h.srv.HoldPersistenceLane(t, h.collarID)
	// The save empties the pending set and queues the delete behind the held
	// job, then gives up on its own ctx. The round stays outstanding, which
	// is the state a real tick leaves behind on a backed-up lane.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	_ = h.srv.ItemInstances.Save(ctx)
	cancel()

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	if !persistedRowExists(t, h, h.collarID) {
		t.Fatalf("precondition failed: row %d was already deleted, so the queued-delete window never happened", h.collarID)
	}
	startInWorld(t, h.client)

	if liveInventory(t, h).ItemByObjectID(h.collarID) != nil {
		t.Fatalf("destroyed collar %d was restored while its row delete was still queued", h.collarID)
	}
	release()
}

// liveInventory returns the owner's inventory from world state.
func liveInventory(t *testing.T, h *petWorld) *itemcontainer.Inventory {
	t.Helper()
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatalf("owner %d not in world state", h.ownerID)
	}
	return obj.(interface {
		Inventory() *itemcontainer.Inventory
	}).Inventory()
}

// persistedRowExists reports whether the owner still has an items row for
// objectID.
func persistedRowExists(t *testing.T, h *petWorld, objectID int32) bool {
	t.Helper()
	rows, err := h.srv.Items.ListByOwner(context.Background(), h.ownerID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	for _, inst := range rows {
		if inst.ObjectID == objectID {
			return true
		}
	}
	return false
}
