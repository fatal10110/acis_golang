package items

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// spawnX/Y/Z is the class template's single spawn point both clients of a
// boot share, so dropped items land inside every observer's known range.
const (
	spawnX = 10
	spawnY = 20
	spawnZ = 30
)

// TestDropGroundItemRoundTrip drops adena to the ground through
// RequestDropItem and walks the full ground lifecycle: DropItem broadcast to
// the dropper and to a second client that already knows the dropper, an
// items_on_ground row once the server persists ground state, pickup via an
// Action click back into the merged inventory stack (DeleteObject broadcast,
// tick-driven InventoryUpdate), and an empty items_on_ground table after the
// pickup. Movement must stay responsive after the pickup resolves.
func TestDropGroundItemRoundTrip(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)

	srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestDropItem(adena, 40, spawnX, spawnY, spawnZ))

	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("DropItem dropper id = %d, want %d", got, objID)
	}
	groundID := r.ReadInt32()
	if groundID == adena {
		t.Fatalf("DropItem ground object id reused source stack id %d", groundID)
	}
	if got := r.ReadInt32(); got != item.AdenaID {
		t.Fatalf("DropItem item id = %d, want adena", got)
	}
	x, y, z := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if x != spawnX || y != spawnY || z != spawnZ {
		t.Fatalf("DropItem location = (%d,%d,%d), want (%d,%d,%d)", x, y, z, spawnX, spawnY, spawnZ)
	}
	if stackable := r.ReadInt32(); stackable != 1 {
		t.Fatalf("DropItem stackable = %d, want 1", stackable)
	}
	if got := r.ReadInt32(); got != 40 {
		t.Fatalf("DropItem count = %d, want 40", got)
	}

	oframe := observer.Read()
	assertFrameOpcode(t, oframe, serverpackets.OpcodeDropItem, "observer DropItem")
	or := wire.NewReader(oframe[1:])
	or.ReadInt32() // dropper
	if got := or.ReadInt32(); got != groundID {
		t.Fatalf("observer DropItem ground id = %d, want %d", got, groundID)
	}
	drainUntilQuiet(t, observer)

	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatalf("world.Object(%d) missing for dropped item", groundID)
	}

	srv.FlushGroundItems(t)
	rows, err := groundRows(t, srv)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ObjectID != groundID ||
		rows[0].TemplateID != item.AdenaID || rows[0].Count != 40 {
		t.Fatalf("items_on_ground rows = %+v, want one adena row object %d count 40", rows, groundID)
	}

	// Pickup: click the ground item. The click is in range, so it collects
	// immediately: ActionFailed releases the pending action, GetItem plays
	// the collection.
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeActionFailed, "pickup pending-action release")
	reply = c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeGetItem, "GetItem")

	// Every client that knows the ground item sees the collection animation
	// (GetItem carries the picker id then the ground item id) followed by
	// the item's removal.
	frame = observer.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeGetItem, "observer GetItem")
	gr := wire.NewReader(frame[1:])
	picker, pickedGround := gr.ReadInt32(), gr.ReadInt32()
	if picker != objID || pickedGround != groundID {
		t.Fatalf("observer GetItem = picker %d item %d, want %d/%d", picker, pickedGround, objID, groundID)
	}
	for _, who := range []*struct {
		name   string
		client *testsupport.ScriptedClient
	}{{"dropper", c}, {"observer", observer}} {
		frame = who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeDeleteObject, who.name+" DeleteObject")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != groundID {
			t.Fatalf("%s DeleteObject object id = %d, want %d", who.name, got, groundID)
		}
	}

	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, adena, 100)
	drainUntilQuiet(t, observer)

	if _, ok := srv.State.Object(groundID); ok {
		t.Fatalf("world.Object(%d) still present after pickup", groundID)
	}
	srv.FlushGroundItems(t)
	rows, err = groundRows(t, srv)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("items_on_ground rows after pickup = %+v, want none", rows)
	}

	srv.FlushItems(t)
	instances := persistedItems(t, srv, objID)
	count := 0
	for _, inst := range instances {
		if inst.TemplateID == item.AdenaID {
			count += inst.Count
		}
	}
	if count != 100 {
		t.Fatalf("persisted adena after round trip = %d, want 100 back in inventory", count)
	}

	// Movement must still work after the pickup resolves.
	c.Send(encodeMoveBackwardToLocation(80, 70, 30, spawnX, spawnY, spawnZ))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToLocation, "movement after pickup")
}

// TestDropItemRejectsZeroCount pins the zero-count drop rejection: only the
// CannotDiscardThisItem system message answers, and nothing leaves the
// inventory.
func TestDropItemRejectsZeroCount(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)

	c.Send(encodeRequestDropItem(adena, 0, spawnX, spawnY, spawnZ))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotDiscardThisItem)
	testsupport.SyncBarrier(t, c, func() { c.Send(encodeRequestItemList()) }, serverpackets.OpcodeItemList)

	for _, inst := range persistedItems(t, srv, objID) {
		if inst.ObjectID == adena && inst.Count != 100 {
			t.Fatalf("adena count after rejected drop = %d, want 100", inst.Count)
		}
	}
}

func groundRows(t *testing.T, srv *gameservertest.Server) ([]item.GroundSnapshot, error) {
	t.Helper()
	return gamesql.NewGroundItemStore(srv.DB).Load(context.Background())
}

func persistedItems(t *testing.T, srv *gameservertest.Server, ownerID int32) []*item.Instance {
	t.Helper()
	instances, err := srv.Items.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	return instances
}

// mustFindItem returns the persisted inventory row with the given object id.
func mustFindItem(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32) *item.Instance {
	t.Helper()
	for _, inst := range persistedItems(t, srv, ownerID) {
		if inst.ObjectID == objectID {
			return inst
		}
	}
	t.Fatalf("no persisted item row for object %d (owner %d)", objectID, ownerID)
	return nil
}

// mustFindItemByTemplate returns the persisted inventory row carrying the
// given template id.
func mustFindItemByTemplate(t *testing.T, srv *gameservertest.Server, ownerID, templateID int32) *item.Instance {
	t.Helper()
	for _, inst := range persistedItems(t, srv, ownerID) {
		if inst.TemplateID == templateID {
			return inst
		}
	}
	t.Fatalf("no persisted item row for template %d (owner %d)", templateID, ownerID)
	return nil
}
