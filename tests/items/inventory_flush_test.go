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

// TestItemListReplyPrecedesDrainQueuedBehindIt pins that the RequestItemList
// reply is sent by the same queue task that builds it. The player's queue is
// parked on a gate, the request's own task is queued behind that gate and a
// batched inventory drain behind the request, so the drain's InventoryUpdate
// must reach the client after the snapshot it supersedes. Handing the frame
// back to the connection to send lets the drain overtake it, and the client
// keeps showing the pre-drain counts until those items change again.
func TestItemListReplyPrecedesDrainQueuedBehindIt(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 20, 100)
	startInWorld(t, c)

	const rounds = 5
	count := int32(100)
	for round := range rounds {
		// A destroy has no reply of its own and leaves its InventoryUpdate
		// waiting for a tick; an ItemList sync proves the server ran it.
		c.Send(encodeRequestDestroyItem(potion, 1))
		count--
		syncOnItemList(t, c)
		drainUntilQuiet(t, c)

		// Every in-world opcode but a handful clears spawn protection on the
		// queue first, so the request lands as two tasks: park the queue
		// once to place that first task, then again to place the reply task
		// itself, with the drain queued behind it.
		queue := srv.PlayerQueue(t, objID)
		protectionRunning, releaseProtection := postGate(t, queue, round)
		<-protectionRunning
		c.Send(encodeRequestItemList())
		time.Sleep(gateSettle)
		replyRunning, releaseReply := postGate(t, queue, round)
		releaseProtection()
		<-replyRunning
		time.Sleep(gateSettle)
		srv.InventoryUpdates.Tick()
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

// syncOnItemList sends a RequestItemList and reads until its reply, ignoring
// the frames an earlier request already put on the wire.
func syncOnItemList(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestItemList())
	for range 100 {
		if frame := c.Read(); len(frame) > 0 && frame[0] == serverpackets.OpcodeItemList {
			return
		}
	}
	t.Fatal("no ItemList reply within 100 frames")
}
