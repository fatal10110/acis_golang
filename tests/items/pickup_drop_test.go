package items

import (
	"context"
	"testing"
	"time"

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

func countCarriedRows(t *testing.T, srv *gameservertest.Server, ownerID, templateID int32) int {
	t.Helper()
	n := 0
	for _, inst := range persistedItems(t, srv, ownerID) {
		if inst.TemplateID == templateID && inst.Location == item.LocationInventory {
			n++
		}
	}
	return n
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

// TestPickupWalksToDistantGroundItem covers the approach path: a pickup
// click on an item beyond interaction range starts a walk instead of
// answering, and the arrival completes the collection exactly like an
// in-range click does — GetItem, DeleteObject broadcast, merged inventory
// stack, and an empty items_on_ground table.
func TestPickupWalksToDistantGroundItem(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)

	c.Send(encodeRequestDropItem(adena, 40, spawnX, spawnY, spawnZ))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // dropper id
	groundID := r.ReadInt32()

	// Walk out of pickup range before clicking; wait until the server
	// reports the player at the destination.
	c.Send(encodeMoveBackwardToLocation(spawnX+200, spawnY, spawnZ, spawnX, spawnY, spawnZ))
	waitFor(t, "walk away completed", func() bool {
		x, _, _ := srv.PlayerPosition(t, objID)
		return x >= spawnX+195
	})
	drainUntilQuiet(t, c)

	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pending-action release")

	// The approach walk runs before the collection resolves.
	deadline := time.Now().Add(15 * time.Second)
	var getItem []byte
	for time.Now().Before(deadline) {
		if frame := c.ReadWithTimeout(500 * time.Millisecond); frame != nil {
			if frame[0] == serverpackets.OpcodeGetItem {
				getItem = frame
				break
			}
			continue
		}
	}
	if getItem == nil {
		t.Fatal("pickup after walking never collected the item")
	}
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeDeleteObject, "DeleteObject")

	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, adena, 100)
	srv.FlushItems(t)
	count := 0
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.TemplateID == item.AdenaID {
			count += inst.Count
		}
	}
	if count != 100 {
		t.Fatalf("persisted adena after walked pickup = %d, want 100", count)
	}
}

// TestPickupRejectsLootLockedByOtherOwner pins the loot lock: another
// player's drop answers ActionFailed plus the failed-pickup system message,
// and the ground item stays where it is.
func TestPickupRejectsLootLockedByOtherOwner(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	second := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	srv.SeedGroundItem(t, second.ID, item.AdenaID, 40, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)

	groundID := soleGroundObjectID(t, srv)
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "loot-locked pickup")
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "failed-pickup SystemMessage")
	barrier(t, c)

	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatal("loot-locked ground item was removed by a non-owner pickup attempt")
	}
}

// TestPickupRejectedWhileTrading pins the trade-window gate: while the
// picker has an open trade, a pickup click answers ActionFailed followed by
// CannotPickupOrUseItemTrading, and nothing leaves the ground.
func TestPickupRejectedWhileTrading(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)

	second := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestDropItem(adena, 40, spawnX, spawnY, spawnZ))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
	r := wire.NewReader(frame[1:])
	r.ReadInt32()
	groundID := r.ReadInt32()
	drainUntilQuiet(t, observer)

	openTrade(t, c, observer, objID, second.ID)

	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "trading pickup gate")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotPickupOrUseItemTrading)

	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatal("ground item removed while its picker was trading")
	}
}

// TestPickupSlotsFullRejection pins the full-inventory rejection: ActionFailed
// leads the SlotsFull message so the client's click is always released.
func TestPickupSlotsFullRejection(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)
	srv.SetInventorySlotLimit(t, objID, 1)

	srv.SeedGroundItem(t, objID, 30, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	groundID := soleGroundObjectID(t, srv)

	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "slots-full lead")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSlotsFull)

	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatal("ground item removed after a slots-full pickup attempt")
	}
	if inst := mustFindItem(t, srv, objID, sword); inst.Location != item.LocationInventory {
		t.Fatalf("held weapon disturbed by rejected pickup: %+v", inst)
	}
}

// TestPickupAttentionAnnouncedToObservers pins the pickup announcement: when
// a player picks up a weapon, every nearby other client receives the
// attention system message naming the picker and the item; an etc-item
// pickup announces nothing.
func TestPickupAttentionAnnouncedToObservers(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)
	srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, c)

	pick := func(objectID int32) [][]byte {
		t.Helper()
		c.Send(encodeRequestDropItem(objectID, 1, spawnX, spawnY, spawnZ))
		frame := c.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
		r := wire.NewReader(frame[1:])
		r.ReadInt32()
		groundID := r.ReadInt32()
		drainUntilQuiet(t, observer)

		c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup release")
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")
		frames := collectUntilQuiet(t, observer)
		srv.InventoryUpdates.Tick()
		waitForInventoryUpdate(t, c, objectID)
		return frames
	}

	for _, f := range pick(weapon) {
		if f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageAttentionS1PickedUpS2 {
			return // announced to the observer as expected
		}
	}
	t.Fatal("weapon pickup produced no attention message for the observer")
}

func TestPickupAttentionSkippedForEtcItems(t *testing.T) {
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
	r.ReadInt32()
	groundID := r.ReadInt32()
	drainUntilQuiet(t, observer)

	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")

	for _, f := range collectUntilQuiet(t, observer) {
		if f[0] == serverpackets.OpcodeSystemMessage {
			t.Fatalf("etc pickup broadcast SystemMessage %d to the observer", systemMessageID(t, f))
		}
	}
}

