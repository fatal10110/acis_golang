package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPickupLosesToEarlierClaim pins the pickup claim through the real click
// path. A drop some other picker already holds answers the click with a bare
// ActionFailed, the reference's silent return for an item that is no longer
// visible, and leaves the item on the ground. Once that holder lets go the
// same click picks it up, and the claim a pickup keeps means the drop can
// never be taken a second time.
func TestPickupLosesToEarlierClaim(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	startInWorld(t, c)
	srv.SeedGroundItem(t, 0, 30, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	groundID := soleGroundObjectID(t, srv)
	ground := groundItem(t, srv, groundID)

	if !ground.Claim() {
		t.Fatal("fresh drop refused its first claim")
	}
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "claimed-drop pickup")
	barrier(t, c)
	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatal("a claimed drop left the ground for a second picker")
	}
	if n := carriedCount(t, srv, objID, 30); n != 0 {
		t.Fatalf("carried weapons after losing the claim = %d, want 0", n)
	}

	ground.Release()
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")
	drainUntilQuiet(t, c)
	if _, ok := srv.State.Object(groundID); ok {
		t.Fatal("picked-up drop is still on the ground")
	}
	if n := carriedCount(t, srv, objID, 30); n != 1 {
		t.Fatalf("carried weapons after the pickup = %d, want 1", n)
	}
	if ground.Claim() {
		t.Fatal("a picked-up drop accepted another claim")
	}
}

// TestRejectedPickupReleasesClaim pins the put-back: a pickup that claims the
// drop and is then refused (here by a full inventory) hands it back, so the
// next picker still can take it.
func TestRejectedPickupReleasesClaim(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)
	srv.SetInventorySlotLimit(t, objID, 1)
	srv.SeedGroundItem(t, 0, 30, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	groundID := soleGroundObjectID(t, srv)

	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "slots-full lead")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSlotsFull)
	barrier(t, c)

	if !groundItem(t, srv, groundID).Claim() {
		t.Fatal("a refused pickup kept its claim on the drop")
	}
}

// TestConcurrentPickupsTakeOneItem races two players on separate queues for
// the same drop, round after round: every drop ends up in exactly one
// inventory. Run on the real pool (ACIS_SIM_EXECUTOR=pool) this is where two
// pickups actually overlap.
func TestConcurrentPickupsTakeOneItem(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	first := srv.Client
	firstID := srv.SoleObjectID(t)
	startInWorld(t, first)
	second := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	other := srv.DialClient(t, "player2", 1)
	startInWorld(t, other)
	drainUntilQuiet(t, other)
	drainUntilQuiet(t, first)

	const rounds = 8
	for round := 1; round <= rounds; round++ {
		srv.SeedGroundItem(t, 0, 30, 1, spawnX, spawnY, spawnZ)
		groundID := soleGroundObjectID(t, srv)
		first.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
		other.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
		waitFor(t, "drop leaves the ground", func() bool {
			_, ok := srv.State.Object(groundID)
			return !ok
		})
		drainUntilQuiet(t, first)
		drainUntilQuiet(t, other)
		if got := carriedCount(t, srv, firstID, 30) + carriedCount(t, srv, second.ID, 30); got != round {
			t.Fatalf("round %d: weapons carried by both players = %d, want %d", round, got, round)
		}
		// Let the winner's post-pickup paralysis lapse so the next round's
		// clicks are not deferred behind it.
		time.Sleep(250 * time.Millisecond)
	}
}

// TestGroundCleanupSkipsClaimedDrop pins the cleanup side of the claim: an
// expired drop a pickup is holding stays in the world, and the next tick
// after the pickup gives it back expires it.
func TestGroundCleanupSkipsClaimedDrop(t *testing.T) {
	t.Parallel()
	tmpl := &item.Template{ID: 57, Stackable: true}
	now := time.Unix(0, 0)
	state := world.New()
	ground, err := grounditem.New(item.Instance{ObjectID: 900001, TemplateID: 57, Count: 1}, tmpl)
	if err != nil {
		t.Fatalf("grounditem.New: %v", err)
	}
	g := task.NewGroundItems(state, task.GroundItemOptions{ItemAutoDestroy: time.Second, PlayerDroppedMultiplier: 1}, func() time.Time { return now })
	g.Drop(ground, task.DropOptions{})

	if !ground.Claim() {
		t.Fatal("fresh drop refused its first claim")
	}
	now = now.Add(2 * time.Second)
	g.Tick()
	if _, ok := state.Object(ground.ObjectID()); !ok {
		t.Fatal("cleanup despawned a drop a pickup was holding")
	}

	ground.Release()
	g.Tick()
	if _, ok := state.Object(ground.ObjectID()); ok {
		t.Fatal("expired drop survived the tick after its claim was released")
	}
	if ground.Claim() {
		t.Fatal("an expired drop accepted a pickup claim")
	}
}

func groundItem(t *testing.T, srv *gameservertest.Server, objectID int32) *grounditem.Item {
	t.Helper()
	obj, ok := srv.State.Object(objectID)
	if !ok {
		t.Fatalf("ground item %d not in the world", objectID)
	}
	ground, ok := obj.(*grounditem.Item)
	if !ok {
		t.Fatalf("world object %d = %T, want *grounditem.Item", objectID, obj)
	}
	return ground
}

// carriedCount returns how many units of templateID the online player's live
// inventory holds.
func carriedCount(t *testing.T, srv *gameservertest.Server, ownerID, templateID int32) int {
	t.Helper()
	obj, ok := srv.State.Player(ownerID)
	if !ok {
		t.Fatalf("player %d not online", ownerID)
	}
	holder, ok := obj.(interface {
		Inventory() *itemcontainer.Inventory
	})
	if !ok {
		t.Fatalf("player %d = %T has no inventory", ownerID, obj)
	}
	n := 0
	for _, inst := range holder.Inventory().ItemsByTemplateID(templateID) {
		n += inst.Snapshot().Count
	}
	return n
}
