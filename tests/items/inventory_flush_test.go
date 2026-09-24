package items

import (
	"bytes"
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestDestroyFlushesBatchedUpdate drives a destroy, which has no synchronous
// reply of its own: an ItemList sync barrier proves the server processed it,
// one InventoryUpdates tick delivers exactly one modified-entry
// InventoryUpdate, and the items row matches the remaining stack.
func TestDestroyFlushesBatchedUpdate(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 20, 5)
	startInWorld(t, c)

	c.Send(encodeRequestDestroyItem(potion, 2))
	// The background batching tick may deliver the weight refresh and the
	// update ahead of the barrier reply; wait for the ItemList itself.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame != nil && frame[0] == serverpackets.OpcodeItemList {
			break
		}
	}
	srv.InventoryUpdates.Tick()
	e := readInventoryUpdateFor(t, c, potion, 3)
	if e.state != 2 {
		t.Fatalf("InventoryUpdate state = %d, want modified (2)", e.state)
	}
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, potion); inst.Count != 3 {
		t.Fatalf("persisted potion count = %d, want 3", inst.Count)
	}
}

// TestCrystallizeGrantsCrystals crystallizes the D-grade sword and requires
// the crystallized message plus one batched InventoryUpdate carrying both
// the removed source row and the added crystal reward, mirrored by the items
// rows.
func TestCrystallizeGrantsCrystals(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, 248, 3); err != nil {
		t.Fatalf("grant crystallize skill: %v", err)
	}
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeRequestCrystallizeItem(weapon, 1))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "crystallized SystemMessage")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageItemCrystallized {
		t.Fatalf("message id = %d, want ItemCrystallized (%d)", id, serverpackets.SystemMessageItemCrystallized)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("message params = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamItemName {
		t.Fatalf("param type = %d, want item name", typ)
	}
	if itemID := r.ReadInt32(); itemID != 30 {
		t.Fatalf("param item id = %d, want 30", itemID)
	}

	srv.InventoryUpdates.Tick()
	entries := readInventoryUpdateEntries(t, c.Read())
	var removedWeapon, addedCrystal bool
	for _, e := range entries {
		if e.state == 3 && e.objID == weapon && e.itemID == 30 {
			removedWeapon = true
		}
		if e.state == 1 && e.itemID == item.CrystalD.ItemID() && e.count == 10 {
			addedCrystal = true
		}
	}
	if !removedWeapon || !addedCrystal {
		t.Fatalf("crystallize InventoryUpdate entries = %+v, want removed weapon plus added 10 D crystals", entries)
	}

	srv.FlushItems(t)
	assertItemGone(t, srv, objID, weapon)
	crystalCount := 0
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.TemplateID == item.CrystalD.ItemID() {
			crystalCount += inst.Count
		}
	}
	if crystalCount != 10 {
		t.Fatalf("persisted crystal count after crystallize = %d, want 10", crystalCount)
	}
}

// TestCrystallizeWithoutSkillIsRejected pins the skill gate: without the
// crystallize skill the request answers CrystallizeLevelTooLow only, and the
// weapon row survives.
func TestCrystallizeWithoutSkillIsRejected(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeRequestCrystallizeItem(weapon, 1))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCrystallizeLevelTooLow)
	barrier(t, c)

	if inst := mustFindItem(t, srv, objID, weapon); inst.Count != 1 {
		t.Fatalf("weapon count after rejected crystallize = %d, want 1", inst.Count)
	}
}

// TestItemListDiscardsPendingInventoryUpdates drives a destroy and a full
// item-list request inside one tick window. The reference's ItemList
// constructor drops the pending update list before it reads the item set, so
// the snapshot supersedes the deltas it already describes: the client sees
// the ItemList and nothing else about that stack, and the drain that follows
// has nothing left to send.
func TestItemListDiscardsPendingInventoryUpdates(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 20, 5)
	startInWorld(t, c)

	// The connection handles both requests on one goroutine, and nothing
	// drains the batch without an explicit tick, so the snapshot is built
	// after the destroy queued its update and before any drain could run.
	c.Send(encodeRequestDestroyItem(potion, 2))
	c.Send(encodeRequestItemList())

	frames := readUntilOpcode(t, c, serverpackets.OpcodeItemList)
	assertNoInventoryUpdateFor(t, frames[:len(frames)-1], potion, "before the ItemList snapshot")
	entries := readItemListEntries(t, frames[len(frames)-1])
	e := findItemListEntry(entries, potion)
	if e == nil {
		t.Fatalf("ItemList entries = %+v, want a row for the destroyed stack", entries)
	}
	if e.count != 3 {
		t.Fatalf("ItemList count for the destroyed stack = %d, want 3", e.count)
	}

	// The tick now finds an empty update queue: the snapshot already told
	// the client the remaining count, so the reference sends nothing more.
	srv.InventoryUpdates.Tick()
	assertNoInventoryUpdateFor(t, readUntilQuiet(t, c), potion, "after the ItemList snapshot")

	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, potion); inst.Count != 3 {
		t.Fatalf("persisted potion count = %d, want 3", inst.Count)
	}
}

