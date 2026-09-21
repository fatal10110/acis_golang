package items

import (
	"context"
	"sync"
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
// reply is sent by the same queue task that builds it. The player's queue is
// parked on a gate and the reply task is queued behind that gate, then a
// mutation and the drain it feeds are queued behind the reply, so the
// InventoryUpdate for a change the snapshot never saw must still reach the
// client after that snapshot. Handing the frame back to the connection to
// send lets the drain overtake it, and the client applies the older full
// list last, keeping pre-drain counts until those items change again.
//
// The mutation is posted as a queue task rather than sent as a packet: the
// connection goroutine blocks on each request it posts, so a packet sent
// while the reply task is parked cannot be read, let alone queued, until
// that task has already run.
func TestItemListReplyPrecedesDrainQueuedBehindIt(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 20, 100)
	startInWorld(t, c)
	inv := srv.PlayerInventory(t, objID)

	// This gate is one-directional. With the reply sent on its task the
	// order is deterministic — reply, mutation and drain are three tasks on
	// one FIFO queue — so the test never fails spuriously. Detecting the
	// off-task regression is probabilistic: Conn.SendFrame only appends to
	// the connection's outbound queue under its own mutex (conn.go:170-193)
	// and a separate writer goroutine does the socket write, so an off-task
	// reply differs only in which goroutine appends first, which nothing the
	// client can observe distinguishes. Each round samples that race; the
	// round count is what makes the inversion show up at all. Measured with
	// the reply reverted to an off-task send, -race, 8 runs per executor
	// mode: pool 3/8, inline 6/8 (9/16 overall). Do not read a single green
	// run of a reverted fix as the property holding.
	const rounds = 15
	count := int32(100)
	for round := range rounds {
		// Every in-world opcode but a handful clears spawn protection on the
		// queue first, so the request lands as two tasks: park the queue
		// once to place that first task, then again to place the reply task
		// itself, with the mutation and its drain queued behind it.
		queue := srv.PlayerQueue(t, objID)
		protectionRunning, releaseProtection := postGate(t, queue, round)
		<-protectionRunning
		c.Send(encodeRequestItemList())
		time.Sleep(gateSettle)
		replyRunning, releaseReply := postGate(t, queue, round)
		releaseProtection()
		<-replyRunning
		time.Sleep(gateSettle)
		count--
		// Registered after the snapshot task builds, so the snapshot cannot
		// have covered it and the drain owes the client this delta. Ticking
		// from the same task keeps the drain on the queue right behind it,
		// with no test-goroutine sync to let the reply win by waiting.
		if !queue.Post(func() {
			inv.DestroyByObjectID(potion, 1)
			srv.InventoryUpdates.Tick()
		}) {
			t.Fatalf("round %d: post destroy task", round)
		}
		releaseReply()

		assertItemListPrecedesInventoryUpdate(t, c, round, potion, count)
	}
}

// gateSettle is how long a round waits for the connection to post the task a
// packet it has already been sent produces, while the queue is parked.
const gateSettle = 200 * time.Millisecond

// postGate posts a gate task to queue without waiting for it: the returned
// channel closes once the gate is running, and release lets it finish. Work
// posted while a gate runs stays pending, in post order, until then.
//
// release is idempotent and also registered with t.Cleanup, so a gate is
// never left held by a later t.Fatalf on a path that has not reached its own
// release yet. A gate held past the end of a test would block the executor's
// own cleanup: under inline that cleanup waits on a pump goroutine parked
// inside the gate, which hangs the whole package rather than failing a test.
func postGate(t *testing.T, queue *sim.Queue, round int) (running <-chan struct{}, release func()) {
	t.Helper()
	started, gate := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	if !queue.Post(func() { close(started); <-gate }) {
		t.Fatalf("round %d: post gate task", round)
	}
	return started, release
}

// assertItemListPrecedesInventoryUpdate reads until the drain's
// InventoryUpdate for objectID, requiring the ItemList snapshot to have
// arrived first and the drain to carry wantCount.
func assertItemListPrecedesInventoryUpdate(t *testing.T, c *testsupport.ScriptedClient, round int, objectID, wantCount int32) {
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
				t.Fatalf("round %d: InventoryUpdate arrived before the ItemList snapshot it supersedes", round)
			}
			if e := findInventoryUpdate(t, [][]byte{frame}, objectID); e.count != wantCount {
				t.Fatalf("round %d: InventoryUpdate count = %d, want %d", round, e.count, wantCount)
			}
			return
		}
	}
	t.Fatalf("round %d: no InventoryUpdate for the destroyed stack", round)
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