// TestWeightGaugeRefreshesOnEveryChange pins the load-gauge refresh (#1137):
// any weight change — not only band crossings — sends StatusUpdate(CUR_LOAD)
// to the owner.
func TestWeightGaugeRefreshesOnEveryChange(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 9500, 4) // heavy ingot, stackable, weight 10
	startInWorld(t, c)

	c.Send(encodeRequestDestroyItem(potion, 2))
	// Let the handler settle, then drive the batching task: its drain sends
	// the InventoryUpdate and refreshes the carried weight afterwards.
	time.Sleep(200 * time.Millisecond)
	srv.InventoryUpdates.Tick()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame := c.ReadWithTimeout(200 * time.Millisecond)
		if frame == nil {
			continue
		}
		if frame[0] == serverpackets.OpcodeStatusUpdate {
			if attrs := statusUpdateAttrs(t, frame); attrs[serverpackets.StatusCurrentLoad] > 0 {
				return // the gauge refreshed with a concrete load value
			}
		}
	}
	t.Fatal("destroying part of a stack sent no StatusUpdate carrying CUR_LOAD")
}

// TestPackageSendableList pins the freight window list wire-through: only
// carried, unequipped inventory items are listed — the equipped weapon is
// excluded — and the request's object id plus the adena count echo back.
// The full carry/warehouse/quest filtering matrix stays in the itemcontainer
// package's pure-function tests.
func TestPackageSendableList(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adenaObj := srv.GiveItem(t, objID, item.AdenaID, 100)
	shield := srv.GiveItem(t, objID, 20, 3)
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(weapon, false))
	readSkippingEquipNoise(t, c, "equip UserInfo")
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, weapon, 1)

	c.Send(encodeRequestPackageSendableItemList(objID))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodePackageSendableList, "PackageSendableList")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("PackageSendableList object id = %d, want %d", got, objID)
	}
	if got := r.ReadInt32(); got != 100 {
		t.Fatalf("PackageSendableList adena = %d, want 100", got)
	}
	if n := r.ReadInt32(); n != 2 {
		t.Fatalf("PackageSendableList count = %d, want 2 carried items (carried rows: adena=%d shield=%d weapon=%d)", n,
			countCarriedRows(t, srv, objID, item.AdenaID),
			countCarriedRows(t, srv, objID, 20),
			countCarriedRows(t, srv, objID, 30))
	}
	type entry struct {
		category uint16
		objID    int32
		itemID   int32
		count    int32
	}
	var entries []entry
	for i := int32(0); i < 2; i++ {
		e := entry{category: r.ReadUint16(), objID: r.ReadInt32(), itemID: r.ReadInt32(), count: r.ReadInt32()}
		r.ReadUint16() // subCategory
		r.ReadUint16() // CustomType1
		r.ReadInt32()  // paperdoll slot
		r.ReadUint16() // enchant
		r.ReadUint16() // CustomType2
		r.ReadUint16() // ?
		r.ReadInt32()  // object id repeat
		entries = append(entries, e)
	}
	wantIDs := map[int32]bool{adenaObj: true, shield: true}
	for _, e := range entries {
		if !wantIDs[e.objID] {
			t.Fatalf("unexpected sendable entry %+v, want only adena %d and shield %d", e, adenaObj, shield)
		}
		delete(wantIDs, e.objID)
	}
	if len(wantIDs) != 0 {
		t.Fatalf("sendable list missing expected objects %+v: got %+v", wantIDs, entries)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("parse PackageSendableList: %v", err)
	}
}

// openTrade performs the request/answer handshake between two entered
// clients, leaving both inside an open trade window.
func openTrade(t *testing.T, requester, target *testsupport.ScriptedClient, requesterID, targetID int32) {
	t.Helper()
	requester.Send(encodeTradeRequest(targetID))
	assertFrameOpcode(t, target.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertStaticSystemMessageText(t, requester.Read())
	target.Send(encodeAnswerTradeRequest(1))
	for _, who := range []*testsupport.ScriptedClient{requester, target} {
		assertStaticSystemMessageText(t, who.Read())
		assertFrameOpcode(t, who.Read(), serverpackets.OpcodeTradeStart, "TradeStart")
	}
}

// assertStaticSystemMessageText accepts any single-text-param SystemMessage
// without pinning its exact wording.
func assertStaticSystemMessageText(t *testing.T, frame []byte) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	if id := systemMessageID(t, frame); id == 0 {
		t.Fatalf("system message id = 0")
	}
}

// soleGroundObjectID returns the single tracked ground item's object id.
func soleGroundObjectID(t *testing.T, srv *gameservertest.Server) int32 {
	t.Helper()
	snaps := srv.GroundItems.Snapshots(nil)
	if len(snaps) != 1 {
		t.Fatalf("tracked ground items = %d, want 1", len(snaps))
	}
	return snaps[0].ObjectID
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s not observed within 10s", what)
}

// waitForInventoryUpdate reads frames until an InventoryUpdate carrying
// objectID with wantCount arrives.
func waitForInventoryUpdate(t *testing.T, c *testsupport.ScriptedClient, objectID int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame := c.ReadWithTimeout(200 * time.Millisecond)
		if frame == nil || frame[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		for _, e := range readInventoryUpdateEntries(t, frame) {
			if e.objID == objectID {
				return
			}
		}
	}
	t.Fatalf("no InventoryUpdate for object %d within 5s", objectID)
}

// collectUntilQuiet gathers frames until the client goes quiet, returning
// everything it received.
func collectUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 100 drains")
	return nil
}