// TestItemListReplyPrecedesDrainQueuedBehindIt pins that the RequestItemList
// reply is queued for the client by the same player-queue task that builds
// it, so an inventory drain queued behind that task cannot overtake the
// snapshot it supersedes. Handing the frame back to the connection to send
// would let the drain win, and the client would apply the older full list
// last, keeping pre-drain counts until those items change again.
//
// Nothing on the wire tells the two apart — the connection's writer
// goroutine does every socket write either way — so the send observer checks
// the enqueuing goroutine itself, which makes the gate deterministic. The
// observer then posts a mutation and the drain it feeds from inside the reply
// task, placing them right behind it on the queue with no gate or sleep, and
// the client must see the snapshot before that drain's InventoryUpdate.
func TestItemListReplyPrecedesDrainQueuedBehindIt(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 20, 100)
	startInWorld(t, c)
	inv := srv.PlayerInventory(t, objID)
	queue := srv.PlayerQueue(t, objID)

	onTask := make(chan [2]bool, 1)
	srv.ObserveSends(t, func(payload []byte) {
		if len(payload) == 0 || payload[0] != serverpackets.OpcodeItemList {
			return
		}
		sent := onQueueTask(queue)
		// Posted while the reply task still runs, after its snapshot was
		// built, so the snapshot cannot cover this change and the drain owes
		// the client its delta.
		posted := sent && queue.Post(func() {
			inv.DestroyByObjectID(potion, 1)
			srv.InventoryUpdates.Tick()
		})
		select {
		case onTask <- [2]bool{sent, posted}:
		default: // only the first ItemList is under test
		}
	})

	c.Send(encodeRequestItemList())
	select {
	case got := <-onTask:
		if !got[0] {
			t.Fatal("ItemList reply was queued off the player-queue task that built it")
		}
		if !got[1] {
			t.Fatal("post destroy task behind the ItemList reply")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no ItemList reply was queued")
	}
	assertItemListPrecedesInventoryUpdate(t, c, potion, 99)
}

// onQueueTask reports whether the calling goroutine is running one of q's
// tasks. sim.AssertOwner alone only proves someone drains q outside simdebug,
// and q may already be on its next task when an off-task send happens, so the
// caller's own stack must also be inside a queue drain (sim.drainAs; renaming
// it fails this check loudly rather than passing it).
func onQueueTask(q *sim.Queue) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	sim.AssertOwner(q)
	buf := make([]byte, 64<<10)
	return bytes.Contains(buf[:runtime.Stack(buf, false)], []byte("/sim.drainAs("))
}

// assertItemListPrecedesInventoryUpdate reads until the drain's
// InventoryUpdate for objectID, requiring the ItemList snapshot to have
// arrived first and the drain to carry wantCount.
func assertItemListPrecedesInventoryUpdate(t *testing.T, c *testsupport.ScriptedClient, objectID, wantCount int32) {
	t.Helper()
	sawItemList := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		frame := c.ReadWithTimeout(500 * time.Millisecond)
		switch {
		case frame == nil:
		case frame[0] == serverpackets.OpcodeItemList:
			sawItemList = true
		case frame[0] == serverpackets.OpcodeInventoryUpdate:
			if !sawItemList {
				t.Fatal("InventoryUpdate arrived before the ItemList snapshot it supersedes")
			}
			if e := findInventoryUpdate(t, [][]byte{frame}, objectID); e.count != wantCount {
				t.Fatalf("InventoryUpdate count = %d, want %d", e.count, wantCount)
			}
			return
		}
	}
	t.Fatal("no InventoryUpdate for the destroyed stack")
}

// readUntilOpcode collects frames until one carries opcode, returning every
// frame read including that one, so a caller can assert on what preceded it.
func readUntilOpcode(t *testing.T, c *testsupport.ScriptedClient, opcode byte) [][]byte {
	t.Helper()
	var frames [][]byte
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		frame := c.ReadWithTimeout(500 * time.Millisecond)
		if frame == nil {
			continue
		}
		frames = append(frames, frame)
		if frame[0] == opcode {
			return frames
		}
	}
	t.Fatalf("no frame with opcode %#x in %d frames", opcode, len(frames))
	return nil
}

// readUntilQuiet collects every frame the client still receives until it
// falls silent for one read timeout.
func readUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 100 reads")
	return nil
}

// assertNoInventoryUpdateFor fails if any frame is an InventoryUpdate
// carrying objectID; when names the window being checked.
func assertNoInventoryUpdateFor(t *testing.T, frames [][]byte, objectID int32, when string) {
	t.Helper()
	for _, frame := range frames {
		if len(frame) == 0 || frame[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		for _, e := range readInventoryUpdateEntries(t, frame) {
			if e.objID == objectID {
				t.Fatalf("InventoryUpdate %s for object %d (count %d), want none", when, objectID, e.count)
			}
		}
	}
}
